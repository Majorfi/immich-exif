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

// immichDateOf is Immich's capture date as a file should carry it. Immich
// serializes the instant in UTC and keeps the zone apart; when that zone is
// usable, the instant is re-anchored on it so the wall clock matches what
// Immich shows.
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
// without an offset reads as wall-clock time, so it is compared on the wall
// clock. One with an offset must name the same instant and, when strictOffset
// is set and Immich knows the zone, carry that zone's offset: Immich's zone
// wins over an offset left behind by an edit.
func (d immichDate) matches(fileDate string, strictOffset bool) bool {
	fileDate = strings.TrimSpace(fileDate)
	parsed, err := ParseDateTime(fileDate)
	if err != nil {
		return false
	}
	if !HasInlineOffset(fileDate) {
		return parsed.Format("2006-01-02 15:04:05") == d.local.Format("2006-01-02 15:04:05")
	}
	if !parsed.Truncate(time.Second).Equal(d.local.Truncate(time.Second)) {
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
	if HasInlineOffset(date) {
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

func HasInlineOffset(date string) bool {
	date = strings.TrimSpace(date)
	if strings.HasSuffix(date, "Z") {
		return true
	}
	n := len(date)
	return n >= 6 && (date[n-6] == '+' || date[n-6] == '-') && date[n-3] == ':'
}

func ReadsAsImmichDate(info *model.ExifInfo, tags ExifTagMap, date string) bool {
	d, ok := immichDateOf(info)
	return ok && d.matches(withSeparateOffset(tags, date), false)
}
