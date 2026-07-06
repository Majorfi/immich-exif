package api

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func TestGetAssetSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/assets/asset-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("missing api key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"asset-123","originalFileName":"photo.jpg"}`))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	asset, err := c.GetAsset("asset-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset.ID != "asset-123" {
		t.Fatalf("expected id asset-123, got %s", asset.ID)
	}
	if asset.OriginalFileName != "photo.jpg" {
		t.Fatalf("expected filename photo.jpg, got %s", asset.OriginalFileName)
	}
}

func TestGetAssetReturnsErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	_, err := c.GetAsset("missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadAssetWritesFile(t *testing.T) {
	content := "fake-image-data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assets/asset-123/original" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")

	if err := c.DownloadAsset("asset-123", destPath, "", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("expected %q, got %q", content, string(got))
	}
}

func TestDownloadAssetReportsProgress(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 200_000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		_, _ = w.Write(content)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")

	var percents []int
	err := c.DownloadAsset("asset-123", destPath, "", func(percent int) {
		percents = append(percents, percent)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(percents) == 0 {
		t.Fatal("expected progress callbacks for a sized download")
	}
	for i := 1; i < len(percents); i++ {
		if percents[i] <= percents[i-1] {
			t.Fatalf("expected strictly increasing percentages, got %v", percents)
		}
	}
	if percents[len(percents)-1] != 100 {
		t.Fatalf("expected the final callback to report 100%%, got %v", percents)
	}
}

func TestDownloadAssetVerifiesChecksumMatch(t *testing.T) {
	content := []byte("fake-image-data")
	sum := sha1.Sum(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")

	if err := c.DownloadAsset("asset-123", destPath, base64.StdEncoding.EncodeToString(sum[:]), nil); err != nil {
		t.Fatalf("unexpected error for matching checksum: %v", err)
	}
}

func TestDownloadAssetRejectsChecksumMismatch(t *testing.T) {
	wrong := sha1.Sum([]byte("not-what-was-served"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake-image-data"))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")

	err := c.DownloadAsset("asset-123", destPath, base64.StdEncoding.EncodeToString(wrong[:]), nil)
	if err == nil {
		t.Fatal("expected error for checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got: %v", err)
	}
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Fatal("corrupt download must not be left on disk")
	}
}

func TestDownloadAssetReturnsErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")
	if err := c.DownloadAsset("asset-123", destPath, "", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestUploadAssetV3OmitsDeviceFields(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewImmichClient(server.URL, "test-key")
	c.apiV3 = true
	asset := &model.AssetResponse{
		ID:             "asset-id",
		DeviceAssetID:  "device-asset",
		DeviceID:       "device-1",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	if _, err := c.UploadAsset(filePath, asset); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := string(receivedBody)
	if strings.Contains(body, `name="deviceAssetId"`) || strings.Contains(body, `name="deviceId"`) {
		t.Fatal("v3 upload must not send deviceAssetId/deviceId fields")
	}
	if !strings.Contains(body, `name="fileCreatedAt"`) {
		t.Fatal("expected fileCreatedAt still present in v3 upload")
	}
}

func TestUploadAssetSendsMultipartForm(t *testing.T) {
	var receivedContentType string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/assets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-asset","status":"created"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "photo.jpg")
	if err := os.WriteFile(filePath, []byte("image-data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewImmichClient(server.URL, "test-key")
	asset := &model.AssetResponse{
		ID:               "old-id",
		DeviceAssetID:    "device-asset",
		DeviceID:         "device-1",
		OriginalFileName: "photo.jpg",
		FileCreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		FileModifiedAt:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
		IsFavorite:       true,
	}

	resp, err := c.UploadAsset(filePath, asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "new-asset" {
		t.Fatalf("expected new-asset, got %s", resp.ID)
	}

	if !strings.Contains(receivedContentType, "multipart/form-data") {
		t.Fatalf("expected multipart content type, got %s", receivedContentType)
	}
	body := string(receivedBody)
	if !strings.Contains(body, "image-data") {
		t.Fatal("expected file data in body")
	}
	if !strings.Contains(body, "device-asset") {
		t.Fatal("expected deviceAssetId in body")
	}
}

func TestUploadAssetStreamsMultipartBody(t *testing.T) {
	var contentLength int64
	var transferEncoding []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentLength = r.ContentLength
		transferEncoding = append([]string{}, r.TransferEncoding...)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-asset","status":"created"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "photo.jpg")
	if err := os.WriteFile(filePath, []byte(strings.Repeat("x", 64*1024)), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewImmichClient(server.URL, "test-key")
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now().UTC(),
		FileModifiedAt: time.Now().UTC(),
	}

	resp, err := c.UploadAsset(filePath, asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "new-asset" {
		t.Fatalf("expected new-asset, got %s", resp.ID)
	}
	if contentLength != -1 {
		t.Fatalf("expected unknown content length for streamed upload, got %d", contentLength)
	}
	if len(transferEncoding) == 0 || transferEncoding[0] != "chunked" {
		t.Fatalf("expected chunked transfer encoding, got %v", transferEncoding)
	}
}

func TestUploadAssetFallbackDeviceFields(t *testing.T) {
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "photo.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewImmichClient(server.URL, "test-key")
	asset := &model.AssetResponse{
		ID:             "asset-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	_, err := c.UploadAsset(filePath, asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := string(receivedBody)
	if !strings.Contains(body, "exif-merger-asset-id") {
		t.Fatal("expected fallback deviceAssetId containing exif-merger-<id>")
	}
	if !strings.Contains(body, "exif-merger") {
		t.Fatal("expected fallback deviceId exif-merger")
	}
}

func TestCopyAssetSendsCorrectPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/assets/copy" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body model.CopyAssetsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.SourceID != "src" || body.TargetID != "dst" {
			t.Fatalf("unexpected source/target: %s/%s", body.SourceID, body.TargetID)
		}
		if !body.Albums || !body.Favorite || !body.SharedLinks || !body.Sidecar || !body.Stack {
			t.Fatal("expected all copy flags to be true")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	if err := c.CopyAsset("src", "dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// /assets/copy has no PATCH alias on v3.0.1 (a PATCH is routed into
// PATCH /assets/:id and fails UUID validation), so copy must stay PUT even
// when the client runs in v3 mode.
func TestCopyAssetAlwaysUsesPut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT for /assets/copy, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	c.apiV3 = true
	if err := c.CopyAsset("src", "dst"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteAssetsSendsCorrectPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/assets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body model.DeleteAssetsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.IDs) != 2 || body.IDs[0] != "a" || body.IDs[1] != "b" {
			t.Fatalf("unexpected IDs: %v", body.IDs)
		}
		if !body.Force {
			t.Fatal("expected force=true")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	if err := c.DeleteAssets([]string{"a", "b"}, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadAssetCreateFileError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	err := c.DownloadAsset("asset-1", "/nonexistent-dir/file.jpg", "", nil)
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
	if !strings.Contains(err.Error(), "create file") {
		t.Fatalf("expected create file error, got: %v", err)
	}
}

func TestUploadAssetFileNotFound(t *testing.T) {
	c := NewImmichClient("http://localhost", "key")
	asset := &model.AssetResponse{
		ID:             "id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}
	_, err := c.UploadAsset("/nonexistent/photo.jpg", asset)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "open file") {
		t.Fatalf("expected open file error, got: %v", err)
	}
}

func TestDoRequestConnectionError(t *testing.T) {
	c := NewImmichClient("http://127.0.0.1:1", "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	_, err := c.doRequest(req)
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestGetAssetInvalidURL(t *testing.T) {
	c := NewImmichClient("://invalid", "key")
	_, err := c.GetAsset("id")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadAssetInvalidURL(t *testing.T) {
	c := NewImmichClient("://invalid", "key")
	err := c.DownloadAsset("id", filepath.Join(t.TempDir(), "f"), "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUploadAssetServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "photo.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	c := NewImmichClient(server.URL, "key")
	asset := &model.AssetResponse{
		ID:             "id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}
	_, err := c.UploadAsset(filePath, asset)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCopyAssetError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	err := c.CopyAsset("src", "dst")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeleteAssetsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	err := c.DeleteAssets([]string{"a"}, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateAssetVisibilityError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	err := c.UpdateAssetVisibility("id", "archive")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAlbumAssetsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	_, err := c.GetAlbumAssets("album-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateAssetVisibilitySendsCorrectPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/assets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body model.UpdateAssetsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.IDs) != 1 || body.IDs[0] != "asset-1" {
			t.Fatalf("unexpected IDs: %v", body.IDs)
		}
		if body.Visibility != "archive" {
			t.Fatalf("expected visibility archive, got %s", body.Visibility)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	if err := c.UpdateAssetVisibility("asset-1", "archive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateAssetVisibilityUsesPatchOnV3(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH on v3, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	c.apiV3 = true
	if err := c.UpdateAssetVisibility("asset-1", "archive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUploadAssetForwardsLivePhotoVideoID(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "photo.heic")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewImmichClient(server.URL, "test-key")
	c.apiV3 = true
	asset := &model.AssetResponse{
		ID:               "asset-id",
		LivePhotoVideoID: "motion-id",
		FileCreatedAt:    time.Now(),
		FileModifiedAt:   time.Now(),
	}

	if _, err := c.UploadAsset(filePath, asset); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body := string(receivedBody)
	if !strings.Contains(body, `name="livePhotoVideoId"`) || !strings.Contains(body, "motion-id") {
		t.Fatal("expected livePhotoVideoId to be forwarded so the pair survives replacement")
	}
}

func TestUploadAssetOmitsEmptyLivePhotoVideoID(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	c := NewImmichClient(server.URL, "test-key")
	c.apiV3 = true
	asset := &model.AssetResponse{ID: "asset-id", FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

	if _, err := c.UploadAsset(filePath, asset); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(receivedBody), `name="livePhotoVideoId"`) {
		t.Fatal("must not send an empty livePhotoVideoId field")
	}
}

func TestGetAssetEscapesIDInPath(t *testing.T) {
	var escapedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escapedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"weird"}`))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	if _, err := c.GetAsset("weird/../id?x=1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if escapedPath != "/api/assets/weird%2F..%2Fid%3Fx=1" {
		t.Fatalf("expected the ID to be path-escaped, server saw: %s", escapedPath)
	}
}

func TestDownloadAssetFailsOnStalledBody(t *testing.T) {
	origStall := stallTimeout
	stallTimeout = 100 * time.Millisecond
	defer func() { stallTimeout = origStall }()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("partial-bytes"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-release
	}))
	defer server.Close()
	// LIFO: release the blocked handler BEFORE server.Close() waits on it.
	defer close(release)

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "stalled.jpg")
	start := time.Now()
	err := c.DownloadAsset("asset-1", destPath, "", nil)
	if err == nil {
		t.Fatal("expected error for a stalled download")
	}
	if !strings.Contains(err.Error(), "stalled") {
		t.Fatalf("expected stall error, got: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("stall detection took too long: %v", elapsed)
	}
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Fatal("stalled download must not leave a partial file behind")
	}
}

func TestFeaturesReportsTrash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/server/features" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"trash":false,"smartSearch":true}`))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	features, err := c.Features()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if features.Trash {
		t.Fatal("expected trash=false")
	}
}
