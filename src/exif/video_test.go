package exif

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestCompareAssetMetadataVideoConvergesOnMP4(t *testing.T) {
	testCompareAssetMetadataVideoConverges(t, "clip.mp4", "video/mp4")
}

func TestCompareAssetMetadataVideoConvergesOnMOV(t *testing.T) {
	testCompareAssetMetadataVideoConverges(t, "clip.mov", "video/quicktime")
}

func TestCompareAssetMetadataVideoConvergesOnM4V(t *testing.T) {
	testCompareAssetMetadataVideoConverges(t, "clip.m4v", "video/x-m4v")
}

func testCompareAssetMetadataVideoConverges(t *testing.T, fileName, mimeType string) {
	t.Helper()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skipf("ffmpeg unavailable: %v", err)
	}
	if err := CheckExiftool(); err != nil {
		t.Skipf("exiftool unavailable: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), fileName)
	cmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "color=c=black:s=16x16:d=1", "-pix_fmt", "yuv420p", "-y", filePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create test video: %v: %s", err, string(output))
	}

	description := "Video description"
	rating := 4
	latitude := 48.8566
	longitude := 2.3522
	city := "Paris"
	state := "Ile-de-France"
	country := "France"
	dateTimeOriginal := "2025-12-10T16:56:36+01:00"
	asset := model.AssetResponse{
		OriginalFileName: fileName,
		OriginalMimeType: mimeType,
		ExifInfo: &model.ExifInfo{
			Description:      &description,
			Rating:           &rating,
			Latitude:         &latitude,
			Longitude:        &longitude,
			City:             &city,
			State:            &state,
			Country:          &country,
			DateTimeOriginal: &dateTimeOriginal,
		},
	}

	args := CollectExifArgs(CompareAssetMetadata(asset, nil))
	if len(args) == 0 {
		t.Fatal("expected video metadata args")
	}
	if err := WriteExifTags(filePath, args); err != nil {
		t.Fatalf("write video metadata: %v", err)
	}

	existing, err := ReadExifTags(filePath)
	if err != nil {
		t.Fatalf("read video metadata: %v", err)
	}

	remainingArgs := CollectExifArgs(CompareAssetMetadata(asset, existing))
	if len(remainingArgs) != 0 {
		t.Fatalf("expected video metadata to converge, got %v", remainingArgs)
	}
}

func TestHasAssetMetadataToEmbedForVideo(t *testing.T) {
	description := "Video description"
	asset := model.AssetResponse{
		OriginalFileName: "clip.mp4",
		OriginalMimeType: "video/mp4",
		ExifInfo:         &model.ExifInfo{Description: &description},
	}

	if !HasAssetMetadataToEmbed(asset) {
		t.Fatal("expected video asset metadata to be detected")
	}
}

func TestHasAssetMetadataToEmbedForVideoWithoutMetadata(t *testing.T) {
	asset := model.AssetResponse{
		OriginalFileName: "clip.mp4",
		OriginalMimeType: "video/mp4",
	}

	if HasAssetMetadataToEmbed(asset) {
		t.Fatal("expected empty video metadata to be ignored")
	}
}

func TestHasAssetMetadataToEmbedForUnsupportedVideo(t *testing.T) {
	description := "Video description"
	asset := model.AssetResponse{
		OriginalFileName: "clip.webm",
		OriginalMimeType: "video/webm",
		ExifInfo:         &model.ExifInfo{Description: &description},
	}

	if HasAssetMetadataToEmbed(asset) {
		t.Fatal("expected unsupported video metadata to be ignored")
	}
}
