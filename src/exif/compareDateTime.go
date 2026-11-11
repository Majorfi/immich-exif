package exif

import (
	"fmt"

	"github.com/majorfi/immich-exif/model"
)

// CompareDateTime writes Immich's date anchored on the asset's zone. Without a
// zone, a file date lacking an offset is the only clue to it, so the offset is
// inferred from its wall clock. With one, Immich's zone wins: a wall clock or
// offset left behind by a date edited in Immich is rewritten, not reinterpreted.
func CompareDateTime(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.DateTimeOriginal == nil || *exif.DateTimeOriginal == "" {
		return nil
	}
	expected := *exif.DateTimeOriginal

	date, ok := immichDateOf(exif)
	if !ok {
		tc := &TagChange{}
		if !StringMatch(existing["DateTimeOriginal"], expected) {
			tc.Args = append(tc.Args, fmt.Sprintf("-DateTimeOriginal=%s", expected))
			tc.Diffs = appendStringDiff(tc.Diffs, "DateTimeOriginal", existing["DateTimeOriginal"], expected)
		}
		appendDateArgs(tc, xmpDateTags, expected, existing, nil)
		if len(tc.Args) == 0 {
			return nil
		}
		return tc
	}
	if date.anchored {
		expected = date.local.Format("2006-01-02T15:04:05.000-07:00")
	}

	existingDate, _ := existing["DateTimeOriginal"].(string)
	existingHasOffset := existing["OffsetTimeOriginal"] != nil || existing["TimeZoneOffset"] != nil || hasInlineOffset(existingDate)
	var tc *TagChange
	if existing["DateTimeOriginal"] != nil && !existingHasOffset && (!date.anchored || date.matches(existingDate, false, false)) {
		tc = compareDateTimeMissingOffset(existing, date, expected)
	} else {
		tc = compareDateTimeFull(existing, date)
	}

	if tc == nil {
		tc = &TagChange{}
	}
	appendDateArgs(tc, xmpDateTags, expected, existing, &date)
	if len(tc.Args) == 0 {
		return nil
	}
	return tc
}

var xmpDateTags = []string{"XMP-exif:DateTimeOriginal", "XMP-xmp:CreateDate"}

func appendDateArgs(tc *TagChange, tags []string, isoDate string, existing ExifTagMap, date *immichDate) {
	for _, tag := range tags {
		var matched bool
		if date != nil {
			value, _ := existing[tag].(string)
			matched = date.matches(value, true, true)
		} else {
			matched = DateTimeStringMatch(existing[tag], isoDate)
		}
		if !matched {
			tc.Args = append(tc.Args, fmt.Sprintf("-%s=%s", tag, isoDate))
			tc.Diffs = appendStringDiff(tc.Diffs, tag, existing[tag], isoDate)
		}
	}
}

func compareDateTimeMissingOffset(existing ExifTagMap, date immichDate, expected string) *TagChange {
	offsetStr, tzHours, hasWholeHour, canInfer := DeriveOffsetValuesForMissingOffset(existing, expected)
	if !canInfer {
		return compareDateTimeFull(existing, date)
	}

	tc := &TagChange{}
	if !StringMatch(existing["OffsetTimeOriginal"], offsetStr) {
		tc.Args = append(tc.Args, fmt.Sprintf("-OffsetTimeOriginal=%s", offsetStr))
		tc.Diffs = appendStringDiff(tc.Diffs, "OffsetTimeOriginal", existing["OffsetTimeOriginal"], offsetStr)
	}
	if hasWholeHour && !IntMatch(existing["TimeZoneOffset"], tzHours) {
		tc.Args = append(tc.Args, fmt.Sprintf("-TimeZoneOffset=%d", tzHours))
		tc.Diffs = appendIntDiff(tc.Diffs, "TimeZoneOffset", existing["TimeZoneOffset"], tzHours)
	}

	if len(tc.Args) == 0 {
		return nil
	}
	return tc
}

func compareDateTimeFull(existing ExifTagMap, date immichDate) *TagChange {
	_, offsetSeconds := date.local.Zone()
	offsetStr, tzHours, hasWholeHour := BuildOffsetValues(offsetSeconds)
	expectedDate := date.local.Format("2006:01:02 15:04:05")

	oldVal := existing["DateTimeOriginal"]
	oldOffset := existing["OffsetTimeOriginal"]
	oldTZOffset := existing["TimeZoneOffset"]
	oldDate, isString := oldVal.(string)
	momentMatches := isString && date.matches(withSeparateOffset(existing, oldDate), true, false) && separateOffsetsAgree(existing, oldDate)

	tc := &TagChange{}
	if !momentMatches {
		tc.Args = append(tc.Args, fmt.Sprintf("-DateTimeOriginal=%s", expectedDate))
		tc.Args = append(tc.Args, fmt.Sprintf("-OffsetTimeOriginal=%s", offsetStr))
		tc.Diffs = appendStringDiff(tc.Diffs, "DateTimeOriginal", oldVal, expectedDate)
		tc.Diffs = appendStringDiff(tc.Diffs, "OffsetTimeOriginal", oldOffset, offsetStr)
		if hasWholeHour && !IntMatch(oldTZOffset, tzHours) {
			tc.Args = append(tc.Args, fmt.Sprintf("-TimeZoneOffset=%d", tzHours))
			tc.Diffs = appendIntDiff(tc.Diffs, "TimeZoneOffset", oldTZOffset, tzHours)
		}
	} else if oldOffset == nil {
		// The instant already matches via the existing TimeZoneOffset, but the
		// explicit OffsetTimeOriginal tag is missing. Backfill it from the offset
		// the file already encodes (never Immich's own zone, which can differ while
		// still matching the instant) so the clock value is not re-anchored.
		if tz, ok := oldTZOffset.(float64); ok {
			consistentOffset := fmt.Sprintf("%+03d:00", int(tz))
			tc.Args = append(tc.Args, fmt.Sprintf("-OffsetTimeOriginal=%s", consistentOffset))
			tc.Diffs = appendStringDiff(tc.Diffs, "OffsetTimeOriginal", oldOffset, consistentOffset)
		}
	} else if StringMatch(oldOffset, offsetStr) && hasWholeHour && !IntMatch(oldTZOffset, tzHours) {
		tc.Args = append(tc.Args, fmt.Sprintf("-TimeZoneOffset=%d", tzHours))
		tc.Diffs = appendIntDiff(tc.Diffs, "TimeZoneOffset", oldTZOffset, tzHours)
	}

	if len(tc.Args) == 0 {
		return nil
	}
	return tc
}
