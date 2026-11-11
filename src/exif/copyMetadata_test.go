package exif

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func writeTinyJPEG(t *testing.T, name string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(tinyJPEGBase64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestCopyMetadataLeavesGeometryBehind(t *testing.T) {
	if err := CheckExiftool(); err != nil {
		t.Skipf("exiftool unavailable: %v", err)
	}

	original := writeTinyJPEG(t, "original.jpg")
	err := WriteExifTags(original, []string{
		"-Make=Canon", "-Model=EOS R5", "-LensModel=RF 50mm",
		"-GPSLatitude=48.85", "-GPSLatitudeRef=N",
		"-XMP-dc:Subject=holiday",
		"-Orientation#=6",
		"-ExifImageWidth=4000", "-ExifImageHeight=3000",
		"-SubjectArea=2000 1500 400 300",
		"-XMP-mwg-rs:RegionInfo={AppliedToDimensions={W=4000,H=3000,Unit=pixel},RegionList=[{Name=Ada,Type=Face,Area={X=0.5,Y=0.5,W=0.1,H=0.1,Unit=normalized}}]}",
	})
	if err != nil {
		t.Fatalf("seed original: %v", err)
	}
	seeded, err := ReadExifTags(original)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	for _, key := range []string{"Orientation", "ExifImageWidth", "XMP-mwg-rs:RegionInfo"} {
		if seeded[key] == nil {
			t.Fatalf("fixture lacks %s", key)
		}
	}
	render := writeTinyJPEG(t, "render.jpeg")

	if err := CopyMetadata(original, render); err != nil {
		t.Fatalf("copy metadata: %v", err)
	}
	tags, err := ReadExifTags(render)
	if err != nil {
		t.Fatalf("read render: %v", err)
	}

	for key, want := range map[string]string{"Make": "Canon", "Model": "EOS R5", "LensModel": "RF 50mm", "Subject": "holiday"} {
		if got, _ := tags[key].(string); got != want {
			t.Errorf("%s = %v, want %v", key, tags[key], want)
		}
	}
	if tags["GPSLatitude"] == nil {
		t.Error("GPS must be carried over")
	}
	for _, key := range []string{"Orientation", "ExifImageWidth", "ExifImageHeight", "SubjectArea", "XMP-mwg-rs:RegionInfo"} {
		if value, ok := tags[key]; ok {
			t.Errorf("%s must not be carried over, got %v", key, value)
		}
	}
	if tags["ImageWidth"] != float64(1) || tags["ImageHeight"] != float64(1) {
		t.Errorf("render raster size changed: %vx%v", tags["ImageWidth"], tags["ImageHeight"])
	}
}
