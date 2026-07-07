package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func facesAssetServer(people []model.PersonResponse, faces []model.AssetFaceResponse, facesStatus int, facesCalls *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/faces" {
			facesCalls.Add(1)
			if facesStatus != http.StatusOK {
				http.Error(w, "boom", facesStatus)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(faces)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/original") {
			w.Write([]byte("fake-image-data"))
			return
		}
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			Checksum:         sha1HexOf("fake-image-data"),
			People:           people,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
}

func TestProcessAssetWritesFaceRegions(t *testing.T) {
	var facesCalls atomic.Int32
	server := facesAssetServer(
		[]model.PersonResponse{{ID: "p1", Name: "Alice"}},
		[]model.AssetFaceResponse{{
			BoundingBoxX1: 100, BoundingBoxY1: 50, BoundingBoxX2: 300, BoundingBoxY2: 250,
			ImageWidth: 1000, ImageHeight: 500,
			Person: &model.PersonResponse{ID: "p1", Name: "Alice"},
		}},
		http.StatusOK, &facesCalls,
	)
	defer server.Close()

	var written []string
	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) {
			return exif.ExifTagMap{"ImageWidth": float64(4000), "ImageHeight": float64(2000)}, nil
		},
		func(_ string, args []string) error {
			written = args
			return nil
		},
	)()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{DryRun: true, Faces: true}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Message)
	}
	if facesCalls.Load() != 1 {
		t.Fatalf("expected 1 faces call, got %d", facesCalls.Load())
	}
	want := "-XMP-mwg-rs:RegionInfo={AppliedToDimensions={W=4000,H=2000,Unit=pixel},RegionList=[{Area={X=0.20000,Y=0.30000,W=0.20000,H=0.40000,Unit=normalized},Name=Alice,Type=Face}]}"
	if len(written) != 1 || written[0] != want {
		t.Fatalf("unexpected write args:\n got %v\nwant %s", written, want)
	}
}

func TestProcessAssetFaceRegionsAlreadyMatch(t *testing.T) {
	var facesCalls atomic.Int32
	server := facesAssetServer(
		[]model.PersonResponse{{ID: "p1", Name: "Alice"}},
		[]model.AssetFaceResponse{{
			BoundingBoxX1: 100, BoundingBoxY1: 50, BoundingBoxX2: 300, BoundingBoxY2: 250,
			ImageWidth: 1000, ImageHeight: 500,
			Person: &model.PersonResponse{ID: "p1", Name: "Alice"},
		}},
		http.StatusOK, &facesCalls,
	)
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) {
			return exif.ExifTagMap{
				"ImageWidth":  float64(4000),
				"ImageHeight": float64(2000),
				"XMP-mwg-rs:RegionInfo": map[string]any{
					"AppliedToDimensions": map[string]any{"W": float64(4000), "H": float64(2000), "Unit": "pixel"},
					"RegionList": []any{map[string]any{
						"Name": "Alice",
						"Type": "Face",
						"Area": map[string]any{"X": 0.2, "Y": 0.3, "W": 0.2, "H": 0.4, "Unit": "normalized"},
					}},
				},
			}, nil
		},
		nil,
	)()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{Faces: true}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "metadata already matches") {
		t.Fatalf("expected 'metadata already matches', got: %s", result.Message)
	}
}

func TestProcessAssetUnnamedPeopleSkipsFacesFetch(t *testing.T) {
	var facesCalls atomic.Int32
	server := facesAssetServer([]model.PersonResponse{{ID: "p1", Name: ""}}, nil, http.StatusOK, &facesCalls)
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{Faces: true}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "no metadata") {
		t.Fatalf("expected 'no metadata' skip, got %s: %s", result.Status, result.Message)
	}
	if facesCalls.Load() != 0 {
		t.Fatalf("faces endpoint must not be queried for unnamed people, got %d calls", facesCalls.Load())
	}
}

func TestProcessAssetFacesFetchFails(t *testing.T) {
	var facesCalls atomic.Int32
	server := facesAssetServer([]model.PersonResponse{{ID: "p1", Name: "Alice"}}, nil, http.StatusInternalServerError, &facesCalls)
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		nil,
	)()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{Faces: true}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusFailed || !strings.Contains(result.Message, "fetch faces") {
		t.Fatalf("expected 'fetch faces' failure, got %s: %s", result.Status, result.Message)
	}
}

func TestProcessAssetFacesOffFetchesNothing(t *testing.T) {
	var facesCalls atomic.Int32
	server := facesAssetServer([]model.PersonResponse{{ID: "p1", Name: "Alice"}}, nil, http.StatusOK, &facesCalls)
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "no metadata") {
		t.Fatalf("expected 'no metadata' skip without -faces, got %s: %s", result.Status, result.Message)
	}
	if facesCalls.Load() != 0 {
		t.Fatalf("faces endpoint must not be queried without -faces, got %d calls", facesCalls.Load())
	}
}
