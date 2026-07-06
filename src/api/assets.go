package api

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func (c *ImmichClient) GetAsset(assetID string) (*model.AssetResponse, error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+url.PathEscape(assetID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var asset model.AssetResponse
	if err := c.doJSON(req, &asset); err != nil {
		return nil, err
	}
	return &asset, nil
}

// downloadProgressWriter reports the integer transfer percentage each time it
// increases, so a callback fires at most 100 times per download.
type downloadProgressWriter struct {
	total       int64
	written     int64
	lastPercent int
	onProgress  func(percent int)
}

func (w *downloadProgressWriter) Write(b []byte) (int, error) {
	w.written += int64(len(b))
	percent := int(w.written * 100 / w.total)
	if percent > 100 {
		percent = 100
	}
	if percent > w.lastPercent {
		w.lastPercent = percent
		w.onProgress(percent)
	}
	return len(b), nil
}

func (c *ImmichClient) DownloadAsset(assetID, destPath, expectedChecksum string, onProgress func(percent int)) (err error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+url.PathEscape(assetID)+"/original", nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	watchdog := newProgressWatchdog(cancel)
	defer watchdog.Stop()
	resp, err := c.doRequest(req.WithContext(ctx))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil && err == nil {
			err = fmt.Errorf("close file: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(destPath)
		}
	}()

	hasher := sha1.New()
	dest := io.MultiWriter(f, hasher)
	if onProgress != nil && resp.ContentLength > 0 {
		dest = io.MultiWriter(f, hasher, &downloadProgressWriter{total: resp.ContentLength, onProgress: onProgress})
	}
	written, err := io.Copy(dest, watchdogReader{reader: resp.Body, watchdog: watchdog})
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("download stalled: no progress for %v", stallTimeout)
		}
		return fmt.Errorf("write file: %w", err)
	}

	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return fmt.Errorf("download truncated for asset %s: wrote %d bytes, expected %d", model.ShortID(assetID), written, resp.ContentLength)
	}

	if expectedChecksum != "" {
		want, decodeErr := model.DecodeSHA1Checksum(expectedChecksum)
		if decodeErr != nil {
			return fmt.Errorf("decode source checksum %q: %w", expectedChecksum, decodeErr)
		}
		if !bytes.Equal(hasher.Sum(nil), want) {
			return fmt.Errorf("download checksum mismatch for asset %s (file is corrupt or truncated)", model.ShortID(assetID))
		}
	}
	return nil
}

func (c *ImmichClient) UploadAsset(filePath string, asset *model.AssetResponse) (*model.UploadResponse, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	contentType := w.FormDataContentType()

	req, err := c.newRequest(http.MethodPost, "/assets", pr)
	if err != nil {
		pw.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	watchdog := newProgressWatchdog(cancel)
	req = req.WithContext(ctx)

	go func() {
		var writeErr error
		defer func() {
			// Once the body is fully written the server may legitimately take a
			// while to answer (checksumming a large upload); that phase is
			// bounded by ResponseHeaderTimeout, not the stall watchdog.
			watchdog.Stop()
			pw.CloseWithError(writeErr)
		}()

		part, err := w.CreateFormFile("assetData", filepath.Base(filePath))
		if err != nil {
			writeErr = err
			return
		}
		if _, err := io.Copy(watchdogWriter{writer: part, watchdog: watchdog}, f); err != nil {
			writeErr = err
			return
		}

		if !c.apiV3 {
			deviceAssetID := asset.DeviceAssetID
			if deviceAssetID == "" {
				deviceAssetID = "exif-merger-" + asset.ID
			}
			deviceID := asset.DeviceID
			if deviceID == "" {
				deviceID = "exif-merger"
			}
			if err := w.WriteField("deviceAssetId", deviceAssetID); err != nil {
				writeErr = err
				return
			}
			if err := w.WriteField("deviceId", deviceID); err != nil {
				writeErr = err
				return
			}
		}
		if err := w.WriteField("fileCreatedAt", asset.FileCreatedAt.Format(time.RFC3339)); err != nil {
			writeErr = err
			return
		}
		if err := w.WriteField("fileModifiedAt", asset.FileModifiedAt.Format(time.RFC3339)); err != nil {
			writeErr = err
			return
		}
		if err := w.WriteField("isFavorite", fmt.Sprintf("%t", asset.IsFavorite)); err != nil {
			writeErr = err
			return
		}
		// Preserve the live-photo pairing: without this the replacement still
		// would permanently lose its link to the motion video.
		if asset.LivePhotoVideoID != "" {
			if err := w.WriteField("livePhotoVideoId", asset.LivePhotoVideoID); err != nil {
				writeErr = err
				return
			}
		}

		if err := w.Close(); err != nil {
			writeErr = err
			return
		}
	}()

	var resp model.UploadResponse
	if err := c.doJSON(req, &resp); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("upload stalled: no progress for %v", stallTimeout)
		}
		return nil, err
	}
	return &resp, nil
}

func (c *ImmichClient) CopyAsset(sourceID, destinationID string) error {
	body := model.CopyAssetsRequest{
		SourceID:    sourceID,
		TargetID:    destinationID,
		Albums:      true,
		Favorite:    true,
		SharedLinks: true,
		Sidecar:     true,
		Stack:       true,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	// Always PUT: v3.0.1 has no PATCH alias for /assets/copy — a PATCH is
	// routed into PATCH /assets/:id with id="copy" and fails UUID validation
	// (confirmed against a live 3.0.1 server and its OpenAPI spec).
	req, err := c.newRequest(http.MethodPut, "/assets/copy", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func (c *ImmichClient) DeleteAssets(assetIDs []string, force bool) error {
	body := model.DeleteAssetsRequest{
		IDs:   assetIDs,
		Force: force,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := c.newRequest(http.MethodDelete, "/assets", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

func (c *ImmichClient) UpdateAssetVisibility(assetID, visibility string) error {
	body := model.UpdateAssetsRequest{
		IDs:        []string{assetID},
		Visibility: visibility,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := c.newRequest(c.writeMethod(), "/assets", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}
