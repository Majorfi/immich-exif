package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
	"github.com/majorfi/immich-exif/process"
	"github.com/majorfi/immich-exif/state"
	"github.com/majorfi/immich-exif/ui"
)

var snapshotAssetFn = state.SnapshotAsset

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := parseConfig()
	if err != nil {
		if errors.Is(err, errShowVersion) {
			fmt.Println("immich-exif " + version)
			return 0
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		return 1
	}

	warnCredentialHygiene()

	if err := exif.CheckExiftoolFn(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	tmpDir, err := setupTmpDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	cfg.TmpDir = tmpDir
	defer os.RemoveAll(tmpDir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := api.NewImmichClient(cfg.URL, cfg.APIKey)

	if err := client.ResolveAPIMode(cfg.ImmichAPI); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot reach Immich server: %v\n", err)
		return 1
	}

	var stateDB *state.StateDB
	snapshots := map[string]string{}
	var shouldSkip func(model.AssetResponse) bool

	if shouldUseStateCache(cfg) {
		stateDBFilePath, stateDBPathErr := state.StateDBPath()
		if stateDBPathErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not resolve state DB path: %v (proceeding without cache)\n", stateDBPathErr)
		} else {
			db, dbErr := state.OpenStateDB(stateDBFilePath, cfg.URL)
			if dbErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not open state DB: %v (proceeding without cache)\n", dbErr)
			} else {
				stateDB = db
				defer stateDB.Close()
			}
		}

		if stateDB != nil && cfg.Force {
			shouldSkip = func(asset model.AssetResponse) bool {
				rememberAssetSnapshot(asset, snapshots)
				return false
			}
		} else if stateDB != nil {
			shouldSkip = func(asset model.AssetResponse) bool {
				snap, ok := rememberAssetSnapshot(asset, snapshots)
				if !ok {
					return false
				}
				if stateDB.IsUpToDate(asset.ID, snap) {
					return true
				}
				return false
			}
		}
	}

	assetIDs, stats, err := resolveAssetIDs(client, cfg, shouldSkip)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving assets: %v\n", err)
		return 1
	}

	if stats.NoWritableMetadataSkipped > 0 {
		fmt.Printf("Pre-filtered %d assets with no writable metadata to embed\n", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped > 0 {
		fmt.Printf("Pre-filtered %d unsupported video assets\n", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped > 0 {
		fmt.Printf("Skipped %d assets with unchanged metadata\n", stats.StateSkipped)
	}

	if len(assetIDs) == 0 {
		fmt.Println("No assets to process.")
		return 0
	}

	uploader := &process.ModernUploader{Client: client, ResolveDuplicate: cfg.ResolveDuplicate, VerifyUpload: cfg.VerifyUpload}

	if cfg.ExportDir != "" {
		cfg.ExportDir = resolveExportDir(cfg)
		if err := os.MkdirAll(cfg.ExportDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating export dir: %v\n", err)
			return 1
		}
	}

	results := runPipeline(ctx, client, uploader, cfg, assetIDs)

	if !cfg.ResolveDuplicate {
		printUnresolvedDuplicateHint(results)
		followUpResults := maybeResolveDuplicatesNow(ctx, client, cfg, results)
		if len(followUpResults) > 0 {
			results = append(results, followUpResults...)
			printUnresolvedDuplicateHint(followUpResults)
		}
	}

	if stateDB != nil {
		state.SaveProcessedState(stateDB, snapshots, results)
	}

	if len(finalUnresolvedDuplicateResults(results)) > 0 {
		fmt.Fprintf(os.Stderr, "Error: unresolved duplicate uploads remain; rerun with -resolve-duplicate\n")
		return 1
	}

	for _, r := range results {
		if r.Status == model.StatusFailed {
			return 1
		}
	}
	return 0
}

func rememberAssetSnapshot(asset model.AssetResponse, snapshots map[string]string) (string, bool) {
	snap, err := snapshotAssetFn(asset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to snapshot asset %s: %v (state cache disabled for this asset)\n", asset.ID, err)
		return "", false
	}
	snapshots[asset.ID] = snap
	return snap, true
}

func runPipeline(ctx context.Context, client *api.ImmichClient, uploader process.Uploader, cfg *model.Config, assetIDs []string) []model.ProcessResult {
	if !cfg.Yes && cfg.Workers > 1 {
		fmt.Printf("Interactive mode forces single worker (requested: %d)\n", cfg.Workers)
		cfg.Workers = 1
	}

	emitter := &ui.LogEmitter{AutoConfirm: cfg.Yes}

	pool := process.NewWorkerPool(client, uploader, cfg, emitter)

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nInterrupt received, finishing in-flight work and stopping...")
			pool.Cancel()
		case <-done:
		}
	}()

	results := pool.Process(assetIDs)
	emitter.EmitAllDone(model.AllDoneEvent{Results: results})

	return results
}

func resolveAssetIDs(client *api.ImmichClient, cfg *model.Config, shouldSkip func(model.AssetResponse) bool) ([]string, api.AssetSelectionStats, error) {
	if len(cfg.AssetIDs) > 0 {
		return dedup(cfg.AssetIDs), api.AssetSelectionStats{}, nil
	}

	var allIDs []string
	shouldMirrorExportByAlbum := shouldMirrorExportByAlbum(cfg)
	exportAlbumIDsByAsset := map[string][]string{}
	stats := api.AssetSelectionStats{}

	if cfg.All {
		ids, allStats, err := client.ListAllAssetIDs(shouldSkip)
		if err != nil {
			return nil, api.AssetSelectionStats{}, fmt.Errorf("list all assets: %w", err)
		}
		allIDs = append(allIDs, ids...)
		stats.NoWritableMetadataSkipped += allStats.NoWritableMetadataSkipped
		stats.UnsupportedVideoSkipped += allStats.UnsupportedVideoSkipped
		stats.StateSkipped += allStats.StateSkipped
	}

	if cfg.AllAlbums && (!cfg.All || shouldMirrorExportByAlbum) {
		albumIDs, err := client.ListAlbumIDs()
		if err != nil {
			return nil, api.AssetSelectionStats{}, fmt.Errorf("list albums: %w", err)
		}
		for _, albumID := range albumIDs {
			ids, err := client.GetAlbumAssets(albumID)
			if err != nil {
				return nil, api.AssetSelectionStats{}, fmt.Errorf("album %s: %w", albumID, err)
			}
			if !cfg.All {
				allIDs = append(allIDs, ids...)
			}
			if shouldMirrorExportByAlbum {
				addAlbumAssetMappings(exportAlbumIDsByAsset, ids, albumID)
			}
		}
	}

	for _, albumID := range cfg.AlbumIDs {
		ids, err := client.GetAlbumAssets(albumID)
		if err != nil {
			return nil, api.AssetSelectionStats{}, fmt.Errorf("album %s: %w", albumID, err)
		}
		allIDs = append(allIDs, ids...)
		if shouldMirrorExportByAlbum {
			addAlbumAssetMappings(exportAlbumIDsByAsset, ids, albumID)
		}
	}

	allIDs = dedup(allIDs)

	if shouldMirrorExportByAlbum {
		if cfg.IncludeNoAlbum {
			addNoAlbumAssetMappings(exportAlbumIDsByAsset, allIDs)
		} else if cfg.All && cfg.AllAlbums {
			allIDs = filterAssetIDsWithAlbumMappings(allIDs, exportAlbumIDsByAsset)
		}
		cfg.ExportAlbumIDsByAsset = exportAlbumIDsByAsset
	} else {
		cfg.ExportAlbumIDsByAsset = nil
	}

	return allIDs, stats, nil
}
