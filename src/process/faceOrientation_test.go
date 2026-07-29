package process

import (
	"testing"

	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func TestVideoRotationToOrientation(t *testing.T) {
	cases := []struct {
		rotation int
		want     int
		ok       bool
	}{
		{0, 1, true},
		{90, 6, true},
		{270, 8, true},
		{180, 0, false}, // Immich does not re-orient 180 video regions
		{45, 0, false},  // non-cardinal
	}
	for _, c := range cases {
		got, ok := videoRotationToOrientation(c.rotation)
		if got != c.want || ok != c.ok {
			t.Fatalf("rotation %d: got (%d,%v), want (%d,%v)", c.rotation, got, ok, c.want, c.ok)
		}
	}
}

func TestRegionOrientationImageUsesExifOrientation(t *testing.T) {
	asset := model.AssetResponse{OriginalMimeType: "image/jpeg"}
	existing := exif.ExifTagMap{"Orientation": float64(6)}
	got, ok := regionOrientation(asset, existing)
	if !ok || got != 6 {
		t.Fatalf("image should use EXIF Orientation 6, got (%d,%v)", got, ok)
	}
}

func TestRegionOrientationVideoUsesRotation(t *testing.T) {
	asset := model.AssetResponse{OriginalMimeType: "video/mp4"}
	if got, ok := regionOrientation(asset, exif.ExifTagMap{"Rotation": float64(90)}); !ok || got != 6 {
		t.Fatalf("video rotation 90 should map to orientation 6, got (%d,%v)", got, ok)
	}
	if _, ok := regionOrientation(asset, exif.ExifTagMap{"Rotation": float64(180)}); ok {
		t.Fatal("video rotation 180 must report not-anchorable")
	}
}
