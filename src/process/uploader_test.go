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

type noopEmitter struct{}

func (e *noopEmitter) EmitProgress(event model.ProgressEvent) {}
func (e *noopEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	return model.ActionConfirm
}
func (e *noopEmitter) EmitAllDone(event model.AllDoneEvent) {}

func TestModernUploaderArchivedAssetRestoresVisibilityBeforeDelete(t *testing.T) {
	var calls []string
	archiveUpdated := false

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
			if len(payload.IDs) != 1 || payload.IDs[0] != "new-asset-id" {
				t.Fatalf("unexpected ids payload: %#v", payload.IDs)
			}
			if payload.Visibility != "archive" {
				t.Fatalf("expected visibility archive, got %q", payload.Visibility)
			}
			archiveUpdated = true
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:               "old-asset-id",
		DeviceAssetID:    "device-asset-id",
		DeviceID:         "device-id",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
		IsArchived:       true,
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}
	if outcome.NewID != "new-asset-id" {
		t.Fatalf("expected new id new-asset-id, got %s", outcome.NewID)
	}
	if !outcome.Cacheable {
		t.Fatal("expected cacheable outcome")
	}
	if !archiveUpdated {
		t.Fatalf("expected archived visibility update request")
	}
	if len(calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(calls), calls)
	}
	if calls[0] != "POST /api/assets" || calls[1] != "PUT /api/assets/copy" || calls[2] != "PUT /api/assets" || calls[3] != "DELETE /api/assets" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestModernUploaderDoesNotDeleteOldAssetWhenArchiveVisibilityUpdateFails(t *testing.T) {
	var calls []string

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
			http.Error(w, "failed update", http.StatusInternalServerError)
		case "DELETE /api/assets":
			t.Fatalf("delete should not be called when archive visibility update fails")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:               "old-asset-id",
		DeviceAssetID:    "device-asset-id",
		DeviceID:         "device-id",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
		IsArchived:       true,
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatalf("expected upload error")
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls before failure, got %d: %v", len(calls), calls)
	}
	if calls[0] != "POST /api/assets" || calls[1] != "PUT /api/assets/copy" || calls[2] != "PUT /api/assets" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestNormalizeUploadStatus(t *testing.T) {
	testCases := []struct {
		input string
		want  string
	}{
		{"Created", "created"},
		{"  DUPLICATE  ", "duplicate"},
		{"replaced", "replaced"},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeUploadStatus(tc.input)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestModernUploaderSkipsCopyDeleteWhenSameID(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "POST" && r.URL.Path == "/api/assets" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"same-id","status":"created"}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "same-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.NewID != "same-id" {
		t.Fatalf("expected same-id, got %s", outcome.NewID)
	}
	if !outcome.Cacheable {
		t.Fatal("expected cacheable outcome")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call (upload only), got %d: %v", len(calls), calls)
	}
}

func TestModernUploaderSkipsCopyDeleteForDuplicateStatus(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "POST" && r.URL.Path == "/api/assets" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"dup-id","status":"duplicate"}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.NewID != "dup-id" {
		t.Fatalf("expected dup-id, got %s", outcome.NewID)
	}
	if outcome.Cacheable {
		t.Fatal("expected non-cacheable duplicate outcome")
	}
	if outcome.DuplicateID != "dup-id" {
		t.Fatalf("expected duplicate ID dup-id, got %s", outcome.DuplicateID)
	}
	if outcome.Message == "" {
		t.Fatal("expected duplicate outcome message")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(calls), calls)
	}
}

func TestModernUploaderReturnsErrorForEmptyID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"","status":"created"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestModernUploaderReturnsErrorForUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-id","status":"weird"}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected error for unexpected status")
	}
}

func TestModernUploaderNonArchivedSkipsVisibilityUpdate(t *testing.T) {
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

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:               "old-id",
		DeviceAssetID:    "dev-asset",
		DeviceID:         "dev",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now(),
		FileModifiedAt:   time.Now(),
		IsArchived:       false,
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.NewID != "new-id" {
		t.Fatalf("expected new-id, got %s", outcome.NewID)
	}
	if !outcome.Cacheable {
		t.Fatal("expected cacheable outcome")
	}
	if len(calls) != 3 {
		t.Fatalf("expected 3 calls (upload, copy, delete), got %d: %v", len(calls), calls)
	}
}

func TestModernUploaderCopyFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			http.Error(w, "copy failed", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected error for copy failure")
	}
}

func TestModernUploaderDeleteFailureReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			http.Error(w, "delete failed", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected delete failure error")
	}
}

func TestModernUploaderReplacedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/assets" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"replaced-id","status":"replaced"}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.NewID != "replaced-id" {
		t.Fatalf("expected replaced-id, got %s", outcome.NewID)
	}
	if outcome.Cacheable {
		t.Fatal("expected non-cacheable replaced outcome")
	}
	if outcome.DuplicateID != "" {
		t.Fatalf("expected empty duplicate ID for replaced status, got %s", outcome.DuplicateID)
	}
	if outcome.Message == "" {
		t.Fatal("expected replaced outcome message")
	}
}

func TestModernUploaderResolvesDuplicateWhenEnabled(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"dup-id","status":"duplicate"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client, ResolveDuplicate: true}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcome.Cacheable {
		t.Fatal("expected cacheable outcome when resolve-duplicate is enabled")
	}
	if outcome.NewID != "dup-id" {
		t.Fatalf("expected resolved ID dup-id, got %s", outcome.NewID)
	}
	if len(calls) != 3 {
		t.Fatalf("expected upload, copy, delete calls, got %d: %v", len(calls), calls)
	}
}

func TestModernUploaderUploadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "asset.jpg")
	os.WriteFile(filePath, []byte("data"), 0644)

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client}
	asset := &model.AssetResponse{
		ID:             "old-id",
		FileCreatedAt:  time.Now(),
		FileModifiedAt: time.Now(),
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected upload error")
	}
}
