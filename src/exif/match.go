package exif

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const gpsMatchTolerance = 0.00001
const minTimezoneOffsetMinutes = -12 * 60
const maxTimezoneOffsetMinutes = 14 * 60

var dateTimeFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006:01:02 15:04:05",
	"2006:01:02 15:04:05.999999999",
	"2006:01:02 15:04:05Z07:00",
	"2006:01:02 15:04:05.999999999Z07:00",
}

func FloatMatch(existing any, expected float64) bool {
	if existing == nil {
		return false
	}
	v, ok := existing.(float64)
	if !ok {
		return false
	}
	return math.Abs(v-expected) < gpsMatchTolerance
}

func StringMatch(existing any, expected string) bool {
	if existing == nil {
		return false
	}
	s, ok := existing.(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(s) == strings.TrimSpace(expected)
}

func DateTimeStringMatch(existing any, expected string) bool {
	if StringMatch(existing, expected) {
		return true
	}

	s, ok := existing.(string)
	if !ok {
		return false
	}

	existingTime, err := ParseDateTime(strings.TrimSpace(s))
	if err != nil {
		return false
	}

	expectedTime, err := ParseDateTime(strings.TrimSpace(expected))
	if err != nil {
		return false
	}

	return existingTime.Equal(expectedTime)
}

func IntMatch(existing any, expected int) bool {
	if existing == nil {
		return false
	}
	v, ok := existing.(float64)
	if !ok {
		return false
	}
	return int(v) == expected
}

func hasAnyTagValue(existing ExifTagMap, keys []string) bool {
	for _, key := range keys {
		if existing[key] != nil {
			return true
		}
	}
	return false
}

func AllLocationTagValuesMatch(existing ExifTagMap, strictKeys, fallbackKeys []string, expected string) bool {
	if hasAnyTagValue(existing, strictKeys) {
		for _, key := range strictKeys {
			if !StringMatch(existing[key], expected) {
				return false
			}
		}
		return true
	}

	for _, key := range fallbackKeys {
		if existing[key] != nil {
			return StringMatch(existing[key], expected)
		}
	}

	return false
}

func ParseDateTime(s string) (time.Time, error) {
	for _, layout := range dateTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized datetime format: %s", s)
}

// resolveImmichLocation turns Immich's exifInfo.timeZone into a time.Location.
// Immich stores either an IANA name ("Europe/Rome") or a fixed offset spelled
// "UTC+2" / "UTC+02:30". Unknown or missing values return nil so callers keep
// the server's UTC anchoring.
func resolveImmichLocation(timeZone *string) *time.Location {
	if timeZone == nil {
		return nil
	}
	name := strings.TrimSpace(*timeZone)
	if name == "" {
		return nil
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	return parseUTCOffsetLocation(name)
}

func parseUTCOffsetLocation(name string) *time.Location {
	s := strings.ToUpper(name)
	s = strings.TrimPrefix(s, "UTC")
	s = strings.TrimPrefix(s, "GMT")
	if s == "" {
		return time.UTC
	}

	sign := 1
	switch s[0] {
	case '+':
	case '-':
		sign = -1
	default:
		return nil
	}
	s = s[1:]

	hoursPart := s
	minutesPart := "0"
	if colon := strings.IndexByte(s, ':'); colon >= 0 {
		hoursPart = s[:colon]
		minutesPart = s[colon+1:]
	}
	hours, err := strconv.Atoi(hoursPart)
	if err != nil {
		return nil
	}
	minutes, err := strconv.Atoi(minutesPart)
	if err != nil {
		return nil
	}

	offsetSeconds := sign * (hours*3600 + minutes*60)
	if !OffsetSecondsArePlausible(offsetSeconds) {
		return nil
	}
	return time.FixedZone(name, offsetSeconds)
}

func BuildOffsetValues(offsetSeconds int) (string, int, bool) {
	offsetMinutes := int(math.Round(float64(offsetSeconds) / 60.0))
	sign := "+"
	if offsetMinutes < 0 {
		sign = "-"
		offsetMinutes = -offsetMinutes
	}

	hours := offsetMinutes / 60
	minutes := offsetMinutes % 60
	offsetStr := fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)

	if minutes != 0 {
		return offsetStr, 0, false
	}

	tzHours := hours
	if sign == "-" {
		tzHours = -hours
	}
	return offsetStr, tzHours, true
}

func OffsetSecondsArePlausible(offsetSeconds int) bool {
	offsetMinutes := int(math.Round(float64(offsetSeconds) / 60.0))
	if offsetMinutes < minTimezoneOffsetMinutes || offsetMinutes > maxTimezoneOffsetMinutes {
		return false
	}

	if offsetMinutes < 0 {
		offsetMinutes = -offsetMinutes
	}
	minutePart := offsetMinutes % 60
	if minutePart != 0 && minutePart != 15 && minutePart != 30 && minutePart != 45 {
		return false
	}
	return true
}

func DeriveOffsetValuesForMissingOffset(existing ExifTagMap, expected string) (string, int, bool, bool) {
	immichTime, err := ParseDateTime(expected)
	if err != nil {
		return "", 0, false, false
	}
	existingDate := existing["DateTimeOriginal"]
	if existingDate == nil {
		return "", 0, false, false
	}

	existingTime, err := ParseDateTime(fmt.Sprintf("%v", existingDate))
	if err != nil {
		return "", 0, false, false
	}

	offsetSeconds := int(existingTime.Sub(immichTime).Seconds())
	if !OffsetSecondsArePlausible(offsetSeconds) {
		return "", 0, false, false
	}

	offsetStr, tzHours, hasWholeHour := BuildOffsetValues(offsetSeconds)
	return offsetStr, tzHours, hasWholeHour, true
}

func DateTimeMatch(existingVal, existingOffset, existingTZOffset any, expected string) bool {
	if existingVal == nil {
		return false
	}
	exifStr, ok := existingVal.(string)
	if !ok {
		return false
	}

	expectedTime, err := ParseDateTime(expected)
	if err != nil {
		return strings.TrimSpace(exifStr) == strings.TrimSpace(expected)
	}

	if offsetStr, ok := existingOffset.(string); ok {
		exifStr = exifStr + strings.TrimSpace(offsetStr)
	} else if tzOffset, ok := existingTZOffset.(float64); ok {
		hours := int(tzOffset)
		exifStr = exifStr + fmt.Sprintf("%+03d:00", hours)
	}

	existingTime, err := ParseDateTime(exifStr)
	if err != nil {
		return false
	}

	return existingTime.Truncate(time.Second).Equal(expectedTime.Truncate(time.Second))
}
