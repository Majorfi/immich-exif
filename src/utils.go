package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
	"github.com/majorfi/immich-exif/process"
)

const noAlbumDirName = "no-album"

func setupTmpDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	dir, err := os.MkdirTemp(wd, ".immich-exif-tmp-")
	if err != nil {
		return "", fmt.Errorf("create tmp dir: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("harden tmp dir: %w", err)
	}
	return dir, nil
}

func dedup(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	var result []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func shouldUseStateCache(cfg *model.Config) bool {
	if cfg == nil || !cfg.All {
		return false
	}
	if cfg.DryRun {
		return false
	}
	if cfg.ExportDir != "" {
		return false
	}
	return true
}

func shouldMirrorExportByAlbum(cfg *model.Config) bool {
	if cfg == nil || cfg.ExportDir == "" {
		return false
	}
	return cfg.AllAlbums || len(cfg.AlbumIDs) > 1
}

func resolveExportDir(cfg *model.Config) string {
	if cfg.ExportDir == "" {
		return ""
	}
	if len(cfg.AlbumIDs) == 1 {
		return filepath.Join(cfg.ExportDir, cfg.AlbumIDs[0])
	}
	return cfg.ExportDir
}

func addAlbumAssetMappings(exportAlbumIDsByAsset map[string][]string, assetIDs []string, albumID string) {
	for _, assetID := range assetIDs {
		existingAlbumIDs := exportAlbumIDsByAsset[assetID]
		if containsString(existingAlbumIDs, albumID) {
			continue
		}
		exportAlbumIDsByAsset[assetID] = append(existingAlbumIDs, albumID)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func addNoAlbumAssetMappings(exportAlbumIDsByAsset map[string][]string, assetIDs []string) {
	for _, assetID := range assetIDs {
		if len(exportAlbumIDsByAsset[assetID]) > 0 {
			continue
		}
		exportAlbumIDsByAsset[assetID] = []string{noAlbumDirName}
	}
}

func filterAssetIDsWithAlbumMappings(assetIDs []string, exportAlbumIDsByAsset map[string][]string) []string {
	var filtered []string
	for _, assetID := range assetIDs {
		if len(exportAlbumIDsByAsset[assetID]) == 0 {
			continue
		}
		filtered = append(filtered, assetID)
	}
	return filtered
}

func unresolvedDuplicateResults(results []model.ProcessResult) []model.ProcessResult {
	seen := map[string]bool{}
	var unresolved []model.ProcessResult
	for _, result := range results {
		if result.Status != model.StatusSkipped || result.DuplicateID == "" {
			continue
		}
		key := result.AssetID + "->" + result.DuplicateID
		if seen[key] {
			continue
		}
		seen[key] = true
		unresolved = append(unresolved, result)
	}
	return unresolved
}

func finalUnresolvedDuplicateResults(results []model.ProcessResult) []model.ProcessResult {
	lastResultIndexByAssetID := map[string]int{}
	for index, result := range results {
		lastResultIndexByAssetID[result.AssetID] = index
	}

	var unresolved []model.ProcessResult
	for index, result := range results {
		if lastResultIndexByAssetID[result.AssetID] != index {
			continue
		}
		if result.Status != model.StatusSkipped || result.DuplicateID == "" {
			continue
		}
		unresolved = append(unresolved, result)
	}
	return unresolved
}

func unresolvedDuplicateAssetIDs(results []model.ProcessResult) []string {
	unresolved := unresolvedDuplicateResults(results)
	seenAssetIDs := map[string]bool{}
	var assetIDs []string
	for _, result := range unresolved {
		if seenAssetIDs[result.AssetID] {
			continue
		}
		seenAssetIDs[result.AssetID] = true
		assetIDs = append(assetIDs, result.AssetID)
	}
	return assetIDs
}

func buildResolveDuplicateCommand(results []model.ProcessResult) string {
	assetIDs := unresolvedDuplicateAssetIDs(results)
	if len(assetIDs) == 0 {
		return ""
	}
	return "immich-exif -y -resolve-duplicate " + strings.Join(assetIDs, " ")
}

func printUnresolvedDuplicateHint(results []model.ProcessResult) {
	unresolved := unresolvedDuplicateResults(results)
	if len(unresolved) == 0 {
		return
	}

	fmt.Printf("\nSkipped duplicate uploads (%d):\n", len(unresolved))
	for _, result := range unresolved {
		fmt.Printf("  %s -> %s\n", result.AssetID, result.DuplicateID)
	}

	command := buildResolveDuplicateCommand(results)
	if command != "" {
		fmt.Println("\nTo patch them automatically, rerun with the same auth flags or environment and:")
		fmt.Printf("  %s\n", command)
	}
}

func maybeResolveDuplicatesNow(ctx context.Context, client *api.ImmichClient, cfg *model.Config, results []model.ProcessResult) []model.ProcessResult {
	if cfg.ResolveDuplicate || cfg.Yes {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	assetIDs := unresolvedDuplicateAssetIDs(results)
	if len(assetIDs) == 0 {
		return nil
	}
	if !isTerminalFile(os.Stdin) || !isTerminalFile(os.Stdout) {
		return nil
	}
	if !promptResolveDuplicatesNow(os.Stdin, os.Stdout) {
		return nil
	}

	fmt.Printf("\nRe-running unresolved duplicates with -resolve-duplicate (%d assets)\n", len(assetIDs))
	resolveCfg := buildResolveDuplicateFollowUpConfig(cfg)
	resolveUploader := &process.ModernUploader{Client: client, ResolveDuplicate: true, VerifyUpload: cfg.VerifyUpload}
	return runPipeline(ctx, client, resolveUploader, resolveCfg, assetIDs)
}

func promptResolveDuplicatesNow(reader io.Reader, writer io.Writer) bool {
	fmt.Fprint(writer, "\nPatch unresolved duplicates now? [y/N]: ")
	input, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && len(input) == 0 {
		return false
	}
	return isAffirmativeInput(input)
}

func isAffirmativeInput(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	return normalized == "y" || normalized == "yes"
}

func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func buildResolveDuplicateFollowUpConfig(cfg *model.Config) *model.Config {
	resolveCfg := *cfg
	resolveCfg.ResolveDuplicate = true
	resolveCfg.Yes = true
	return &resolveCfg
}
