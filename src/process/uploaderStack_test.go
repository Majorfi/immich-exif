package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

func stackedAsset(visibility string) *model.AssetResponse {
	return &model.AssetResponse{
		ID:               "old-id",
		DeviceAssetID:    "device-asset-id",
		DeviceID:         "device-id",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
		Visibility:       visibility,
		Stack:            &model.AssetStack{ID: "stack-id", PrimaryAssetID: "old-id"},
	}
}

func TestModernUploaderPromotesReplacementToStackPrimary(t *testing.T) {
	var calls []string
	var promotedTo string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/assets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case "PUT /api/assets/copy":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"new-id","stack":{"id":"stack-id","primaryAssetId":"old-id","assetCount":3}}`))
		case "PUT /api/stacks/stack-id":
			var payload model.UpdateStackRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode stack payload: %v", err)
			}
			promotedTo = payload.PrimaryAssetID
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	if _, err := uploader.Upload(writeUploaderFixture(t), stackedAsset(""), &noopEmitter{}); err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	if promotedTo != "new-id" {
		t.Fatalf("expected new-id promoted as stack primary, got %q", promotedTo)
	}
	want := []string{
		"POST /api/assets",
		"PUT /api/assets/copy",
		"GET /api/assets/new-id",
		"PUT /api/stacks/stack-id",
		"DELETE /api/assets",
	}
	assertCalls(t, calls, want)
}

func TestModernUploaderSkipsStackPromotionForStackChild(t *testing.T) {
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

	asset := stackedAsset("")
	asset.Stack.PrimaryAssetID = "sibling-id"

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	if _, err := uploader.Upload(writeUploaderFixture(t), asset, &noopEmitter{}); err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	assertCalls(t, calls, []string{"POST /api/assets", "PUT /api/assets/copy", "DELETE /api/assets"})
}

func TestModernUploaderSkipsStackPromotionAfterStackMerge(t *testing.T) {
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
		case "GET /api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"new-id","stack":{"id":"other-stack","primaryAssetId":"third-id","assetCount":4}}`))
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	if _, err := uploader.Upload(writeUploaderFixture(t), stackedAsset(""), &noopEmitter{}); err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	assertCalls(t, calls, []string{
		"POST /api/assets",
		"PUT /api/assets/copy",
		"GET /api/assets/new-id",
		"DELETE /api/assets",
	})
}

func TestModernUploaderDoesNotDeleteOldAssetWhenStackPromotionFails(t *testing.T) {
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
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"new-id","stack":{"id":"stack-id","primaryAssetId":"old-id","assetCount":3}}`))
		case "PUT /api/stacks/stack-id":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
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
		t.Fatal("expected an error when the stack promotion fails")
	}
	if deleted {
		t.Fatal("old asset must not be trashed when the stack promotion fails")
	}
	if !strings.Contains(err.Error(), "stack.update") {
		t.Fatalf("expected the error to name the stack.update permission, got: %v", err)
	}
}

func TestModernUploaderPromotesStackPrimaryBeforeRestoringHiddenVisibility(t *testing.T) {
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
		case "GET /api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"new-id","stack":{"id":"stack-id","primaryAssetId":"old-id","assetCount":3}}`))
		case "PUT /api/stacks/stack-id":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		case "PUT /api/assets":
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "test-key")}
	if _, err := uploader.Upload(writeUploaderFixture(t), stackedAsset("hidden"), &noopEmitter{}); err != nil {
		t.Fatalf("unexpected upload error: %v", err)
	}

	assertCalls(t, calls, []string{
		"POST /api/assets",
		"PUT /api/assets/copy",
		"GET /api/assets/new-id",
		"PUT /api/stacks/stack-id",
		"PUT /api/assets",
		"DELETE /api/assets",
	})
}

func assertCalls(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("unexpected call sequence:\n got: %v\nwant: %v", got, want)
	}
}
