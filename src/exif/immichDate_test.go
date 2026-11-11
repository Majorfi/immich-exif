package exif

import (
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func immichDateFor(t *testing.T, date, zone string) immichDate {
	t.Helper()
	info := &model.ExifInfo{DateTimeOriginal: &date}
	if zone != "" {
		info.TimeZone = &zone
	}
	d, ok := immichDateOf(info)
	if !ok {
		t.Fatalf("cannot read %q", date)
	}
	return d
}

func TestImmichDateMatches(t *testing.T) {
	cases := []struct {
		name, immich, zone string
		tags               ExifTagMap
		strict, want       bool
	}{
		{"exact match with offset tag", "2025-12-10T16:56:36+01:00", "", ExifTagMap{"DateTimeOriginal": "2025:12:10 16:56:36", "OffsetTimeOriginal": "+01:00"}, true, true},
		{"same moment in another offset, no zone", "2025-12-10T16:56:36+01:00", "", ExifTagMap{"DateTimeOriginal": "2025:12:10 17:56:36", "OffsetTimeOriginal": "+02:00"}, true, true},
		{"different moment", "2025-12-10T16:56:36+01:00", "", ExifTagMap{"DateTimeOriginal": "2025:12:10 18:00:00", "OffsetTimeOriginal": "+01:00"}, true, false},
		{"TimeZoneOffset hours", "2025-12-10T16:56:36+01:00", "", ExifTagMap{"DateTimeOriginal": "2025:12:10 16:56:36", "TimeZoneOffset": float64(1)}, true, true},
		{"unparseable file date", "2025-12-10T16:56:36+01:00", "", ExifTagMap{"DateTimeOriginal": "not-a-date", "OffsetTimeOriginal": "+01:00"}, true, false},
		{"no offset reads as UTC without a zone", "2025-12-10T16:56:36Z", "", ExifTagMap{"DateTimeOriginal": "2025:12:10 16:56:36"}, true, true},
		{"no offset, other wall clock", "2025-12-10T16:56:36Z", "", ExifTagMap{"DateTimeOriginal": "2025:12:10 17:00:00"}, true, false},
		{"zone known: other offset is not Immich's", "2025-12-10T15:56:36Z", "UTC+1", ExifTagMap{"DateTimeOriginal": "2025:12:10 17:56:36", "OffsetTimeOriginal": "+02:00"}, true, false},
		{"zone known: other offset passes when not strict", "2025-12-10T15:56:36Z", "UTC+1", ExifTagMap{"DateTimeOriginal": "2025:12:10 17:56:36", "OffsetTimeOriginal": "+02:00"}, false, true},
		{"zone known: wall clock without offset", "2025-12-10T15:56:36Z", "UTC+1", ExifTagMap{"DateTimeOriginal": "2025:12:10 16:56:36"}, true, true},
		{"Z offset tag is UTC", "2025-12-10T16:56:36Z", "UTC", ExifTagMap{"DateTimeOriginal": "2025:12:10 16:56:36", "OffsetTimeOriginal": "Z"}, true, true},
		{"inline offset wins over separate tags", "2025-12-10T15:56:36Z", "UTC+1", ExifTagMap{"DateTimeOriginal": "2025:12:10 16:56:36+01:00", "OffsetTimeOriginal": "+05:00"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			date, _ := tc.tags["DateTimeOriginal"].(string)
			got := immichDateFor(t, tc.immich, tc.zone).matches(withSeparateOffset(tc.tags, date), tc.strict)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
