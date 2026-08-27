package process

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func TestWorkerPoolSkipMessagePrefersTheStopReason(t *testing.T) {
	pool := &WorkerPool{workers: 1}
	if got := pool.skipMessage(); got != "user cancelled" {
		t.Fatalf("expected the cancellation wording by default, got %q", got)
	}

	pool.stopReason.Store("run stopped: no permission")
	if got := pool.skipMessage(); got != "run stopped: no permission" {
		t.Fatalf("expected the stop reason once set, got %q", got)
	}
}

type keyPermissionUploader struct{ calls int }

func (u *keyPermissionUploader) Upload(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
	u.calls++
	return UploadOutcome{}, nonRetryable(fmt.Errorf("%w: the API key needs the stack.update permission", errKeyPermission))
}

func TestWorkerPoolStopsTheRunOnAKeyPermissionFailure(t *testing.T) {
	description := "Test Description"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/assets/id-1/original" || r.URL.Path == "/api/assets/id-2/original" || r.URL.Path == "/api/assets/id-3/original" {
			_, _ = w.Write([]byte("fake-data"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.AssetResponse{
			ID:               "asset",
			OriginalFileName: "photo.jpg",
			Checksum:         sha1HexOf("fake-data"),
			ExifInfo:         &model.ExifInfo{Description: &description},
		})
	}))
	defer server.Close()

	originalRead, originalWrite := exif.ReadExifTagsFn, exif.WriteExifTagsFn
	exif.ReadExifTagsFn = func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil }
	exif.WriteExifTagsFn = func(string, []string) error { return nil }
	defer func() {
		exif.ReadExifTagsFn = originalRead
		exif.WriteExifTagsFn = originalWrite
	}()

	uploader := &keyPermissionUploader{}
	pool := NewWorkerPool(api.NewImmichClient(server.URL, "key"), uploader, &model.Config{Workers: 1}, &noopEmitter{})

	results := pool.Process([]string{"id-1", "id-2", "id-3"})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if uploader.calls != 1 {
		t.Fatalf("expected the run to stop after the first denied asset, got %d uploads", uploader.calls)
	}

	stopped := 0
	for _, result := range results {
		if result.Message == "run stopped: the API key is missing a permission every remaining asset needs" {
			stopped++
		}
		if result.Message == "user cancelled" {
			t.Fatal("a key-permission stop must not be reported as a user cancellation")
		}
	}
	if stopped != 2 {
		t.Fatalf("expected the 2 untouched assets to carry the stop reason, got %d", stopped)
	}
}
