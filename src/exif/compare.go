package exif

import (
	"fmt"
	"math"
	"strings"

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
	if exif.Rating == nil {
		return nil
	}
	rating := *exif.Rating
	// Immich reports rating as null when never set and 0 when the user cleared
	// it, so an explicit 0 means the file's stale rating must be removed.
	if rating == 0 {
		return clearRatingTags(existing)
	}
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

func clearRatingTags(existing ExifTagMap) *TagChange {
	tc := &TagChange{}
	for _, key := range []string{"Rating", "RatingPercent", "XMP-xmp:Rating"} {
		value := existing[key]
		if value == nil {
			continue
		}
		if IntMatch(value, 0) {
			continue
		}
		tc.Args = append(tc.Args, fmt.Sprintf("-%s=", key))
		tc.Diffs = append(tc.Diffs, model.DiffEntry{Tag: key, Symbol: model.DiffChange, Old: fmt.Sprintf("%v", value), New: "(cleared)"})
	}
	if len(tc.Args) == 0 {
		return nil
	}
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

	if hasAnyTagValue(existing, strictKeys) {
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
