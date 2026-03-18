package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

func TestModernUploaderRestoresHiddenVisibilityBeforeDelete(t *testing.T) {
	var calls []string
	visibilityUpdated := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-asset-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "PUT /api/assets":
			var payload model.UpdateAssetsRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode update payload: %v", err)
			}
			if payload.Visibility != "hidden" {
				t.Fatalf("expected hidden visibility, got %q", payload.Visibility)
			}
			visibilityUpdated = true
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	filePath := writeUploaderFixture(t)
	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:               "old-asset-id",
		DeviceAssetID:    "device-asset-id",
		DeviceID:         "device-id",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
		Visibility:       "hidden",
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}
	if outcome.NewID != "new-asset-id" {
		t.Fatalf("expected new id new-asset-id, got %s", outcome.NewID)
	}
	if !visibilityUpdated {
		t.Fatal("expected hidden visibility update request")
	}
	if len(calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(calls), calls)
	}
}

func TestModernUploaderTimelineVisibilitySkipsVisibilityUpdate(t *testing.T) {
	var calls []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	filePath := writeUploaderFixture(t)
	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:               "old-id",
		DeviceAssetID:    "device-asset-id",
		DeviceID:         "device-id",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
		Visibility:       "timeline",
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.NewID != "new-id" {
		t.Fatalf("expected new-id, got %s", outcome.NewID)
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(calls), calls)
	}
}

func writeUploaderFixture(t *testing.T) string {
	t.Helper()
	filePath := filepath.Join(t.TempDir(), "asset.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return filePath
}
