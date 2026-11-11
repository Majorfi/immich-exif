package process

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

type exifWithGeometry struct {
	model.ExifInfo
	ExifImageWidth  int `json:"exifImageWidth"`
	ExifImageHeight int `json:"exifImageHeight"`
}

type assetWithGeometry struct {
	model.AssetResponse
	ExifInfo exifWithGeometry `json:"exifInfo"`
}

const bakeTestEdits = `[{"id":"e1","action":"crop","parameters":{"x":1000,"y":500,"width":2000,"height":1000}},{"id":"e2","action":"rotate","parameters":{"angle":90}}]`

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func bakeTestImages(t *testing.T) (preview, render, stale []byte) {
	t.Helper()
	var edits []model.AssetEdit
	_ = json.Unmarshal([]byte(bakeTestEdits), &edits)
	source := patternImage(400, 300, 1)
	draw := func(edits []model.AssetEdit) []byte {
		steps := editSteps(edits)
		return encodeJPEG(t, transformed(source, 100, 200, func(x, y int) (int, int) {
			p := sourcePoint(point{x: (float64(x) + 0.5) / 100, y: (float64(y) + 0.5) / 200}, steps, 4000, 3000)
			return int(p.x * 400), int(p.y * 300)
		}))
	}
	mirrored := append(append([]model.AssetEdit{}, edits...), model.AssetEdit{Action: "mirror", Parameters: []byte(`{"axis":"horizontal"}`)})
	return encodeJPEG(t, source), draw(edits), draw(mirrored)
}

func editedAssetServer(t *testing.T, renderBody string) *httptest.Server {
	t.Helper()
	return editedAssetServerAt(t, renderBody, time.Time{}, time.Now())
}

func editedAssetServerAt(t *testing.T, renderBody string, updatedAt, serverNow time.Time) *httptest.Server {
	t.Helper()
	return editedAssetServerWithTags(t, renderBody, updatedAt, serverNow, func() []model.TagResponse { return nil })
}

func editedAssetServerWithTags(t *testing.T, renderBody string, updatedAt, serverNow time.Time, tags func() []model.TagResponse) *httptest.Server {
	t.Helper()
	desc := "Test Description"
	preview, render, stale := bakeTestImages(t)
	bodies := map[string][]byte{"render": render, "stale": stale, "original": []byte("original")}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/server/ping":
			w.Header().Set("Date", serverNow.UTC().Format(http.TimeFormat))
			_, _ = w.Write([]byte(`{"res":"pong"}`))
		case r.URL.Path == "/api/assets/asset-1/original" && r.URL.Query().Get("edited") == "true":
			w.Header().Set("Content-Disposition", `inline; filename*=UTF-8''photo.jpeg`)
			_, _ = w.Write(bodies[renderBody])
		case r.URL.Path == "/api/assets/asset-1/thumbnail" && r.URL.Query().Get("edited") == "false":
			_, _ = w.Write(preview)
		case r.URL.Path == "/api/assets/asset-1/original":
			_, _ = w.Write([]byte("original"))
		case r.URL.Path == "/api/assets/asset-1/edits":
			_, _ = w.Write([]byte(`{"assetId":"asset-1","edits":` + bakeTestEdits + `}`))
		case r.URL.Path == "/api/assets/asset-1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(assetWithGeometry{
				AssetResponse: model.AssetResponse{
					ID:               "asset-1",
					OriginalFileName: "photo.jpg",
					Checksum:         sha1HexOf("original"),
					IsEdited:         true,
					UpdatedAt:        updatedAt,
					Tags:             tags(),
				},
				ExifInfo: exifWithGeometry{ExifInfo: model.ExifInfo{Description: &desc}, ExifImageWidth: 4000, ExifImageHeight: 3000},
			})
		default:
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
	}))
}

func withBakeExiftool(t *testing.T, renderWidth, renderHeight int, copied *[2]string) {
	t.Helper()
	restore := withMockExiftool(func(path string) (exif.ExifTagMap, error) {
		if strings.HasPrefix(filepath.Base(filepath.Dir(path)), "edited-") {
			return exif.ExifTagMap{"ImageWidth": float64(renderWidth), "ImageHeight": float64(renderHeight), "Description": "Test Description"}, nil
		}
		return exif.ExifTagMap{"ImageWidth": float64(3000), "ImageHeight": float64(4000), "Orientation": float64(6)}, nil
	}, func(string, []string) error { return nil })
	origCopy := exif.CopyMetadataFn
	exif.CopyMetadataFn = func(src, dst string) error {
		copied[0], copied[1] = src, dst
		return nil
	}
	t.Cleanup(func() {
		restore()
		exif.CopyMetadataFn = origCopy
	})
}

type diffRecorder struct {
	noopEmitter
	entries []model.DiffEntry
}

func (e *diffRecorder) EmitDiff(event model.DiffEvent) model.DiffAction {
	e.entries = append(e.entries, event.Entries...)
	return model.ActionConfirm
}

func TestProcessAssetBakesEditsIntoUpload(t *testing.T) {
	server := editedAssetServer(t, "render")
	defer server.Close()
	var copied [2]string
	withBakeExiftool(t, 1000, 2000, &copied)

	var uploadedName, uploadedBody string
	uploader := &mockUploader{uploadFn: func(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
		data, _ := os.ReadFile(filePath)
		uploadedName, uploadedBody = filepath.Base(filePath), string(data)
		return UploadOutcome{NewID: "new-id", Cacheable: true}, nil
	}}
	emitter := &diffRecorder{}
	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), uploader, &model.Config{ApplyEdits: true}, "asset-1", 1, 1, emitter, nil)

	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Message)
	}
	_, render, _ := bakeTestImages(t)
	if uploadedName != "photo.jpeg" || uploadedBody != string(render) {
		t.Fatalf("expected the render to be uploaded, got %s (%d bytes)", uploadedName, len(uploadedBody))
	}
	if filepath.Base(copied[0]) != "photo.jpg" || filepath.Base(copied[1]) != "photo.jpeg" {
		t.Fatalf("expected metadata copied original -> render, got %v", copied)
	}
	last := emitter.entries[len(emitter.entries)-1]
	if last.Tag != "Immich edits" || !strings.Contains(last.New, "crop 2000x1000, rotate 90°") {
		t.Fatalf("expected an edits diff entry, got %+v", emitter.entries)
	}
}

