package exif

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/model"
)

type TagChange struct {
	Diffs []model.DiffEntry
	Args  []string
}

func CompareMetadata(exif *model.ExifInfo, existing ExifTagMap) []TagChange {
	if exif == nil {
		return nil
	}
	var changes []TagChange

	appendIfChanged := func(tc *TagChange) {
		if tc != nil {
			changes = append(changes, *tc)
		}
	}

	appendIfChanged(CompareGPS(exif, existing))
	appendIfChanged(CompareDescription(exif, existing))
	appendIfChanged(CompareRating(exif, existing))

	appendIfChanged(CompareLocation("City", exif.City,
		[]string{"IPTC:City", "XMP-photoshop:City"}, []string{"City"},
		"-IPTC:City=%s", "-XMP-photoshop:City=%s", existing))

	appendIfChanged(CompareLocation("State", exif.State,
		[]string{"IPTC:Province-State", "XMP-photoshop:State"}, []string{"Province-State", "State"},
		"-IPTC:Province-State=%s", "-XMP-photoshop:State=%s", existing))

	appendIfChanged(CompareLocation("Country", exif.Country,
		[]string{"IPTC:Country-PrimaryLocationName", "XMP-photoshop:Country"}, []string{"Country-PrimaryLocationName", "Country"},
		"-IPTC:Country-PrimaryLocationName=%s", "-XMP-photoshop:Country=%s", existing))

	appendIfChanged(CompareDateTime(exif, existing))
	appendIfChanged(CompareSimpleString("Make", "Make", exif.Make, existing, true))
	appendIfChanged(CompareSimpleString("Model", "Model", exif.Model, existing, true))
	appendIfChanged(CompareSimpleString("LensModel", "LensModel", exif.LensModel, existing, true))

	return changes
}

func CompareGPS(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.Latitude == nil || exif.Longitude == nil {
		return nil
	}
	lat := *exif.Latitude
	lon := *exif.Longitude
	exifMatch := FloatMatch(existing["GPSLatitude"], lat) && FloatMatch(existing["GPSLongitude"], lon)
	xmpMatch := FloatMatch(existing["XMP-exif:GPSLatitude"], lat) && FloatMatch(existing["XMP-exif:GPSLongitude"], lon)
	if exifMatch && xmpMatch {
		return nil
	}

	latRef := "N"
	if lat < 0 {
		latRef = "S"
	}
	lonRef := "E"
	if lon < 0 {
		lonRef = "W"
	}

	tc := &TagChange{
		Args: []string{
			fmt.Sprintf("-GPSLatitude=%f", math.Abs(lat)),
			fmt.Sprintf("-GPSLatitudeRef=%s", latRef),
			fmt.Sprintf("-GPSLongitude=%f", math.Abs(lon)),
			fmt.Sprintf("-GPSLongitudeRef=%s", lonRef),
			fmt.Sprintf("-XMP-exif:GPSLatitude=%f", lat),
			fmt.Sprintf("-XMP-exif:GPSLongitude=%f", lon),
		},
	}
	tc.Diffs = appendFloatDiff(tc.Diffs, "GPS Latitude", existing["GPSLatitude"], lat)
	tc.Diffs = appendFloatDiff(tc.Diffs, "GPS Longitude", existing["GPSLongitude"], lon)
	tc.Diffs = appendFloatDiff(tc.Diffs, "XMP-exif:GPSLatitude", existing["XMP-exif:GPSLatitude"], lat)
	tc.Diffs = appendFloatDiff(tc.Diffs, "XMP-exif:GPSLongitude", existing["XMP-exif:GPSLongitude"], lon)
	return tc
}

func CompareDescription(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.Description == nil || *exif.Description == "" {
		return nil
	}
	desc := *exif.Description
	if StringMatch(existing["ImageDescription"], desc) &&
		StringMatch(existing["XPComment"], desc) &&
		StringMatch(existing["XMP-dc:Description"], desc) &&
		StringMatch(existing["IPTC:Caption-Abstract"], desc) {
		return nil
	}

	tc := &TagChange{
		Args: []string{
			fmt.Sprintf("-ImageDescription=%s", desc),
			fmt.Sprintf("-XPComment=%s", desc),
			fmt.Sprintf("-XMP-dc:Description=%s", desc),
			fmt.Sprintf("-IPTC:Caption-Abstract=%s", desc),
		},
	}
	tc.Diffs = appendStringDiff(tc.Diffs, "ImageDescription", existing["ImageDescription"], desc)
	tc.Diffs = appendStringDiff(tc.Diffs, "XPComment", existing["XPComment"], desc)
	tc.Diffs = appendStringDiff(tc.Diffs, "XMP-dc:Description", existing["XMP-dc:Description"], desc)
	tc.Diffs = appendStringDiff(tc.Diffs, "IPTC:Caption-Abstract", existing["IPTC:Caption-Abstract"], desc)
	return tc
}

