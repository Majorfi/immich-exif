package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func ProcessAsset(client *api.ImmichClient, uploader Uploader, cfg *model.Config, assetID string, index, total int, emitter model.EventEmitter) model.ProcessResult {
	fail := func(msg string, args ...any) model.ProcessResult {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusFailed, Message: fmt.Sprintf("[%s] %s", model.ShortID(assetID), fmt.Sprintf(msg, args...))}
	}

	asset, err := client.GetAsset(assetID)
	if err != nil {
		return fail("fetch asset: %v", err)
	}
	if model.IsUnsupportedVideoAsset(*asset) {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "unsupported video container for metadata embedding"}
	}
	if len(exif.CollectExifArgs(exif.CompareAssetMetadata(*asset, nil))) == 0 {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSkipped, Message: "no metadata to embed"}
	}

	assetDir, err := os.MkdirTemp(cfg.TmpDir, assetID+"-*")
	if err != nil {
		return fail("create temp dir: %v", err)
	}
	defer func() {
		if !cfg.DryRun {
			os.RemoveAll(assetDir)
		}
	}()

	filePath := filepath.Join(assetDir, asset.OriginalFileName)
	if err := client.DownloadAsset(assetID, filePath); err != nil {
		return fail("download: %v", err)
	}

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

	if err := exif.WriteExifTagsFn(filePath, exifArgs); err != nil {
		return fail("write exif: %v", err)
	}

	if cfg.ExportDir != "" {
		albumIDs := cfg.ExportAlbumIDsByAsset[assetID]
		if len(albumIDs) > 0 {
			for _, albumID := range albumIDs {
				albumDir := filepath.Join(cfg.ExportDir, albumID)
				if err := os.MkdirAll(albumDir, 0755); err != nil {
					return fail("export (%s): %v", albumID, err)
				}

				destPath := filepath.Join(albumDir, asset.OriginalFileName)
				if err := copyFile(filePath, destPath); err != nil {
					return fail("export (%s): %v", albumID, err)
				}
			}
			return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: fmt.Sprintf("exported to %d album folders", len(albumIDs))}
		}

		destPath := filepath.Join(cfg.ExportDir, asset.OriginalFileName)
		if err := copyFile(filePath, destPath); err != nil {
			return fail("export: %v", err)
		}
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: fmt.Sprintf("exported to %s", destPath)}
	}

	if cfg.DryRun {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusSuccess, Message: fmt.Sprintf("dry-run: %s", filePath)}
	}

	var uploadOutcome UploadOutcome
	var uploadErr error
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		uploadOutcome, uploadErr = uploader.Upload(filePath, asset, emitter)
		if uploadErr == nil {
			break
		}
		if attempt < maxRetries {
			emitter.EmitProgress(model.ProgressEvent{
				AssetID:  assetID,
				Filename: asset.OriginalFileName,
				Step:     fmt.Sprintf("Upload failed (attempt %d/%d), retrying in 2s: %v", attempt, maxRetries, uploadErr),
			})
			time.Sleep(2 * time.Second)
		}
	}
	if uploadErr != nil {
		return fail("upload (after %d attempts): %v", maxRetries, uploadErr)
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
