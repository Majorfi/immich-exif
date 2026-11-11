package process

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

var sleepFn = time.Sleep

func ProcessAsset(client *api.ImmichClient, uploader Uploader, cfg *model.Config, assetID string, index, total int, emitter model.EventEmitter, cancelled func() bool) model.ProcessResult {
	fail := func(msg string, args ...any) model.ProcessResult {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusFailed, Message: fmt.Sprintf("[%s] %s", model.ShortID(assetID), fmt.Sprintf(msg, args...))}
	}

	asset, err := client.GetAsset(assetID)
	if err != nil {
		return fail("fetch asset: %v", err)
	}
	// Filename stays empty: with it set, LogEmitter would print an extra
	// "=> id | file" header and update its asset-grouping state, which drives
	// the blank-line separators between diff blocks.
	displayName := model.TruncateFilename(asset.OriginalFileName, 60)
	stepProgress := func(step string, percent int) {
		emitter.EmitProgress(model.ProgressEvent{
			AssetID: assetID,
			Index:   index,
			Total:   total,
			Percent: percent,
			Step:    step,
		})
	}
	stepProgress("Scanning "+displayName+"...", 0)
	defer emitter.EmitProgress(model.ProgressEvent{AssetID: assetID, Index: index, Total: total, Done: true})
	if asset.IsTrashed {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "asset is in the Immich trash; restore it before processing"}
	}
	// Replacing an external-library asset would migrate a copy into the
	// internal library and duplicate it at the next scan; read-only modes
	// (dry-run, export) are safe. A set libraryId always means external on
	// servers this client can address: the /api/assets routes and nullable
	// libraryId both arrived in Immich 1.106.
	inPlace := inPlaceTarget{}
	if asset.LibraryID != "" && len(cfg.InPlaceMaps) > 0 {
		var note string
		if inPlace, note = resolveInPlace(cfg, *asset); note != "" {
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: note}
		}
	}
	if cfg.WillReplace() && asset.LibraryID != "" && inPlace.local == "" {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "asset belongs to an external library; replacing it would migrate it into the internal library (-in-place-map updates it in place; dry-run and export still work)"}
	}
	if model.IsLivePhotoMotionCandidate(*asset) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "hidden video (likely a live-photo motion part); replacing it would sever the pair"}
	}
	if model.IsUnsupportedVideoAsset(*asset) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "unsupported video container for metadata embedding"}
	}
	if len(exif.CollectExifArgs(exif.CompareAssetMetadata(*asset, nil))) == 0 && !wantsFaceRegions(cfg, *asset) && !cfg.BakesEdits(*asset) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "no metadata to embed"}
	}
	if strings.TrimSpace(asset.Checksum) == "" {
		return fail("server returned no checksum for this asset; refusing to replace without integrity verification")
	}

	plan := bakePlan{}
	if cfg.BakesEdits(*asset) {
		if plan, err = planBake(client, *asset); err != nil {
			return fail("apply edits: %v", err)
		}
		if plan.skipNote != "" {
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: plan.skipNote}
		}
	}

	assetDir, err := os.MkdirTemp(cfg.TmpDir, assetID+"-*")
	if err != nil {
		return fail("create temp dir: %v", err)
	}
	defer os.RemoveAll(assetDir)

	safeName, err := safePathComponent("asset filename", asset.OriginalFileName)
	if err != nil {
		return fail("%v", err)
	}
	filePath := filepath.Join(assetDir, safeName)
	stepProgress("Downloading "+displayName+"...", 0)
	contentChecksum := asset.Checksum
	if model.IsPathChecksum(*asset) {
		contentChecksum = ""
	}
	if err := client.DownloadAsset(assetID, filePath, contentChecksum, func(percent int) {
		stepProgress("Downloading "+displayName+"...", percent)
	}); err != nil {
		return fail("download: %v", err)
	}

	if inPlace.local != "" {
		var note string
		if inPlace, note, err = prepareInPlace(client, cfg, *asset, inPlace, filePath); err != nil {
			return fail("in-place check: %v", err)
		}
		if note != "" {
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: note}
		}
	}

	baked := bakedFile{}
	var existing exif.ExifTagMap
	if cfg.BakesEdits(*asset) {
		stepProgress("Downloading edited "+displayName+"...", 0)
		baked, err = bakeEdits(client, *asset, plan, filePath, func(percent int) {
			stepProgress("Downloading edited "+displayName+"...", percent)
		})
		if err != nil {
			return fail("apply edits: %v", err)
		}
		if baked.skipNote != "" {
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: baked.skipNote}
		}
		filePath, safeName, existing = baked.path, filepath.Base(baked.path), baked.tags
	} else {
		stepProgress("Analyzing "+displayName+"...", 0)
		existing, err = exif.ReadExifTagsFn(filePath)
		if err != nil {
			return fail("read exif: %v", err)
		}
	}

	changes := exif.CompareAssetMetadata(*asset, existing)
	if baked.path != "" {
		if tags := bakedTagsChange(*asset, existing); tags != nil {
			changes = append(changes, *tags)
		}
	}
	changes, faceRegions, faceSkip, err := appendFaceRegionChange(client, cfg, *asset, existing, changes)
	if err != nil {
		return fail("fetch faces: %v", err)
	}
	exifArgs := exif.CollectExifArgs(changes)
	if len(exifArgs) == 0 && baked.path == "" {
		message := "metadata already matches"
		if faceSkip != "" {
			message += "; no face regions written: " + faceSkip
		}
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: message, ExifMatched: true}
	}

	diffEntries := exif.CollectDiffEntries(changes)
	// Surface the reason inside the diff block: it is the one place a piped or
	// -y run still prints, so "-faces wrote nothing" is never silent.
	if faceSkip != "" {
		diffEntries = append(diffEntries, model.DiffEntry{Tag: "Face regions", Symbol: model.DiffChange, Old: "(skipped)", New: faceSkip})
	}
	if baked.path != "" {
		diffEntries = append(diffEntries, model.DiffEntry{Tag: "Immich edits", Symbol: model.DiffChange, Old: "(Immich only)", New: "baked into the file: " + baked.edits})
	}
	action := emitter.EmitDiff(model.DiffEvent{
		AssetID:  assetID,
		Filename: asset.OriginalFileName,
		Index:    index,
		Total:    total,
		Entries:  diffEntries,
	})

	switch action {
	case model.ActionSkip:
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "user skipped"}
	case model.ActionQuit:
		return model.ProcessResult{AssetID: assetID, Status: model.StatusFailed, Message: "user cancelled", Cancelled: true}
	}

	if len(exifArgs) > 0 {
		stepProgress("Writing tags to "+displayName+"...", 0)
		if err := exif.WriteExifTagsFn(filePath, exifArgs); err != nil {
			return fail("write exif: %v", err)
		}
	}

	if cfg.DryRun {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: "dry-run: changes previewed, nothing written"}
	}

	if cfg.ExportDir != "" {
		exportName, err := exportFileName(cfg, asset, safeName)
		if err != nil {
			return fail("export: %v", err)
		}
		albumIDs := cfg.ExportAlbumIDsByAsset[assetID]
		if len(albumIDs) > 0 {
			for _, albumID := range albumIDs {
				safeAlbumID, err := safePathComponent("album ID", albumID)
				if err != nil {
					return fail("export: %v", err)
				}
				albumDir := filepath.Join(cfg.ExportDir, safeAlbumID)
				if err := os.MkdirAll(albumDir, 0755); err != nil {
					return fail("export (%s): %v", albumID, err)
				}

				if _, err := copyFileToDir(filePath, albumDir, exportName, cfg.Rename); err != nil {
					return fail("export (%s): %v", albumID, err)
				}
			}
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: fmt.Sprintf("exported to %d album folders", len(albumIDs))}
		}

		destPath, err := copyFileToDir(filePath, cfg.ExportDir, exportName, cfg.Rename)
		if err != nil {
			return fail("export: %v", err)
		}
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: fmt.Sprintf("exported to %s", destPath)}
	}

	// Metadata edited server-side while we were downloading and rewriting the
	// file would be silently overwritten by the replacement; skip instead so a
	// re-run picks up the fresh state.
	fresh, err := client.GetAsset(assetID)
	if err != nil {
		return fail("re-check asset before upload: %v", err)
	}
	if !fresh.UpdatedAt.Equal(asset.UpdatedAt) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "asset changed on the server while processing; re-run to pick up the latest metadata"}
	}
	if baked.path != "" && !slices.Equal(tagValues(*fresh), tagValues(*asset)) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "tags changed on the server while processing; re-run to pick up the latest state"}
	}
	// Face edits (renames, box moves, reassignments) do not bump updatedAt, so
	// the regions we are about to embed get their own freshness check. Skip it
	// when no region is being written: the file already matched, so a mid-run
	// face move is not ours to guard and must not block an unrelated exif write.
	if len(faceRegions) > 0 {
		stale, staleErr := faceRegionsStale(client, *asset, existing, faceRegions)
		if staleErr != nil {
			return fail("re-check faces before upload: %v", staleErr)
		}
		if stale {
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "faces changed on the server while processing; re-run to pick up the latest state"}
		}
	}

	if inPlace.local != "" {
		return commitInPlace(client, cfg, asset, filePath, inPlace, emitter)
	}
	return uploadReplacement(uploader, filePath, asset, emitter, cancelled)
}

func safePathComponent(kind, value string) (string, error) {
	base := filepath.Base(value)
	if base != value || base == "." || base == ".." {
		return "", fmt.Errorf("unsafe %s: %q", kind, value)
	}
	return base, nil
}
