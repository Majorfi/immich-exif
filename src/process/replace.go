package process

import (
	"errors"
	"fmt"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func uploadReplacement(uploader Uploader, filePath string, asset *model.AssetResponse, emitter model.EventEmitter, cancelled func() bool) model.ProcessResult {
	assetID := asset.ID
	fail := func(message string) model.ProcessResult {
		return model.ProcessResult{AssetID: assetID, Status: model.StatusFailed, Message: fmt.Sprintf("[%s] %s", model.ShortID(assetID), message)}
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
		result := fail(fmt.Sprintf("upload (after %d attempts): %v", attempt, uploadErr))
		if errors.Is(uploadErr, errKeyPermission) {
			result.StopReason = "run stopped: the API key is missing a permission every remaining asset needs"
		}
		return result
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
