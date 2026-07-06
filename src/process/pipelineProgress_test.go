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

type recordingEmitter struct {
	noopEmitter
	sequence []string
	progress []model.ProgressEvent
}

func (e *recordingEmitter) EmitProgress(event model.ProgressEvent) {
	e.progress = append(e.progress, event)
	e.sequence = append(e.sequence, "progress")
}

func (e *recordingEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	e.sequence = append(e.sequence, "diff")
	return model.ActionConfirm
}

func TestProcessAssetEmitsScanProgressBeforeDiff(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	emitter := &recordingEmitter{}
	result := ProcessAsset(client, nil, &model.Config{DryRun: true}, "asset-1", 2, 5, emitter, nil)
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Message)
	}

	if len(emitter.sequence) < 2 || emitter.sequence[0] != "progress" || emitter.sequence[1] != "diff" {
		t.Fatalf("expected scan progress before diff, got sequence: %v", emitter.sequence)
	}
	scan := emitter.progress[0]
	if scan.Step != "Scanning photo.jpg..." {
		t.Fatalf("unexpected scan step: %q", scan.Step)
	}
	if scan.Index != 2 || scan.Total != 5 {
		t.Fatalf("expected batch position 2/5, got %d/%d", scan.Index, scan.Total)
	}
	if scan.AssetID != "asset-1" {
		t.Fatalf("expected asset ID on scan event, got %q", scan.AssetID)
	}
	if scan.Filename != "" {
		t.Fatalf("scan event must not carry a filename (it would trigger the emitter header), got %q", scan.Filename)
	}
}

func TestProcessAssetEmitsScanProgressForSkippedAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			ExifInfo:         nil,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	emitter := &recordingEmitter{}
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 3, 4, emitter, nil)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s", result.Status)
	}

	if len(emitter.progress) != 1 {
		t.Fatalf("expected exactly one scan progress event, got %d", len(emitter.progress))
	}
	scan := emitter.progress[0]
	if scan.Step != "Scanning photo.jpg..." || scan.Index != 3 || scan.Total != 4 {
		t.Fatalf("unexpected scan event: step=%q position=%d/%d", scan.Step, scan.Index, scan.Total)
	}
}

func TestProcessAssetNoScanProgressWhenFetchFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	emitter := &recordingEmitter{}
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, emitter, nil)
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if len(emitter.progress) != 0 {
		t.Fatalf("expected no progress before the asset is fetched, got %v", emitter.progress)
	}
}
