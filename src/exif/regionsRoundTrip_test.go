package exif

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// TestFaceRegionsExiftoolRoundTrip proves the whole chain against a real
// exiftool: the struct serialization (escaping included) writes cleanly, the
// -struct read returns it in the shape parseFaceRegions expects, and the
// comparison then reports a no-op.
func TestFaceRegionsExiftoolRoundTrip(t *testing.T) {
	if err := CheckExiftool(); err != nil {
		t.Skipf("exiftool unavailable: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), "tiny.jpg")
	data, err := base64.StdEncoding.DecodeString(tinyJPEGBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	regions := []FaceRegion{
		{Name: "Ali{ce}", X: 0.25, Y: 0.6, W: 0.2, H: 0.1},
		{Name: "Doe, John", X: 0.5, Y: 0.4, W: 0.1, H: 0.15},
	}

	change := CompareFaceRegions(regions, 4000, 2000, ExifTagMap{})
	if change == nil {
		t.Fatal("expected a change against an empty file")
	}
	if err := WriteExifTags(filePath, change.Args); err != nil {
		t.Fatalf("write exif tags: %v", err)
	}

	existing, err := ReadExifTags(filePath)
	if err != nil {
		t.Fatalf("read exif tags: %v", err)
	}

	parsed := parseFaceRegions(existing["XMP-mwg-rs:RegionInfo"])
	if len(parsed) != 2 || parsed[0].Name != "Ali{ce}" || parsed[1].Name != "Doe, John" {
		t.Fatalf("names did not survive the round trip: %+v", parsed)
	}

	if change := CompareFaceRegions(regions, 4000, 2000, existing); change != nil {
		t.Fatalf("expected a no-op after the round trip, got %+v", change)
	}
}
