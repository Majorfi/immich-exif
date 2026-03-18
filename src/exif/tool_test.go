package exif

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestFloatMatchTolerance(t *testing.T) {
	base := 48.856613
	if !FloatMatch(base, base+(gpsMatchTolerance/2.0)) {
		t.Fatalf("expected match within tolerance")
	}
	if FloatMatch(base, base+(gpsMatchTolerance*2.0)) {
		t.Fatalf("expected mismatch outside tolerance")
	}
}

func TestBuildExifArgsDateTimeOffsetAndTimeZone(t *testing.T) {
	dateTimeOriginal := "2025-12-10T16:56:36+01:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dateTimeOriginal}

	testCases := []struct {
		name        string
		existing    ExifTagMap
		expectEmpty bool
	}{
		{
			name: "all datetime tags match",
			existing: ExifTagMap{
				"DateTimeOriginal":          "2025:12:10 16:56:36",
				"OffsetTimeOriginal":        "+01:00",
				"TimeZoneOffset":            float64(1),
				"XMP-exif:DateTimeOriginal": "2025-12-10T16:56:36+01:00",
				"XMP-xmp:CreateDate":        "2025-12-10T16:56:36+01:00",
			},
			expectEmpty: true,
		},
		{
			name: "timezone mismatch produces args",
			existing: ExifTagMap{
				"DateTimeOriginal":   "2025:12:10 16:56:36",
				"OffsetTimeOriginal": "+01:00",
				"TimeZoneOffset":     float64(2),
			},
			expectEmpty: false,
		},
		{
			name: "missing timezone produces args when whole hour offset",
			existing: ExifTagMap{
				"DateTimeOriginal":   "2025:12:10 16:56:36",
				"OffsetTimeOriginal": "+01:00",
			},
			expectEmpty: false,
		},
		{
			name: "missing offset produces args even with timezone present",
			existing: ExifTagMap{
				"DateTimeOriginal": "2025:12:10 16:56:36",
				"TimeZoneOffset":   float64(1),
			},
			expectEmpty: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := CollectExifArgs(CompareMetadata(exif, tc.existing))
			gotEmpty := len(args) == 0
			if gotEmpty != tc.expectEmpty {
				t.Fatalf("expected empty=%v, got empty=%v (args: %v)", tc.expectEmpty, gotEmpty, args)
			}
		})
	}
}

func TestBuildExifArgsDateTimeNonWholeHourOffset(t *testing.T) {
	dateTimeOriginal := "2025-12-10T16:56:36+05:30"
	exif := &model.ExifInfo{DateTimeOriginal: &dateTimeOriginal}
	existing := ExifTagMap{
		"DateTimeOriginal":          "2025:12:10 16:56:36",
		"OffsetTimeOriginal":        "+05:30",
		"XMP-exif:DateTimeOriginal": "2025-12-10T16:56:36+05:30",
		"XMP-xmp:CreateDate":        "2025-12-10T16:56:36+05:30",
	}

	args := CollectExifArgs(CompareMetadata(exif, existing))
	if len(args) != 0 {
		t.Fatalf("expected no args for matching non-whole-hour offset, got %v", args)
	}
}

func TestBuildExifArgsDescriptionCompanionTags(t *testing.T) {
	description := "A sample description"
	exif := &model.ExifInfo{Description: &description}

	existingMatch := ExifTagMap{
		"ImageDescription":      description,
		"XPComment":             description,
		"XMP-dc:Description":    description,
		"IPTC:Caption-Abstract": description,
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingMatch))) != 0 {
		t.Fatalf("expected no args when description companion tags match")
	}

	existingMissingCompanion := ExifTagMap{
		"ImageDescription": description,
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingMissingCompanion))) == 0 {
		t.Fatalf("expected args when XPComment is missing")
	}
}

func TestBuildExifArgsRatingCompanionTags(t *testing.T) {
	rating := 4
	exif := &model.ExifInfo{Rating: &rating}

	existingMatch := ExifTagMap{
		"Rating":         float64(4),
		"RatingPercent":  float64(80),
		"XMP-xmp:Rating": float64(4),
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingMatch))) != 0 {
		t.Fatalf("expected no args when rating companion tags match")
	}

	existingWrongPercent := ExifTagMap{
		"Rating":        float64(4),
		"RatingPercent": float64(60),
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingWrongPercent))) == 0 {
		t.Fatalf("expected args when RatingPercent does not match")
	}
}

func TestBuildExifArgsLocationDualTags(t *testing.T) {
	city := "Paris"
	state := "Ile-de-France"
	country := "France"
	exif := &model.ExifInfo{
		City:    &city,
		State:   &state,
		Country: &country,
	}

	existingGroupedMatch := ExifTagMap{
		"IPTC:City":                        city,
		"XMP-photoshop:City":               city,
		"IPTC:Province-State":              state,
		"XMP-photoshop:State":              state,
		"IPTC:Country-PrimaryLocationName": country,
		"XMP-photoshop:Country":            country,
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingGroupedMatch))) != 0 {
		t.Fatalf("expected no args when grouped IPTC/XMP location tags match")
	}

	existingGroupedMismatch := ExifTagMap{
		"IPTC:City":                        city,
		"XMP-photoshop:City":               "Lyon",
		"IPTC:Province-State":              state,
		"XMP-photoshop:State":              state,
		"IPTC:Country-PrimaryLocationName": country,
		"XMP-photoshop:Country":            country,
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingGroupedMismatch))) == 0 {
		t.Fatalf("expected args when one grouped location tag differs")
	}

	existingFallbackMatch := ExifTagMap{
		"City":                        city,
		"Province-State":              state,
		"Country-PrimaryLocationName": country,
	}
	if len(CollectExifArgs(CompareMetadata(exif, existingFallbackMatch))) != 0 {
		t.Fatalf("expected no args when fallback location tags match")
	}
}

