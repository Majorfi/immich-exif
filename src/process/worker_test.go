package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func TestWorkerPoolCancel(t *testing.T) {
	pool := &WorkerPool{
		workers: 1,
	}
	if pool.cancelled.Load() {
		t.Fatal("expected not cancelled initially")
	}
	pool.Cancel()
	if !pool.cancelled.Load() {
		t.Fatal("expected cancelled after Cancel()")
	}
}

func TestNewWorkerPoolSetsFields(t *testing.T) {
	cfg := &model.Config{Workers: 4}
	pool := NewWorkerPool(nil, nil, cfg, nil)
	if pool.workers != 4 {
		t.Fatalf("expected 4 workers, got %d", pool.workers)
	}
	if pool.cfg != cfg {
		t.Fatal("expected cfg to be set")
	}
}

func TestWorkerPoolProcessEmpty(t *testing.T) {
	emitter := &noopEmitter{}
	cfg := &model.Config{Workers: 1}
	pool := NewWorkerPool(nil, nil, cfg, emitter)

	results := pool.Process([]string{})
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestWorkerPoolProcessCollectsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := model.AssetResponse{
			ID:               "test-id",
			OriginalFileName: "photo.jpg",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{Workers: 1, DryRun: true, Yes: true}
	emitter := &noopEmitter{}
	pool := NewWorkerPool(client, nil, cfg, emitter)

	results := pool.Process([]string{"id-1", "id-2"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r.AssetID != results[i].AssetID {
			t.Fatalf("result[%d]: unexpected asset ID %s", i, r.AssetID)
		}
	}
}

func TestWorkerPoolProcessCancellation(t *testing.T) {
	desc := "Test Description"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/assets/id-1/original" || r.URL.Path == "/api/assets/id-2/original" || r.URL.Path == "/api/assets/id-3/original" {
			w.Write([]byte("fake-data"))
			return
		}
		asset := model.AssetResponse{
			ID:               "asset",
			OriginalFileName: "photo.jpg",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	origRead := exif.ReadExifTagsFn
	origWrite := exif.WriteExifTagsFn
	exif.ReadExifTagsFn = func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil }
	exif.WriteExifTagsFn = func(string, []string) error { return nil }
	defer func() {
		exif.ReadExifTagsFn = origRead
		exif.WriteExifTagsFn = origWrite
	}()

	client := api.NewImmichClient(server.URL, "key")
	emitter := &quitEmitter{}
	pool := NewWorkerPool(client, nil, &model.Config{Workers: 1}, emitter)

	results := pool.Process([]string{"id-1", "id-2", "id-3"})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	cancelledCount := 0
	for _, r := range results {
		if r.Message == "user cancelled" {
			cancelledCount++
		}
	}
	if cancelledCount < 2 {
		t.Fatalf("expected at least 2 cancelled results, got %d", cancelledCount)
	}
}

func TestWorkerPoolProcessFailedAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{Workers: 1}
	emitter := &noopEmitter{}
	pool := NewWorkerPool(client, nil, cfg, emitter)

	results := pool.Process([]string{"bad-1", "bad-2"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Status != model.StatusFailed {
			t.Fatalf("result[%d]: expected failed, got %s", i, r.Status)
		}
	}
}
