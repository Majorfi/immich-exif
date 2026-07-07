package process

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

// The single most data-loss-critical branch: a nonRetryable failure happens
// after a new asset was created on the server, so the pipeline must stop after
// one attempt instead of re-uploading duplicates.
func TestProcessAssetNonRetryableErrorHaltsRetries(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()
	withStubbedSleep(t)

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	attempts := 0
	uploader := &mockUploader{
		uploadFn: func(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
			attempts++
			return UploadOutcome{}, nonRetryable(fmt.Errorf("copy failed after upload"))
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt for a nonRetryable error, got %d", attempts)
	}
}

func TestProcessAssetCancelledDuringRetryLoop(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()
	withStubbedSleep(t)

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	attempts := 0
	uploader := &mockUploader{
		uploadFn: func(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
			attempts++
			return UploadOutcome{}, fmt.Errorf("transient error")
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, func() bool { return true })
	if !result.Cancelled {
		t.Fatal("expected Cancelled=true when cancellation arrives during the retry loop")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt before cancellation stops retries, got %d", attempts)
	}
}

func TestProcessAssetFailsWhenServerReturnsNoChecksum(t *testing.T) {
	desc := "Test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "no checksum") {
		t.Fatalf("expected no-checksum error, got: %s", result.Message)
	}
}

func TestProcessAssetSkipsHiddenVideoAsLivePhotoMotion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		desc := "Test"
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "motion.mov",
			OriginalMimeType: "video/quicktime",
			Visibility:       "hidden",
			Checksum:         sha1HexOf("irrelevant"),
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "live-photo") {
		t.Fatalf("expected live-photo skip message, got: %s", result.Message)
	}
}

// Metadata edited in Immich between the initial fetch and the upload must not
// be overwritten by the replacement; the asset is skipped for a re-run.
func TestProcessAssetSkipsWhenAssetChangedServerSide(t *testing.T) {
	desc := "Test Description"
	var mu sync.Mutex
	getCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/original") {
			w.Write([]byte("fake-image-data"))
			return
		}
		mu.Lock()
		getCount++
		count := getCount
		mu.Unlock()
		updatedAt := "2026-01-01T00:00:00Z"
		if count > 1 {
			updatedAt = "2026-01-01T00:05:00Z"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"asset-1","originalFileName":"photo.jpg","checksum":%q,"updatedAt":%q,"exifInfo":{"description":%q}}`,
			sha1HexOf("fake-image-data"), updatedAt, desc)
	}))
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	uploader := &mockUploader{outcome: UploadOutcome{NewID: "new-id", Cacheable: true}}
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "changed on the server") {
		t.Fatalf("expected staleness message, got: %s", result.Message)
	}
}

// The asymmetric twin of the trashed-duplicate guard: a trashed original must
// not be silently resurrected as a fresh live asset by a stale re-run.
func TestProcessAssetSkipsTrashedOriginal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		desc := "Test"
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			IsTrashed:        true,
			Checksum:         sha1HexOf("irrelevant"),
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "trash") {
		t.Fatalf("expected trash skip message, got: %s", result.Message)
	}
}

// Replacing an external-library asset would migrate it into the internal
// library and duplicate it at the next scan; only read-only modes may touch it.
func TestProcessAssetSkipsExternalLibraryAssetOnReplace(t *testing.T) {
	server := assetServerInLibrary("5b9f1a2e-1111-4222-8333-444455556666")
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "external library") {
		t.Fatalf("expected external-library skip message, got: %s", result.Message)
	}
}

func TestProcessAssetAllowsExternalLibraryAssetInDryRun(t *testing.T) {
	server := assetServerInLibrary("5b9f1a2e-1111-4222-8333-444455556666")
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{DryRun: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected dry-run success, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "dry-run") {
		t.Fatalf("expected dry-run message, got: %s", result.Message)
	}
}

func TestProcessAssetAllowsExternalLibraryAssetInExport(t *testing.T) {
	server := assetServerInLibrary("5b9f1a2e-1111-4222-8333-444455556666")
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{ExportDir: t.TempDir()}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected export success, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "exported to") {
		t.Fatalf("expected export message, got: %s", result.Message)
	}
}
