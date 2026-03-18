package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

	if err := c.DownloadAsset("asset-123", destPath); err != nil {
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

func TestDownloadAssetReturnsErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "test-key")
	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")
	if err := c.DownloadAsset("asset-123", destPath); err == nil {
		t.Fatal("expected error")
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
	err := c.DownloadAsset("asset-1", "/nonexistent-dir/file.jpg")
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
	err := c.DownloadAsset("id", filepath.Join(t.TempDir(), "f"))
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
