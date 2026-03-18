package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func (c *ImmichClient) GetAsset(assetID string) (*model.AssetResponse, error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+assetID, nil)
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

func (c *ImmichClient) DownloadAsset(assetID, destPath string) (err error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+assetID+"/original", nil)
	if err != nil {
		return err
	}
	resp, err := c.doRequest(req)
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

	if _, err = io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file: %w", err)
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

	deviceAssetID := asset.DeviceAssetID
	if deviceAssetID == "" {
		deviceAssetID = "exif-merger-" + asset.ID
	}
	deviceID := asset.DeviceID
	if deviceID == "" {
		deviceID = "exif-merger"
	}

	req, err := c.newRequest(http.MethodPost, "/assets", pr)
	if err != nil {
		pw.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	go func() {
		var writeErr error
		defer func() {
			pw.CloseWithError(writeErr)
		}()

		part, err := w.CreateFormFile("assetData", filepath.Base(filePath))
		if err != nil {
			writeErr = err
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			writeErr = err
			return
		}

		if err := w.WriteField("deviceAssetId", deviceAssetID); err != nil {
			writeErr = err
			return
		}
		if err := w.WriteField("deviceId", deviceID); err != nil {
			writeErr = err
			return
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

		if err := w.Close(); err != nil {
			writeErr = err
			return
		}
	}()

	var resp model.UploadResponse
	if err := c.doJSON(req, &resp); err != nil {
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
	req, err := c.newRequest(http.MethodPut, "/assets/copy", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
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
	req, err := c.newRequest(http.MethodPut, "/assets", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doRequest(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
