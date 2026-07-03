package process

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

// Pins the full finalize sequence on a v3 server: checksum verification runs
// BEFORE associations are copied, copy stays PUT (no PATCH alias on v3.0.1),
// the visibility update uses PATCH, and the delete is a trash move.
func TestModernUploaderV3SequenceVerifiesBeforeCopy(t *testing.T) {
	content := []byte("uploaded-bytes")
	sum := sha1.Sum(content)

	var calls []string
	var deleteForce bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/server/about":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"3.0.1"}`))
		case "POST /api/assets":
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "GET /api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(model.AssetResponse{ID: "new-id", Checksum: base64.StdEncoding.EncodeToString(sum[:])})
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "PATCH /api/assets":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			var payload model.DeleteAssetsRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode delete payload: %v", err)
			}
			deleteForce = payload.Force
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := api.NewImmichClient(server.URL, "key")
	if err := client.ResolveAPIMode("auto"); err != nil {
		t.Fatalf("resolve api mode: %v", err)
	}

	uploader := &ModernUploader{Client: client, VerifyUpload: true}
	asset := &model.AssetResponse{ID: "old-id", OriginalFileName: "f.jpg", IsArchived: true, FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

	if _, err := uploader.Upload(filePath, asset, &noopEmitter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"GET /api/server/about",
		"POST /api/assets",
		"GET /api/assets/new-id",
		"PUT /api/assets/copy",
		"PATCH /api/assets",
		"DELETE /api/assets",
	}
	if len(calls) != len(want) {
		t.Fatalf("expected %d calls, got %d: %v", len(want), len(calls), calls)
	}
	for i, call := range want {
		if calls[i] != call {
			t.Fatalf("call %d: expected %q, got %q (full sequence: %v)", i, call, calls[i], calls)
		}
	}
	if deleteForce {
		t.Fatal("expected trash delete (force=false) even with verification on")
	}
}

// Duplicate resolution combined with verification: the checksum of the
// duplicate target is verified before any association is copied, and the
// original still goes to the trash.
func TestModernUploaderResolvesDuplicateWithVerification(t *testing.T) {
	content := []byte("duplicate-bytes")
	sum := sha1.Sum(content)

	var calls []string
	var deleteForce bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"dup-id","status":"duplicate"}`))
		case "GET /api/assets/dup-id":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(model.AssetResponse{ID: "dup-id", Checksum: base64.StdEncoding.EncodeToString(sum[:])})
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			var payload model.DeleteAssetsRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode delete payload: %v", err)
			}
			deleteForce = payload.Force
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := api.NewImmichClient(server.URL, "key")
	uploader := &ModernUploader{Client: client, ResolveDuplicate: true, VerifyUpload: true}
	asset := &model.AssetResponse{ID: "old-id", OriginalFileName: "f.jpg", FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome.NewID != "dup-id" {
		t.Fatalf("expected dup-id, got %s", outcome.NewID)
	}

	want := []string{
		"POST /api/assets",
		"GET /api/assets/dup-id",
		"GET /api/assets/dup-id",
		"PUT /api/assets/copy",
		"DELETE /api/assets",
	}
	if len(calls) != len(want) {
		t.Fatalf("expected %d calls, got %d: %v", len(want), len(calls), calls)
	}
	for i, call := range want {
		if calls[i] != call {
			t.Fatalf("call %d: expected %q, got %q (full sequence: %v)", i, call, calls[i], calls)
		}
	}
	if deleteForce {
		t.Fatal("expected trash delete (force=false) when resolving a duplicate")
	}
}