func CompareRating(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.Rating == nil || *exif.Rating == 0 {
		return nil
	}
	rating := *exif.Rating
	ratingPercent := rating * 20
	writePercent := rating > 0

	if IntMatch(existing["Rating"], rating) &&
		(!writePercent || IntMatch(existing["RatingPercent"], ratingPercent)) &&
		IntMatch(existing["XMP-xmp:Rating"], rating) {
		return nil
	}

	tc := &TagChange{}
	tc.Args = append(tc.Args, fmt.Sprintf("-Rating=%d", rating))
	if writePercent {
		tc.Args = append(tc.Args, fmt.Sprintf("-RatingPercent=%d", ratingPercent))
	}
	tc.Args = append(tc.Args, fmt.Sprintf("-XMP-xmp:Rating=%d", rating))

	tc.Diffs = appendIntDiff(tc.Diffs, "Rating", existing["Rating"], rating)
	if writePercent {
		tc.Diffs = appendIntDiff(tc.Diffs, "RatingPercent", existing["RatingPercent"], ratingPercent)
	}
	tc.Diffs = appendIntDiff(tc.Diffs, "XMP-xmp:Rating", existing["XMP-xmp:Rating"], rating)
	return tc
}

func CompareLocation(label string, value *string, strictKeys, fallbackKeys []string, iptcArg, xmpArg string, existing ExifTagMap) *TagChange {
	if value == nil || *value == "" {
		return nil
	}
	val := *value
	if AllLocationTagValuesMatch(existing, strictKeys, fallbackKeys, val) {
		return nil
	}

	tc := &TagChange{
		Args: []string{
			fmt.Sprintf(iptcArg, val),
			fmt.Sprintf(xmpArg, val),
		},
	}

	useStrict := false
	for _, key := range strictKeys {
		if existing[key] != nil {
			useStrict = true
			break
		}
	}
	if useStrict {
		for _, key := range strictKeys {
			tc.Diffs = appendStringDiff(tc.Diffs, label+" ("+key+")", existing[key], val)
		}
	} else {
		var old any
		for _, k := range fallbackKeys {
			if v := existing[k]; v != nil {
				old = v
				break
			}
		}
		tc.Diffs = appendStringDiff(tc.Diffs, label, old, val)
	}

	return tc
}

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

func CompareSimpleString(tag, exifKey string, value *string, existing ExifTagMap, onlyIfEmpty bool) *TagChange {
	if value == nil || *value == "" {
		return nil
	}
	if onlyIfEmpty && existing[exifKey] != nil {
		if s, ok := existing[exifKey].(string); !ok || strings.TrimSpace(s) != "" {
			return nil
		}
	}
	if StringMatch(existing[exifKey], *value) {
		return nil
	}
	return &TagChange{
		Diffs: appendStringDiff(nil, tag, existing[exifKey], *value),
		Args:  []string{fmt.Sprintf("-%s=%s", exifKey, *value)},
	}
}

func CollectExifArgs(changes []TagChange) []string {
	var args []string
	for _, c := range changes {
		args = append(args, c.Args...)
	}
	return args
}

func CollectDiffEntries(changes []TagChange) []model.DiffEntry {
	var entries []model.DiffEntry
	for _, c := range changes {
		entries = append(entries, c.Diffs...)
	}
	return entries
}

func appendDiff(diffs []model.DiffEntry, tag string, existing any, expected string, matchFn func(any, string) bool) []model.DiffEntry {
	if existing == nil {
		return append(diffs, model.DiffEntry{Tag: tag, Symbol: model.DiffAdd, Old: "(none)", New: expected})
	}
	if !matchFn(existing, expected) {
		return append(diffs, model.DiffEntry{Tag: tag, Symbol: model.DiffChange, Old: fmt.Sprintf("%v", existing), New: expected})
	}
	return diffs
}

func appendFloatDiff(diffs []model.DiffEntry, tag string, existing any, expected float64) []model.DiffEntry {
	return appendDiff(diffs, tag, existing, fmt.Sprintf("%f", expected), func(e any, s string) bool { return FloatMatch(e, expected) })
}

func appendStringDiff(diffs []model.DiffEntry, tag string, existing any, expected string) []model.DiffEntry {
	return appendDiff(diffs, tag, existing, expected, func(e any, s string) bool { return StringMatch(e, s) })
}

func appendIntDiff(diffs []model.DiffEntry, tag string, existing any, expected int) []model.DiffEntry {
	return appendDiff(diffs, tag, existing, fmt.Sprintf("%d", expected), func(e any, s string) bool { return IntMatch(e, expected) })
}