func TestProcessAssetSkipsStaleRender(t *testing.T) {
	server := editedAssetServer(t, "render")
	defer server.Close()
	var copied [2]string
	withBakeExiftool(t, 100, 50, &copied)

	uploader := &mockUploader{uploadFn: func(string, *model.AssetResponse, model.EventEmitter) (UploadOutcome, error) {
		t.Fatal("a stale render must not be uploaded")
		return UploadOutcome{}, nil
	}}
	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), uploader, &model.Config{ApplyEdits: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "re-rendering") {
		t.Fatalf("expected a stale-render skip, got %s: %s", result.Status, result.Message)
	}
}

func TestProcessAssetSkipsWhenServerFallsBackToOriginal(t *testing.T) {
	server := editedAssetServer(t, "original")
	defer server.Close()
	var copied [2]string
	withBakeExiftool(t, 1000, 2000, &copied)

	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), nil, &model.Config{ApplyEdits: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "not rendered") {
		t.Fatalf("expected a missing-render skip, got %s: %s", result.Status, result.Message)
	}
	if copied[1] != "" {
		t.Fatal("metadata must not be copied onto a fallback original")
	}
}

func TestAppendFaceRegionChangeSkipsUnbakedEditedAsset(t *testing.T) {
	asset := model.AssetResponse{ID: "asset-1", IsEdited: true, People: []model.PersonResponse{{ID: "p1", Name: "Ada"}}}
	_, regions, reason, err := appendFaceRegionChange(nil, &model.Config{Faces: true}, asset, exif.ExifTagMap{}, nil)
	if err != nil || regions != nil {
		t.Fatalf("expected no regions and no error, got %v %v", regions, err)
	}
	if !strings.Contains(reason, "-apply-edits") {
		t.Fatalf("expected an -apply-edits hint, got %q", reason)
	}
}

func TestProcessAssetWaitsForRecentEditsOnTheServerClock(t *testing.T) {
	edited := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	server := editedAssetServerAt(t, "render", edited, edited.Add(10*time.Second))
	defer server.Close()
	var copied [2]string
	withBakeExiftool(t, 1000, 2000, &copied)

	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), nil, &model.Config{ApplyEdits: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "less than a minute ago") {
		t.Fatalf("expected a settle skip, got %s: %s", result.Status, result.Message)
	}
	if copied[1] != "" {
		t.Fatal("nothing must be baked while edits may still be rendering")
	}
}

func TestBakedTagsChange(t *testing.T) {
	asset := model.AssetResponse{Tags: []model.TagResponse{{Value: "Trips/Rome"}, {Value: "Family"}}}
	change := bakedTagsChange(asset, exif.ExifTagMap{"TagsList": "Old"})
	if change == nil || strings.Join(change.Args, " ") != "-XMP-digiKam:TagsList=Family -XMP-digiKam:TagsList=Trips/Rome" {
		t.Fatalf("expected a TagsList rewrite, got %+v", change)
	}
	var listed exif.ExifTagMap
	_ = json.Unmarshal([]byte(`{"TagsList":["Trips/Rome","Family"]}`), &listed)
	if bakedTagsChange(asset, listed) != nil {
		t.Fatal("matching tags must not be rewritten")
	}
	if bakedTagsChange(model.AssetResponse{}, exif.ExifTagMap{"TagsList": "Old"}) != nil {
		t.Fatal("an asset without Immich tags must leave the file's list alone")
	}
}

func TestProcessAssetSkipsBakeWhenTagsChangeMidRun(t *testing.T) {
	fetches := 0
	server := editedAssetServerWithTags(t, "render", time.Time{}, time.Now(), func() []model.TagResponse {
		fetches++
		if fetches > 1 {
			return []model.TagResponse{{Value: "Added"}}
		}
		return nil
	})
	defer server.Close()
	var copied [2]string
	withBakeExiftool(t, 1000, 2000, &copied)

	uploader := &mockUploader{uploadFn: func(string, *model.AssetResponse, model.EventEmitter) (UploadOutcome, error) {
		t.Fatal("a bake with stale tags must not be uploaded")
		return UploadOutcome{}, nil
	}}
	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), uploader, &model.Config{ApplyEdits: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "tags changed") {
		t.Fatalf("expected a tags-changed skip, got %s: %s", result.Status, result.Message)
	}
}

func TestProcessAssetSkipsRenderWithStaleContent(t *testing.T) {
	server := editedAssetServer(t, "stale")
	defer server.Close()
	var copied [2]string
	withBakeExiftool(t, 1000, 2000, &copied)

	uploader := &mockUploader{uploadFn: func(string, *model.AssetResponse, model.EventEmitter) (UploadOutcome, error) {
		t.Fatal("a render showing other edits must not be uploaded")
		return UploadOutcome{}, nil
	}}
	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), uploader, &model.Config{ApplyEdits: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "does not match its edits") {
		t.Fatalf("expected a stale-content skip, got %s: %s", result.Status, result.Message)
	}
}