func TestDeriveOffsetFromExistingLocalTime(t *testing.T) {
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:10 17:56:36",
	}

	offsetStr, tzHours, hasWholeHour, canInfer := DeriveOffsetValuesForMissingOffset(existing, "2025-12-10T16:56:36Z")
	if !canInfer {
		t.Fatalf("expected inference to succeed")
	}
	if offsetStr != "+01:00" {
		t.Fatalf("expected +01:00, got %s", offsetStr)
	}
	if !hasWholeHour {
		t.Fatalf("expected whole-hour offset")
	}
	if tzHours != 1 {
		t.Fatalf("expected timezone 1, got %d", tzHours)
	}
}

func TestOffsetSecondsArePlausible(t *testing.T) {
	testCases := []struct {
		name          string
		offsetSeconds int
		expected      bool
	}{
		{name: "valid whole hour", offsetSeconds: 5 * 3600, expected: true},
		{name: "valid half hour", offsetSeconds: 5*3600 + 30*60, expected: true},
		{name: "valid quarter hour", offsetSeconds: 5*3600 + 45*60, expected: true},
		{name: "too large positive", offsetSeconds: 15 * 3600, expected: false},
		{name: "too large negative", offsetSeconds: -13 * 3600, expected: false},
		{name: "invalid minute granularity", offsetSeconds: 5*3600 + 1*60, expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := OffsetSecondsArePlausible(tc.offsetSeconds)
			if got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestBuildExifArgsFallsBackWhenInferredTimezoneIsImplausible(t *testing.T) {
	dateTimeOriginal := "2025-12-10T16:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dateTimeOriginal}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:11 16:56:36",
	}

	args := CollectExifArgs(CompareMetadata(exif, existing))
	if len(args) == 0 {
		t.Fatalf("expected datetime args fallback for implausible inferred timezone")
	}
}

func TestBuildDiffEntriesFallBackWhenInferredTimezoneIsImplausible(t *testing.T) {
	dateTimeOriginal := "2025-12-10T16:56:36Z"
	exif := &model.ExifInfo{DateTimeOriginal: &dateTimeOriginal}
	existing := ExifTagMap{
		"DateTimeOriginal": "2025:12:11 16:56:36",
	}

	entries := CollectDiffEntries(CompareMetadata(exif, existing))
	if len(entries) == 0 {
		t.Fatalf("expected datetime diff fallback for implausible inferred timezone")
	}
}

func TestBuildExifArgsSameMomentDifferentTimezone(t *testing.T) {
	dateTimeOriginal := "2025-12-10T15:56:36+00:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dateTimeOriginal}
	existing := ExifTagMap{
		"DateTimeOriginal":          "2025:12:10 16:56:36",
		"OffsetTimeOriginal":        "+01:00",
		"TimeZoneOffset":            float64(1),
		"XMP-exif:DateTimeOriginal": "2025-12-10T15:56:36+00:00",
		"XMP-xmp:CreateDate":        "2025-12-10T15:56:36+00:00",
	}

	args := CollectExifArgs(CompareMetadata(exif, existing))
	if len(args) != 0 {
		t.Fatalf("expected no args for same moment in different timezone representation, got %v", args)
	}
}

func TestBuildDiffEntriesSameMomentDifferentTimezone(t *testing.T) {
	dateTimeOriginal := "2025-12-10T15:56:36+00:00"
	exif := &model.ExifInfo{DateTimeOriginal: &dateTimeOriginal}
	existing := ExifTagMap{
		"DateTimeOriginal":          "2025:12:10 16:56:36",
		"OffsetTimeOriginal":        "+01:00",
		"TimeZoneOffset":            float64(1),
		"XMP-exif:DateTimeOriginal": "2025-12-10T15:56:36+00:00",
		"XMP-xmp:CreateDate":        "2025-12-10T15:56:36+00:00",
	}

	entries := CollectDiffEntries(CompareMetadata(exif, existing))
	if len(entries) != 0 {
		t.Fatalf("expected no diff entries for same moment in different timezone, got %v", entries)
	}
}

func TestWriteExifTagsHandlesLiteralArgs(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.jpg")
	os.WriteFile(filePath, []byte("fake"), 0644)

	args := []string{
		"-ImageDescription=Line one\nLine two",
		"-XPComment=Value with # hash",
		"-IPTC:City=Normal city",
	}

	err := WriteExifTags(filePath, args)
	if err != nil && !strings.Contains(err.Error(), "exiftool") {
		t.Fatalf("unexpected non-exiftool error: %v", err)
	}
}
