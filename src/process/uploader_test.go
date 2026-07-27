package process

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// videoReplaceServer handles a full created-status replace for a video asset
// whose old copy has one assigned face, answering POST /api/faces with
// facePostStatus so the caller can drive the skip vs fail hook branches.
func videoReplaceServer(t *testing.T, calls *[]string, facePostStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-asset-id","status":"created"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/faces":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("id") == "old-asset-id" {
				_ = json.NewEncoder(w).Encode([]model.AssetFaceResponse{
					{BoundingBoxX1: 10, BoundingBoxY1: 20, BoundingBoxX2: 110, BoundingBoxY2: 140, ImageWidth: 640, ImageHeight: 360, Person: &model.PersonResponse{ID: "p1"}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode([]model.AssetFaceResponse{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/faces":
			w.WriteHeader(facePostStatus)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func videoAsset() *model.AssetResponse {
	return &model.AssetResponse{
		ID:               "old-asset-id",
		OriginalFileName: "clip.mp4",
		OriginalMimeType: "video/mp4",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
	}
}

func TestModernUploaderVideoTooOldForFacesSkipsAndStillTrashes(t *testing.T) {
	var calls []string
	server := videoReplaceServer(t, &calls, http.StatusNotFound)
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "clip.mp4")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), Faces: true}
	if _, err := uploader.Upload(filePath, videoAsset(), &noopEmitter{}); err != nil {
		t.Fatalf("a 404 from the faces endpoint must not fail the replace: %v", err)
	}
	if !slices.Contains(calls, "POST /api/faces") {
		t.Fatal("expected a face-create attempt for a video with -faces")
	}
	if !slices.Contains(calls, "DELETE /api/assets") {
		t.Fatal("old asset must still be trashed when face preservation is unsupported")
	}
}

func TestModernUploaderVideoFacePermissionDeniedDoesNotTrash(t *testing.T) {
	var calls []string
	server := videoReplaceServer(t, &calls, http.StatusForbidden)
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "clip.mp4")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), Faces: true}
	_, err := uploader.Upload(filePath, videoAsset(), &noopEmitter{})
	if err == nil {
		t.Fatal("a face.create permission denial must fail the replace")
	}
	if slices.Contains(calls, "DELETE /api/assets") {
		t.Fatal("old asset must NOT be trashed when face preservation is denied")
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
		case "GET /api/assets/dup-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"dup-id","isTrashed":false}`))
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
	if len(calls) != 4 {
		t.Fatalf("expected upload, trash-check, copy, delete calls, got %d: %v", len(calls), calls)
	}
}

func TestModernUploaderRefusesTrashedDuplicate(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"dup-id","status":"duplicate"}`))
		case "GET /api/assets/dup-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"dup-id","isTrashed":true}`))
		case "DELETE /api/assets":
			deleteCalled = true
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

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected error when the duplicate target is trashed")
	}
	if !strings.Contains(err.Error(), "trash") {
		t.Fatalf("expected trash-related error, got: %v", err)
	}
	var nonRetry *nonRetryableError
	if !errors.As(err, &nonRetry) {
		t.Fatalf("expected nonRetryable error, got: %v", err)
	}
	if deleteCalled {
		t.Fatal("the original must NOT be deleted when the duplicate is trashed")
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

func TestModernUploaderRefusesDuplicateLackingLivePhotoLink(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"dup-id","status":"duplicate"}`))
		case "GET /api/assets/dup-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"dup-id","isTrashed":false}`))
		case "DELETE /api/assets":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "still.heic")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := api.NewImmichClient(server.URL, "test-key")
	uploader := &ModernUploader{Client: client, ResolveDuplicate: true}
	asset := &model.AssetResponse{
		ID:               "old-id",
		LivePhotoVideoID: "motion-id",
		FileCreatedAt:    time.Now(),
		FileModifiedAt:   time.Now(),
	}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected error when the duplicate lacks the live-photo link")
	}
	if !strings.Contains(err.Error(), "live-photo") {
		t.Fatalf("expected live-photo error, got: %v", err)
	}
	var nonRetry *nonRetryableError
	if !errors.As(err, &nonRetry) {
		t.Fatalf("expected nonRetryable error, got: %v", err)
	}
	if deleteCalled {
		t.Fatal("the original must NOT be deleted when resolving would sever a live-photo pair")
	}
}
