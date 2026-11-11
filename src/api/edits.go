package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/majorfi/immich-exif/model"
)

var ErrEditedRenderMissing = errors.New("the server has no edited render for this asset yet")

func (c *ImmichClient) GetAssetEdits(assetID string) ([]model.AssetEdit, error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+url.PathEscape(assetID)+"/edits", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	var edits model.AssetEdits
	if err := c.doJSON(req, &edits); err != nil {
		return nil, err
	}
	return edits.Edits, nil
}

func (c *ImmichClient) ReplaceAssetEdits(assetID string, edits []model.AssetEdit) ([]model.AssetEdit, error) {
	jsonBody, err := json.Marshal(model.AssetEdits{Edits: edits})
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(http.MethodPut, "/assets/"+url.PathEscape(assetID)+"/edits", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	var applied model.AssetEdits
	if err := c.doJSON(req, &applied); err != nil {
		return nil, err
	}
	return applied.Edits, nil
}

// DownloadEditedAsset saves the server's edited render into destDir under the
// name the server gives it (the render's extension can differ from the
// original's) and returns the path. Immich silently serves the original when
// no render exists yet, so bytes matching the original's SHA-1 are refused.
func (c *ImmichClient) DownloadEditedAsset(assetID, destDir string, originalSum []byte, onProgress func(percent int)) (string, error) {
	var destPath string
	destFor := func(resp *http.Response) (string, error) {
		name, err := attachmentName(resp)
		if err != nil {
			return "", err
		}
		destPath = filepath.Join(destDir, name)
		return destPath, nil
	}
	sum, err := c.download(assetID, "/assets/"+url.PathEscape(assetID)+"/original?edited=true", destFor, onProgress)
	if err != nil {
		return "", err
	}

	if bytes.Equal(sum, originalSum) {
		_ = os.Remove(destPath)
		return "", ErrEditedRenderMissing
	}
	return destPath, nil
}

func attachmentName(resp *http.Response) (string, error) {
	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil {
		return "", fmt.Errorf("edited render has no usable file name: %w", err)
	}
	name := params["filename"]
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("edited render has an unsafe file name: %q", name)
	}
	return name, nil
}

// GetExifGeometry returns the asset's size after EXIF orientation, computed
// like Immich's getDimensions (the size its editor validates crops against and
// renders from), and the orientation Immich resolved for it, sidecar included.
// A zero size means metadata extraction has not filled it in yet.
func (c *ImmichClient) GetExifGeometry(assetID string) (int, int, string, error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+url.PathEscape(assetID), nil)
	if err != nil {
		return 0, 0, "", err
	}
	req.Header.Set("Accept", "application/json")
	var asset struct {
		ExifInfo *struct {
			ExifImageWidth  *int    `json:"exifImageWidth"`
			ExifImageHeight *int    `json:"exifImageHeight"`
			Orientation     *string `json:"orientation"`
		} `json:"exifInfo"`
	}
	if err := c.doJSON(req, &asset); err != nil {
		return 0, 0, "", err
	}
	info := asset.ExifInfo
	if info == nil {
		return 0, 0, "", nil
	}
	orientation := ""
	if info.Orientation != nil {
		orientation = *info.Orientation
	}
	if info.ExifImageWidth == nil || info.ExifImageHeight == nil {
		return 0, 0, orientation, nil
	}
	if slices.Contains([]string{"5", "6", "7", "8", "-90", "90"}, orientation) {
		return *info.ExifImageHeight, *info.ExifImageWidth, orientation, nil
	}
	return *info.ExifImageWidth, *info.ExifImageHeight, orientation, nil
}

func (c *ImmichClient) GetUneditedPreview(assetID string) ([]byte, error) {
	req, err := c.newRequest(http.MethodGet, "/assets/"+url.PathEscape(assetID)+"/thumbnail?size=preview&edited=false", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func (c *ImmichClient) ServerTime() (time.Time, error) {
	req, err := c.newRequest(http.MethodGet, "/server/ping", nil)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := c.doRequest(req)
	if err != nil {
		return time.Time{}, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return http.ParseTime(resp.Header.Get("Date"))
}
