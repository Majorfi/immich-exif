package exif

import (
	"fmt"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func CompareDateTime(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.DateTimeOriginal == nil || *exif.DateTimeOriginal == "" {
		return nil
	}
	expected := *exif.DateTimeOriginal

	immichTime, err := ParseDateTime(expected)
	if err != nil {
		tc := &TagChange{}
		if !StringMatch(existing["DateTimeOriginal"], expected) {
			tc.Args = append(tc.Args, fmt.Sprintf("-DateTimeOriginal=%s", expected))
			tc.Diffs = appendStringDiff(tc.Diffs, "DateTimeOriginal", existing["DateTimeOriginal"], expected)
		}
		appendXMPDateArgs(tc, expected, existing)
		if len(tc.Args) == 0 {
			return nil
		}
		return tc
	}

	// Immich serializes dateTimeOriginal as UTC ISO; re-anchor it to the asset's
	// own time zone (exifInfo.timeZone) so the wall-clock time written into the
	// file matches what the user sees, instead of collapsing to UTC.
	if loc := resolveImmichLocation(exif.TimeZone); loc != nil {
		immichTime = immichTime.In(loc)
		expected = immichTime.Format("2006-01-02T15:04:05.000-07:00")
	}

	existingHasDate := existing != nil && existing["DateTimeOriginal"] != nil
	existingHasOffset := existing != nil && (existing["OffsetTimeOriginal"] != nil || existing["TimeZoneOffset"] != nil)

	var tc *TagChange
	if existingHasDate && !existingHasOffset {
		tc = compareDateTimeMissingOffset(existing, immichTime, expected)
	} else {
		tc = compareDateTimeFull(existing, immichTime, expected)
	}

	if tc == nil {
		tc = &TagChange{}
	}
	appendXMPDateArgs(tc, expected, existing)
	if len(tc.Args) == 0 {
		return nil
	}
	return tc
}

func appendXMPDateArgs(tc *TagChange, isoDate string, existing ExifTagMap) {
	if !DateTimeStringMatch(existing["XMP-exif:DateTimeOriginal"], isoDate) {
		tc.Args = append(tc.Args, fmt.Sprintf("-XMP-exif:DateTimeOriginal=%s", isoDate))
		tc.Diffs = appendStringDiff(tc.Diffs, "XMP-exif:DateTimeOriginal", existing["XMP-exif:DateTimeOriginal"], isoDate)
	}
	if !DateTimeStringMatch(existing["XMP-xmp:CreateDate"], isoDate) {
		tc.Args = append(tc.Args, fmt.Sprintf("-XMP-xmp:CreateDate=%s", isoDate))
		tc.Diffs = appendStringDiff(tc.Diffs, "XMP-xmp:CreateDate", existing["XMP-xmp:CreateDate"], isoDate)
	}
}

func compareDateTimeMissingOffset(existing ExifTagMap, immichTime time.Time, expected string) *TagChange {
	offsetStr, tzHours, hasWholeHour, canInfer := DeriveOffsetValuesForMissingOffset(existing, expected)
	if !canInfer {
		return compareDateTimeFull(existing, immichTime, expected)
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

func compareDateTimeFull(existing ExifTagMap, immichTime time.Time, expected string) *TagChange {
	_, offsetSeconds := immichTime.Zone()
	offsetStr, tzHours, hasWholeHour := BuildOffsetValues(offsetSeconds)
	expectedDate := immichTime.Format("2006:01:02 15:04:05")

	oldVal := existing["DateTimeOriginal"]
	oldOffset := existing["OffsetTimeOriginal"]
	oldTZOffset := existing["TimeZoneOffset"]

	momentMatches := DateTimeMatch(oldVal, oldOffset, oldTZOffset, expected)

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
