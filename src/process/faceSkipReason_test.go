package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func faceSkipServer(faces []model.AssetFaceResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(faces)
	}))
}

// Every guard in appendFaceRegionChange must name itself: a -faces run that
// embeds nothing has to say which condition stopped it.
func TestAppendFaceRegionChangeExplainsEverySkip(t *testing.T) {
	namedPerson := []model.PersonResponse{{ID: "p1", Name: "Alice"}}
	usableFace := model.AssetFaceResponse{
		BoundingBoxX1: 10, BoundingBoxY1: 20, BoundingBoxX2: 110, BoundingBoxY2: 140,
		ImageWidth: 1000, ImageHeight: 500, Person: &model.PersonResponse{ID: "p1", Name: "Alice"},
	}
	sizedTags := exif.ExifTagMap{"ImageWidth": float64(1920), "ImageHeight": float64(1080)}

	cases := []struct {
		name       string
		asset      model.AssetResponse
		tags       exif.ExifTagMap
		faces      []model.AssetFaceResponse
		wantReason string
	}{
		{
			name:       "no named person",
			asset:      model.AssetResponse{ID: "a", OriginalMimeType: "image/jpeg"},
			tags:       sizedTags,
			wantReason: "no named, visible person",
		},
		{
			name:       "unsupported video container",
			asset:      model.AssetResponse{ID: "a", OriginalMimeType: "video/x-matroska", OriginalFileName: "c.mkv", People: namedPerson},
			tags:       sizedTags,
			wantReason: "container cannot hold face regions",
		},
		{
			// A writable container with nobody named must not be blamed on the
			// container: that would send the user chasing the wrong problem.
			name:       "supported video without named people",
			asset:      model.AssetResponse{ID: "a", OriginalMimeType: "video/quicktime", OriginalFileName: "IMG_4827.MOV"},
			tags:       sizedTags,
			wantReason: "no named, visible person",
		},
		{
			name:       "video rotated 180",
			asset:      model.AssetResponse{ID: "a", OriginalMimeType: "video/mp4", People: namedPerson},
			tags:       exif.ExifTagMap{"ImageWidth": float64(1920), "ImageHeight": float64(1080), "Rotation": float64(180)},
			faces:      []model.AssetFaceResponse{usableFace},
			wantReason: "rotation 180° cannot be anchored",
		},
		{
			name:       "file has no pixel dimensions",
			asset:      model.AssetResponse{ID: "a", OriginalMimeType: "image/jpeg", People: namedPerson},
			tags:       exif.ExifTagMap{},
			faces:      []model.AssetFaceResponse{usableFace},
			wantReason: "no pixel dimensions",
		},
		{
			name:       "face boxes carry no usable person",
			asset:      model.AssetResponse{ID: "a", OriginalMimeType: "image/jpeg", People: namedPerson},
			tags:       sizedTags,
			faces:      []model.AssetFaceResponse{{BoundingBoxX2: 10, BoundingBoxY2: 10, ImageWidth: 100, ImageHeight: 100}},
			wantReason: "carry a named person",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := faceSkipServer(c.faces)
			defer server.Close()
			client := api.NewImmichClient(server.URL, "key")

			_, regions, reason, err := appendFaceRegionChange(client, &model.Config{Faces: true}, c.asset, c.tags, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(regions) != 0 {
				t.Fatalf("expected no regions, got %d", len(regions))
			}
			if !strings.Contains(reason, c.wantReason) {
				t.Fatalf("reason %q does not mention %q", reason, c.wantReason)
			}
		})
	}
}

func TestAppendFaceRegionChangeSilentWithoutFacesFlag(t *testing.T) {
	server := faceSkipServer(nil)
	defer server.Close()

	_, _, reason, err := appendFaceRegionChange(
		api.NewImmichClient(server.URL, "key"),
		&model.Config{Faces: false},
		model.AssetResponse{ID: "a", OriginalMimeType: "image/jpeg"},
		exif.ExifTagMap{}, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != "" {
		t.Fatalf("a run without -faces must not report a face skip, got %q", reason)
	}
}

func TestAppendFaceRegionChangeNoReasonWhenRegionsMatch(t *testing.T) {
	face := model.AssetFaceResponse{
		BoundingBoxX1: 100, BoundingBoxY1: 50, BoundingBoxX2: 300, BoundingBoxY2: 250,
		ImageWidth: 1000, ImageHeight: 500, Person: &model.PersonResponse{ID: "p1", Name: "Alice"},
	}
	server := faceSkipServer([]model.AssetFaceResponse{face})
	defer server.Close()

	asset := model.AssetResponse{ID: "a", OriginalMimeType: "image/jpeg", People: []model.PersonResponse{{ID: "p1", Name: "Alice"}}}
	tags := exif.ExifTagMap{"ImageWidth": float64(4000), "ImageHeight": float64(2000)}

	changes, regions, reason, err := appendFaceRegionChange(api.NewImmichClient(server.URL, "key"), &model.Config{Faces: true}, asset, tags, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 || len(regions) != 1 {
		t.Fatalf("expected one region change, got %d changes / %d regions", len(changes), len(regions))
	}
	if reason != "" {
		t.Fatalf("a successful embed must report no skip reason, got %q", reason)
	}
}
