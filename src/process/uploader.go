package process

import (
	"fmt"
	"strings"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

type Uploader interface {
	Upload(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error)
}

type UploadOutcome struct {
	NewID          string
	Message        string
	Cacheable      bool
	RejectedReason string
	DuplicateID    string
}

type ModernUploader struct {
	Client           *api.ImmichClient
	ResolveDuplicate bool
}

func (u *ModernUploader) Upload(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: "Uploading new asset..."})
	uploadResp, err := u.Client.UploadAsset(filePath, asset)
	if err != nil {
		return UploadOutcome{}, fmt.Errorf("upload: %w", err)
	}
	newID := uploadResp.ID
	status := normalizeUploadStatus(uploadResp.Status)

	if newID == "" {
		return UploadOutcome{}, fmt.Errorf("upload returned empty asset ID (status: %q)", uploadResp.Status)
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Upload status: %s (asset ID: %s)", status, newID)})

	if newID == asset.ID {
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: "Upload resolved to the original asset ID, skipping copy/delete"})
		return UploadOutcome{NewID: newID, Message: "replaced in-place", Cacheable: true}, nil
	}

	if status == "duplicate" {
		if u.ResolveDuplicate && newID != asset.ID {
			emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Duplicate detected. Resolving by moving associations to %s and deleting old asset...", newID)})
			if err := u.finalizeReplacement(asset, newID, emitter); err != nil {
				return UploadOutcome{}, err
			}
			return UploadOutcome{
				NewID:     newID,
				Message:   fmt.Sprintf("resolved duplicate to existing asset %s", newID),
				Cacheable: true,
			}, nil
		}
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Upload rejected. Reason: %s (duplicate asset ID: %s)", status, newID)})
		return UploadOutcome{
			NewID:          newID,
			Message:        fmt.Sprintf("upload status=%s (asset ID: %s), skipped copy/delete", status, model.ShortID(newID)),
			Cacheable:      false,
			RejectedReason: status,
			DuplicateID:    newID,
		}, nil
	}

	if status == "replaced" {
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Upload returned status: %s (asset ID: %s), skipping copy/delete", status, newID)})
		return UploadOutcome{
			NewID:          newID,
			Message:        fmt.Sprintf("upload status=%s (asset ID: %s), skipped copy/delete", status, model.ShortID(newID)),
			Cacheable:      false,
			RejectedReason: status,
		}, nil
	}

	if status != "created" {
		return UploadOutcome{}, fmt.Errorf("unexpected upload status %q; no copy/delete performed", uploadResp.Status)
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("New asset ID: %s", newID)})
	if err := u.finalizeReplacement(asset, newID, emitter); err != nil {
		return UploadOutcome{}, err
	}

	return UploadOutcome{NewID: newID, Cacheable: true}, nil
}

func (u *ModernUploader) finalizeReplacement(asset *model.AssetResponse, targetID string, emitter model.EventEmitter) error {
	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Copying associations from %s to %s...", model.ShortID(asset.ID), model.ShortID(targetID))})
	if err := u.Client.CopyAsset(asset.ID, targetID); err != nil {
		return fmt.Errorf("copy associations failed (target asset %s exists but old %s NOT deleted): %w", targetID, asset.ID, err)
	}

	visibility := desiredVisibility(asset)
	if visibility != "" {
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Restoring %s visibility on %s...", visibility, model.ShortID(targetID))})
		if err := u.Client.UpdateAssetVisibility(targetID, visibility); err != nil {
			return fmt.Errorf("restore %s visibility failed (target asset %s exists but old %s NOT deleted): %w", visibility, targetID, asset.ID, err)
		}
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Deleting old asset %s...", model.ShortID(asset.ID))})
	if err := u.Client.DeleteAssets([]string{asset.ID}, true); err != nil {
		msg := fmt.Sprintf("WARNING: failed to delete old asset %s: %v (target asset %s is live)", model.ShortID(asset.ID), err, model.ShortID(targetID))
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: msg})
		return fmt.Errorf("delete old asset failed (target asset %s is live but old %s was not deleted): %w", targetID, asset.ID, err)
	}
	return nil
}

func normalizeUploadStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func desiredVisibility(asset *model.AssetResponse) string {
	if asset == nil {
		return ""
	}

	visibility := normalizeUploadStatus(asset.Visibility)
	if visibility != "" && visibility != "timeline" {
		return visibility
	}
	if asset.IsArchived {
		return "archive"
	}
	return ""
}
