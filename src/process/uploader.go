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
	VerifyUpload     bool
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
		return UploadOutcome{}, nonRetryable(fmt.Errorf("upload returned empty asset ID (status: %q)", uploadResp.Status))
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Upload status: %s (asset ID: %s)", status, newID)})

	if newID == asset.ID {
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: "Upload resolved to the original asset ID, skipping copy/delete"})
		return UploadOutcome{NewID: newID, Message: "replaced in-place", Cacheable: true}, nil
	}

	if status == "duplicate" {
		if u.ResolveDuplicate && newID != asset.ID {
			emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Duplicate detected. Resolving by moving associations to %s and deleting old asset...", newID)})
			if err := u.finalizeReplacement(filePath, asset, newID, emitter); err != nil {
				return UploadOutcome{}, nonRetryable(err)
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
		return UploadOutcome{}, nonRetryable(fmt.Errorf("unexpected upload status %q; no copy/delete performed", uploadResp.Status))
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("New asset ID: %s", newID)})
	if err := u.finalizeReplacement(filePath, asset, newID, emitter); err != nil {
		return UploadOutcome{}, nonRetryable(err)
	}

	return UploadOutcome{NewID: newID, Cacheable: true}, nil
}

func (u *ModernUploader) finalizeReplacement(filePath string, asset *model.AssetResponse, targetID string, emitter model.EventEmitter) error {
	if u.VerifyUpload {
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: fmt.Sprintf("Verifying uploaded asset %s checksum...", model.ShortID(targetID))})
		if err := verifyUploadedChecksum(u.Client, filePath, targetID); err != nil {
			return fmt.Errorf("upload verification failed (old asset %s NOT deleted, new asset %s left in place): %w", model.ShortID(asset.ID), model.ShortID(targetID), err)
		}
	}

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

	permanent := u.VerifyUpload
	deleteStep := fmt.Sprintf("Moving old asset %s to trash...", model.ShortID(asset.ID))
	if permanent {
		deleteStep = fmt.Sprintf("Deleting old asset %s...", model.ShortID(asset.ID))
	}
	emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: deleteStep})
	if err := u.Client.DeleteAssets([]string{asset.ID}, permanent); err != nil {
		msg := fmt.Sprintf("WARNING: failed to delete old asset %s: %v (target asset %s is live)", model.ShortID(asset.ID), err, model.ShortID(targetID))
		emitter.EmitProgress(model.ProgressEvent{AssetID: asset.ID, Filename: asset.OriginalFileName, Step: msg})
		return fmt.Errorf("delete old asset failed (target asset %s is live but old %s was not deleted): %w", targetID, asset.ID, err)
	}
	return nil
}

// nonRetryableError marks a failure that happened after a new asset was already
// created on the server; retrying the whole upload would risk duplicates or an
// orphaned asset, so the pipeline must fail loudly instead of re-running it.
type nonRetryableError struct{ err error }

func (e *nonRetryableError) Error() string { return e.err.Error() }
func (e *nonRetryableError) Unwrap() error { return e.err }

func nonRetryable(err error) error { return &nonRetryableError{err: err} }

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
