package exif

import (
	"fmt"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/model"
)

type immichDate struct {
	local    time.Time
	anchored bool
}

func immichDateOf(info *model.ExifInfo) (immichDate, bool) {
	if info == nil || info.DateTimeOriginal == nil || *info.DateTimeOriginal == "" {
		return immichDate{}, false
	}
	instant, err := ParseDateTime(*info.DateTimeOriginal)
	if err != nil {
		return immichDate{}, false
	}
	if loc := resolveImmichLocation(info.TimeZone); loc != nil {
		return immichDate{local: instant.In(loc), anchored: true}, true
	}
	return immichDate{local: instant}, true
}

// matches reports whether a date read from a file names Immich's date. One
// without an offset reads as wall-clock time and is compared on the wall
// clock, except under strictOffset with a known zone, where it lacks the
// offset Immich needs to read it back. One with an offset must name the same
// instant and, under strictOffset with a known zone, carry that zone's offset:
// Immich's zone wins over an offset left behind by an edit. exact compares to
// the sub-second; otherwise to the second, as EXIF dates are stored.
func (d immichDate) matches(fileDate string, strictOffset, exact bool) bool {
	fileDate = strings.TrimSpace(fileDate)
	parsed, err := ParseDateTime(fileDate)
	if err != nil {
		return false
	}
	layout := "2006-01-02 15:04:05"
	precision := time.Second
	if exact {
		layout, precision = "2006-01-02 15:04:05.000000000", time.Nanosecond
	}
	if !hasInlineOffset(fileDate) {
		return !(strictOffset && d.anchored) && parsed.Format(layout) == d.local.Format(layout)
	}
	if !parsed.Truncate(precision).Equal(d.local.Truncate(precision)) {
		return false
	}
	if !strictOffset || !d.anchored {
		return true
	}
	_, fileOffset := parsed.Zone()
	_, immichOffset := d.local.Zone()
	return fileOffset == immichOffset
}

func withSeparateOffset(tags ExifTagMap, date string) string {
	date = strings.TrimSpace(date)
	if hasInlineOffset(date) {
		return date
	}
	if offset, ok := tags["OffsetTimeOriginal"].(string); ok && strings.TrimSpace(offset) != "" {
		return date + strings.TrimSpace(offset)
	}
	if hours, ok := tags["TimeZoneOffset"].(float64); ok {
		return date + fmt.Sprintf("%+03d:00", int(hours))
	}
	return date
}

func hasInlineOffset(date string) bool {
	date = strings.TrimSpace(date)
	if strings.HasSuffix(date, "Z") {
		return true
	}
	n := len(date)
	return n >= 6 && (date[n-6] == '+' || date[n-6] == '-') && date[n-3] == ':'
}

// ReadImmichDate returns a sidecar date as Immich reads it, with
// OffsetTimeOriginal or TimeZoneOffset applied when the date is
// DateTimeOriginal (the only date they describe), and whether it names
// Immich's instant.
func ReadImmichDate(info *model.ExifInfo, tags ExifTagMap, key, date string) (string, bool) {
	if key == "DateTimeOriginal" {
		date = withSeparateOffset(tags, date)
	}
	d, ok := immichDateOf(info)
	return date, ok && d.matches(date, false, false)
}

// separateOffsetsAgree reports whether OffsetTimeOriginal and TimeZoneOffset,
// when present, name the same offset as the date they describe. TimeZoneOffset
// holds whole hours, so it is only checked against a whole-hour offset.
func separateOffsetsAgree(tags ExifTagMap, date string) bool {
	effective := withSeparateOffset(tags, date)
	parsed, err := ParseDateTime(effective)
	if err != nil || !hasInlineOffset(effective) {
		return true
	}
	_, seconds := parsed.Zone()
	if text, ok := tags["OffsetTimeOriginal"].(string); ok && strings.TrimSpace(text) != "" {
		offset, err := time.Parse("Z07:00", strings.TrimSpace(text))
		if err != nil {
			return false
		}
		if _, offsetSeconds := offset.Zone(); offsetSeconds != seconds {
			return false
		}
	}
	if hours, ok := tags["TimeZoneOffset"].(float64); ok && seconds%3600 == 0 && int(hours)*3600 != seconds {
		return false
	}
	return true
}
