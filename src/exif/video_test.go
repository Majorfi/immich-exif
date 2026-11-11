package exif

import (
	"os/exec"
	"path/filepath"
	"slices"
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

func TestCompareVideoDateTimeRewritesStaleValue(t *testing.T) {
	dt := "2025-12-10T15:56:36Z"
	exifInfo := &model.ExifInfo{DateTimeOriginal: &dt}
	existing := ExifTagMap{
		"DateTimeOriginal": "2001:01:01 00:00:00",
	}

	change := compareVideoDateTime(exifInfo, existing)
	if change == nil {
		t.Fatal("expected a change when the existing datetime is stale")
	}

	if !slices.Contains(change.Args, "-DateTimeOriginal="+dt) {
		t.Fatalf("expected -DateTimeOriginal=%s, got %v", dt, change.Args)
	}

	foundChange := false
	for _, d := range change.Diffs {
		if d.Tag == "DateTimeOriginal" && d.Symbol == model.DiffChange && d.Old == "2001:01:01 00:00:00" {
			foundChange = true
		}
	}
	if !foundChange {
		t.Fatalf("expected a DiffChange entry for the stale DateTimeOriginal, got %v", change.Diffs)
	}
}

func TestCompareAssetMetadataRoutesVideoAndPhoto(t *testing.T) {
	desc := "hello"
	unsupported := model.AssetResponse{OriginalFileName: "c.webm", OriginalMimeType: "video/webm", ExifInfo: &model.ExifInfo{Description: &desc}}
	if changes := CompareAssetMetadata(unsupported, ExifTagMap{}); changes != nil {
		t.Fatalf("expected nil for unsupported video, got %v", changes)
	}

	video := model.AssetResponse{OriginalFileName: "c.mp4", OriginalMimeType: "video/mp4", ExifInfo: &model.ExifInfo{Description: &desc}}
	if len(CollectExifArgs(CompareAssetMetadata(video, ExifTagMap{}))) == 0 {
		t.Fatal("expected metadata args for a supported video")
	}

	photo := model.AssetResponse{OriginalFileName: "p.jpg", OriginalMimeType: "image/jpeg", ExifInfo: &model.ExifInfo{Description: &desc}}
	if len(CollectExifArgs(CompareAssetMetadata(photo, ExifTagMap{}))) == 0 {
		t.Fatal("expected metadata args for a photo")
	}
}

func TestCompareVideoDateTimeAnchorsOnImmichTimeZone(t *testing.T) {
	cases := map[string]struct{ zone, want string }{
		"utc offset": {"UTC+9", "2026-06-25T08:20:13+09:00"},
		"iana zone":  {"Asia/Tokyo", "2026-06-25T08:20:13+09:00"},
		"no zone":    {"", "2026-06-24T23:20:13+00:00"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dt := "2026-06-24T23:20:13+00:00"
			info := &model.ExifInfo{DateTimeOriginal: &dt}
			if tc.zone != "" {
				info.TimeZone = &tc.zone
			}
			change := compareVideoDateTime(info, ExifTagMap{})
			if change == nil {
				t.Fatal("expected a change for a video without dates")
			}
			for _, tag := range []string{"DateTimeOriginal", "XMP-exif:DateTimeOriginal", "XMP-xmp:CreateDate"} {
				if !slices.Contains(change.Args, "-"+tag+"="+tc.want) {
					t.Fatalf("expected -%s=%s, got %v", tag, tc.want, change.Args)
				}
			}
		})
	}
}

func TestCompareVideoDateTimeKeepsSubSeconds(t *testing.T) {
	dt := "2026-06-24T23:20:13.822Z"
	zone := "UTC+9"
	change := compareVideoDateTime(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, ExifTagMap{})
	if change == nil || !slices.Contains(change.Args, "-DateTimeOriginal=2026-06-25T08:20:13.822+09:00") {
		t.Fatalf("expected the sub-seconds kept on the anchored date, got %v", change)
	}
}

func TestCompareVideoDateTimeKeepsAnchoredFile(t *testing.T) {
	dt := "2026-06-24T23:20:13+00:00"
	zone := "UTC+9"
	existing := ExifTagMap{
		"DateTimeOriginal":          "2026:06:25 08:20:13+09:00",
		"XMP-exif:DateTimeOriginal": "2026:06:25 08:20:13+09:00",
		"XMP-xmp:CreateDate":        "2026:06:25 08:20:13+09:00",
	}
	if change := compareVideoDateTime(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, existing); change != nil {
		t.Fatalf("a file already anchored on the zone must not be rewritten, got %v", change.Args)
	}
}

func TestCompareVideoDateTimeRepairsUTCWrittenFile(t *testing.T) {
	dt := "2026-06-24T23:20:13+00:00"
	zone := "UTC+9"
	existing := ExifTagMap{
		"DateTimeOriginal":          "2026:06:24 23:20:13+00:00",
		"XMP-exif:DateTimeOriginal": "2026:06:24 23:20:13+00:00",
		"XMP-xmp:CreateDate":        "2026:06:24 23:20:13+00:00",
	}
	change := compareVideoDateTime(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, existing)
	if change == nil || !slices.Contains(change.Args, "-DateTimeOriginal=2026-06-25T08:20:13+09:00") {
		t.Fatalf("a date written in UTC must be re-anchored on the asset's zone, got %v", change)
	}
}

func TestCompareVideoDateTimeWithoutZoneMatchesInstant(t *testing.T) {
	dt := "2026-06-24T23:20:13+00:00"
	existing := ExifTagMap{
		"DateTimeOriginal":          "2026:06:25 08:20:13+09:00",
		"XMP-exif:DateTimeOriginal": "2026:06:25 08:20:13+09:00",
		"XMP-xmp:CreateDate":        "2026:06:25 08:20:13+09:00",
	}
	if change := compareVideoDateTime(&model.ExifInfo{DateTimeOriginal: &dt}, existing); change != nil {
		t.Fatalf("without a zone from Immich the file's own offset must be kept, got %v", change.Args)
	}
}
