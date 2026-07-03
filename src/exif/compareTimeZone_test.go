package exif

import (
	"slices"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func TestResolveImmichLocation(t *testing.T) {
	utcPlus2 := "UTC+2"
	utcPlus230 := "UTC+02:30"
	utcMinus5 := "UTC-5"
	rome := "Europe/Rome"
	garbage := "not-a-zone"
	empty := ""
	bare := "UTC"

	cases := []struct {
		name          string
		timeZone      *string
		wantNil       bool
		offsetSeconds int
	}{
		{name: "nil", timeZone: nil, wantNil: true},
		{name: "empty", timeZone: &empty, wantNil: true},
		{name: "garbage", timeZone: &garbage, wantNil: true},
		{name: "bare UTC", timeZone: &bare, offsetSeconds: 0},
		{name: "UTC+2", timeZone: &utcPlus2, offsetSeconds: 2 * 3600},
		{name: "UTC+02:30", timeZone: &utcPlus230, offsetSeconds: 2*3600 + 30*60},
		{name: "UTC-5", timeZone: &utcMinus5, offsetSeconds: -5 * 3600},
		{name: "IANA name", timeZone: &rome, offsetSeconds: 2 * 3600},
	}

	summer := time.Date(2021, 6, 2, 10, 0, 0, 0, time.UTC)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc := resolveImmichLocation(tc.timeZone)
			if tc.wantNil {
				if loc != nil {
					t.Fatalf("expected nil location, got %v", loc)
				}
				return
			}
			if loc == nil {
				t.Fatal("expected a location, got nil")
			}
			_, offset := summer.In(loc).Zone()
			if offset != tc.offsetSeconds {
				t.Fatalf("expected offset %d, got %d", tc.offsetSeconds, offset)
			}
		})
	}
}

// Immich serializes dateTimeOriginal as UTC; with exifInfo.timeZone the write
// must carry the local wall-clock time, not the collapsed UTC one.
func TestCompareDateTimeAnchorsToImmichTimeZone(t *testing.T) {
	date := "2021-06-02T10:00:00.000Z"
	tz := "UTC+2"
	info := &model.ExifInfo{DateTimeOriginal: &date, TimeZone: &tz}

	tc := CompareDateTime(info, ExifTagMap{})
	if tc == nil {
		t.Fatal("expected a change for an empty file")
	}
	if !slices.Contains(tc.Args, "-DateTimeOriginal=2021:06:02 12:00:00") {
		t.Fatalf("expected local wall-clock date, got args: %v", tc.Args)
	}
	if !slices.Contains(tc.Args, "-OffsetTimeOriginal=+02:00") {
		t.Fatalf("expected +02:00 offset, got args: %v", tc.Args)
	}
	if !slices.Contains(tc.Args, "-XMP-exif:DateTimeOriginal=2021-06-02T12:00:00.000+02:00") {
		t.Fatalf("expected local-offset XMP date, got args: %v", tc.Args)
	}
}

func TestCompareDateTimeKeepsUTCWithoutTimeZone(t *testing.T) {
	date := "2021-06-02T10:00:00.000Z"
	info := &model.ExifInfo{DateTimeOriginal: &date}

	tc := CompareDateTime(info, ExifTagMap{})
	if tc == nil {
		t.Fatal("expected a change for an empty file")
	}
	if !slices.Contains(tc.Args, "-DateTimeOriginal=2021:06:02 10:00:00") {
		t.Fatalf("expected UTC date without a time zone, got args: %v", tc.Args)
	}
}

// A file already carrying the correct local time for the same instant must
// not be rewritten when the zone is known.
func TestCompareDateTimeNoChurnWhenLocalTimeMatches(t *testing.T) {
	date := "2021-06-02T10:00:00.000Z"
	tz := "UTC+2"
	info := &model.ExifInfo{DateTimeOriginal: &date, TimeZone: &tz}

	existing := ExifTagMap{
		"DateTimeOriginal":          "2021:06:02 12:00:00",
		"OffsetTimeOriginal":        "+02:00",
		"TimeZoneOffset":            float64(2),
		"XMP-exif:DateTimeOriginal": "2021-06-02T12:00:00+02:00",
		"XMP-xmp:CreateDate":        "2021-06-02T12:00:00+02:00",
	}
	if tc := CompareDateTime(info, existing); tc != nil {
		t.Fatalf("expected no change for a matching local time, got args: %v", tc.Args)
	}
}

func TestCompareRatingClearsExplicitZero(t *testing.T) {
	zero := 0
	info := &model.ExifInfo{Rating: &zero}
	existing := ExifTagMap{"Rating": float64(5), "XMP-xmp:Rating": float64(5)}

	tc := CompareRating(info, existing)
	if tc == nil {
		t.Fatal("expected clearing change for rating 0 with stale file rating")
	}
	if !slices.Contains(tc.Args, "-Rating=") || !slices.Contains(tc.Args, "-XMP-xmp:Rating=") {
		t.Fatalf("expected clearing args, got: %v", tc.Args)
	}
	if slices.Contains(tc.Args, "-RatingPercent=") {
		t.Fatalf("must not clear an absent RatingPercent, got: %v", tc.Args)
	}
}

func TestCompareRatingZeroIsNoopWhenFileHasNoRating(t *testing.T) {
	zero := 0
	info := &model.ExifInfo{Rating: &zero}
	if tc := CompareRating(info, ExifTagMap{}); tc != nil {
		t.Fatalf("expected no change, got: %v", tc.Args)
	}
	if tc := CompareRating(info, ExifTagMap{"Rating": float64(0)}); tc != nil {
		t.Fatalf("expected no change for an already-zero file rating, got: %v", tc.Args)
	}
}

func TestCompareRatingNilNeverClears(t *testing.T) {
	info := &model.ExifInfo{}
	if tc := CompareRating(info, ExifTagMap{"Rating": float64(5)}); tc != nil {
		t.Fatalf("a null rating (never set in Immich) must not clear the file, got: %v", tc.Args)
	}
}
