package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	e.sequence = append(e.sequence, "progress:"+event.Step)
}

func (e *recordingEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	e.sequence = append(e.sequence, "diff")
	return model.ActionConfirm
}

func TestProcessAssetEmitsStepProgress(t *testing.T) {
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

	diffAt := slices.Index(emitter.sequence, "diff")
	if diffAt == -1 {
		t.Fatalf("expected a diff to be emitted, got sequence: %v", emitter.sequence)
	}

	wantBeforeDiff := []string{
		"progress:Scanning photo.jpg...",
		"progress:Downloading photo.jpg...",
		"progress:Analyzing photo.jpg...",
	}
	matched := 0
	for _, s := range emitter.sequence[:diffAt] {
		if matched < len(wantBeforeDiff) && s == wantBeforeDiff[matched] {
			matched++
		}
	}
	if matched != len(wantBeforeDiff) {
		t.Fatalf("expected steps %v in order before the diff, got: %v", wantBeforeDiff, emitter.sequence[:diffAt])
	}
	if !slices.Contains(emitter.sequence[diffAt:], "progress:Writing tags to photo.jpg...") {
		t.Fatalf("expected a writing-tags step after the diff, got: %v", emitter.sequence[diffAt:])
	}

	first := emitter.progress[0]
	if first.Index != 2 || first.Total != 5 {
		t.Fatalf("expected batch position 2/5, got %d/%d", first.Index, first.Total)
	}
	if first.AssetID != "asset-1" {
		t.Fatalf("expected asset ID on step events, got %q", first.AssetID)
	}
	if first.Filename != "" {
		t.Fatalf("step events must not carry a filename (it would trigger the emitter header), got %q", first.Filename)
	}

	sawFullDownload := false
	for _, p := range emitter.progress {
		if p.Step == "Downloading photo.jpg..." && p.Percent == 100 {
			sawFullDownload = true
		}
	}
	if !sawFullDownload {
		t.Fatalf("expected a download progress event reaching 100%%, got: %+v", emitter.progress)
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
