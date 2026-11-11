package exif

import (
	"slices"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestCompareDateTimeUsesImmichZoneWhenTheWallClockDiffers(t *testing.T) {
	dt, zone := "2021-05-06T09:08:09+00:00", "Pacific/Rarotonga"
	existing := ExifTagMap{"DateTimeOriginal": "2021:05:06 07:08:09"}
	args := CollectExifArgs(CompareMetadata(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, existing))
	for _, want := range []string{"-DateTimeOriginal=2021:05:05 23:08:09", "-OffsetTimeOriginal=-10:00", "-TimeZoneOffset=-10"} {
		if !slices.Contains(args, want) {
			t.Fatalf("a date edited in Immich must be rewritten in Immich's zone: want %s in %v", want, args)
		}
	}
}

func TestCompareDateTimeOnlyAddsTheOffsetWhenTheWallClockMatches(t *testing.T) {
	dt, zone := "2021-05-06T09:08:09+00:00", "UTC+10"
	existing := ExifTagMap{"DateTimeOriginal": "2021:05:06 19:08:09"}
	args := CollectExifArgs(CompareMetadata(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, existing))
	if !slices.Contains(args, "-OffsetTimeOriginal=+10:00") || !slices.Contains(args, "-TimeZoneOffset=10") {
		t.Fatalf("expected Immich's offset added, got %v", args)
	}
	for _, arg := range args {
		if len(arg) > 18 && arg[:18] == "-DateTimeOriginal=" {
			t.Fatalf("a matching wall clock must not be rewritten, got %v", args)
		}
	}
}

func TestCompareDateTimeRepairsAnInventedOffset(t *testing.T) {
	dt, zone := "2021-05-06T09:08:09+00:00", "Pacific/Rarotonga"
	existing := ExifTagMap{"DateTimeOriginal": "2021:05:06 07:08:09", "OffsetTimeOriginal": "-02:00", "TimeZoneOffset": float64(-2)}
	args := CollectExifArgs(CompareMetadata(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, existing))
	for _, want := range []string{"-DateTimeOriginal=2021:05:05 23:08:09", "-OffsetTimeOriginal=-10:00", "-TimeZoneOffset=-10"} {
		if !slices.Contains(args, want) {
			t.Fatalf("an offset that is not Immich's must be repaired: want %s in %v", want, args)
		}
	}
}

func TestCompareDateTimeKeepsAFileAnchoredOnImmichZone(t *testing.T) {
	dt, zone := "2021-05-06T09:08:09+00:00", "Pacific/Rarotonga"
	existing := ExifTagMap{
		"DateTimeOriginal":          "2021:05:05 23:08:09",
		"OffsetTimeOriginal":        "-10:00",
		"TimeZoneOffset":            float64(-10),
		"XMP-exif:DateTimeOriginal": "2021:05:05 23:08:09.000-10:00",
		"XMP-xmp:CreateDate":        "2021:05:05 23:08:09.000-10:00",
	}
	if args := CollectExifArgs(CompareMetadata(&model.ExifInfo{DateTimeOriginal: &dt, TimeZone: &zone}, existing)); len(args) != 0 {
		t.Fatalf("a file already in Immich's zone must not be rewritten, got %v", args)
	}
}
