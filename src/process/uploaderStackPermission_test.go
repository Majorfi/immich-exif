package process

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
)

func TestModernUploaderStackPromotionServerErrorDoesNotBlamePermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"new-id","stack":{"id":"stack-id","primaryAssetId":"old-id"}}`))
		case "PUT /api/stacks/stack-id":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	_, err := uploader.Upload(writeUploaderFixture(t), stackedAsset(""), &noopEmitter{})
	if err == nil {
		t.Fatal("expected an error when the stack promotion fails")
	}
	if strings.Contains(err.Error(), "stack.update") {
		t.Fatalf("a 500 must not be diagnosed as a missing permission, got: %v", err)
	}
	if errors.Is(err, errKeyPermission) {
		t.Fatalf("a 500 must not stop the run, got: %v", err)
	}
}

func TestModernUploaderStackPromotionDeniedMarksKeyPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"new-id","stack":{"id":"stack-id","primaryAssetId":"old-id"}}`))
		case "PUT /api/stacks/stack-id":
			w.WriteHeader(http.StatusForbidden)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	_, err := uploader.Upload(writeUploaderFixture(t), stackedAsset(""), &noopEmitter{})
	if !errors.Is(err, errKeyPermission) {
		t.Fatalf("expected a key-permission error that stops the run, got: %v", err)
	}
	if !strings.Contains(err.Error(), "stack.update") {
		t.Fatalf("expected the error to name the stack.update permission, got: %v", err)
	}
}

func TestModernUploaderDoesNotDeleteOldAssetWhenTheStackReadFails(t *testing.T) {
	deleted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/assets/new-id":
			w.WriteHeader(http.StatusInternalServerError)
		case "DELETE /api/assets":
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	_, err := uploader.Upload(writeUploaderFixture(t), stackedAsset(""), &noopEmitter{})
	if err == nil {
		t.Fatal("expected an error when the stack re-read fails")
	}
	if deleted {
		t.Fatal("old asset must not be trashed when the stack re-read fails")
	}
	if !strings.Contains(err.Error(), "NOT deleted") {
		t.Fatalf("expected the error to state the old asset survived, got: %v", err)
	}
	if errors.Is(err, errKeyPermission) {
		t.Fatalf("a 500 must not stop the run, got: %v", err)
	}
}
