package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	// (dry-run, export) are safe. libraryId means external only on servers
	// where it is nullable (1.106+) — before that every asset carried one.
	if cfg.WillReplace() && client.CanDetectExternalLibrary() && asset.LibraryID != "" {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "asset belongs to an external library; replacing it would migrate it into the internal library (dry-run and export still work)"}
	}
	if model.IsLivePhotoMotionCandidate(*asset) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "hidden video (likely a live-photo motion part); replacing it would sever the pair"}
	}
	if model.IsUnsupportedVideoAsset(*asset) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "unsupported video container for metadata embedding"}
	}
	if len(exif.CollectExifArgs(exif.CompareAssetMetadata(*asset, nil))) == 0 {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "no metadata to embed"}
	}
	if strings.TrimSpace(asset.Checksum) == "" {
		return fail("server returned no checksum for this asset; refusing to replace without integrity verification")
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
	if err := client.DownloadAsset(assetID, filePath, asset.Checksum, func(percent int) {
		stepProgress("Downloading "+displayName+"...", percent)
	}); err != nil {
		return fail("download: %v", err)
	}

	stepProgress("Analyzing "+displayName+"...", 0)
	var existing exif.ExifTagMap
	existing, err = exif.ReadExifTagsFn(filePath)
	if err != nil {
		return fail("read exif: %v", err)
	}

	changes := exif.CompareAssetMetadata(*asset, existing)
	exifArgs := exif.CollectExifArgs(changes)
	if len(exifArgs) == 0 {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "metadata already matches", ExifMatched: true}
	}

	diffEntries := exif.CollectDiffEntries(changes)
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

	stepProgress("Writing tags to "+displayName+"...", 0)
	if err := exif.WriteExifTagsFn(filePath, exifArgs); err != nil {
		return fail("write exif: %v", err)
	}

	if cfg.DryRun {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: "dry-run: changes previewed, nothing written"}
	}

	if cfg.ExportDir != "" {
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

				destPath := filepath.Join(albumDir, safeName)
				if err := copyFile(filePath, destPath); err != nil {
					return fail("export (%s): %v", albumID, err)
				}
			}
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: fmt.Sprintf("exported to %d album folders", len(albumIDs))}
		}

		destPath := filepath.Join(cfg.ExportDir, safeName)
		if err := copyFile(filePath, destPath); err != nil {
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

	var uploadOutcome UploadOutcome
	var uploadErr error
	maxRetries := 3
	attempt := 0
	for attempt < maxRetries {
		attempt++
		uploadOutcome, uploadErr = uploader.Upload(filePath, asset, emitter)
		if uploadErr == nil {
			break
		}
		var nonRetry *nonRetryableError
		if errors.As(uploadErr, &nonRetry) {
			break
		}
		if attempt < maxRetries {
			if cancelled != nil && cancelled() {
				return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "user cancelled", Cancelled: true}
			}
			emitter.EmitProgress(model.ProgressEvent{
				AssetID:  assetID,
				Filename: asset.OriginalFileName,
				Step:     fmt.Sprintf("Upload failed (attempt %d/%d), retrying in 2s: %v", attempt, maxRetries, uploadErr),
			})
			sleepFn(2 * time.Second)
			if cancelled != nil && cancelled() {
				return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "user cancelled", Cancelled: true}
			}
		}
	}
	if uploadErr != nil {
		return fail("upload (after %d attempts): %v", attempt, uploadErr)
	}

	if !uploadOutcome.Cacheable {
		msg := uploadOutcome.Message
		if msg == "" {
			msg = "upload completed without copy/delete; not cached"
		}
		return model.ProcessResult{
			AssetID:     assetID,
			Status:      model.StatusSkipped,
			Message:     msg,
			DuplicateID: uploadOutcome.DuplicateID,
		}
	}

	newID := uploadOutcome.NewID
	if newID == "" {
		return fail("upload returned empty asset ID")
	}

	msg := uploadOutcome.Message
	if msg == "" {
		msg = fmt.Sprintf("uploaded (new ID: %s)", newID)
		if newID == assetID {
			msg = "replaced in-place"
		}
	}
	return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, NewID: newID, Message: msg}
}

func safePathComponent(kind, value string) (string, error) {
	base := filepath.Base(value)
	if base != value || base == "." || base == ".." {
		return "", fmt.Errorf("unsafe %s: %q", kind, value)
	}
	return base, nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("destination exists: %s", dst)
		}
		return err
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(dst)
		return err
	}
	if err := dstFile.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
