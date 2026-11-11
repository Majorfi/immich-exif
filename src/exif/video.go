package exif

import (
	"fmt"
	"strings"

	"github.com/majorfi/immich-exif/model"
)

func CompareAssetMetadata(asset model.AssetResponse, existing ExifTagMap) []TagChange {
	if model.IsVideoAsset(asset) {
		if model.IsUnsupportedVideoAsset(asset) {
			return nil
		}
		return compareVideoMetadata(asset.ExifInfo, existing)
	}
	return CompareMetadata(asset.ExifInfo, existing)
}

func HasAssetMetadataToEmbed(asset model.AssetResponse) bool {
	if model.IsUnsupportedVideoAsset(asset) {
		return false
	}
	return len(CollectExifArgs(CompareAssetMetadata(asset, nil))) > 0
}

func compareVideoMetadata(exif *model.ExifInfo, existing ExifTagMap) []TagChange {
	if exif == nil {
		return nil
	}

	var changes []TagChange
	appendIfChanged := func(change *TagChange) {
		if change != nil {
			changes = append(changes, *change)
		}
	}

	appendIfChanged(CompareGPS(exif, existing))
	appendIfChanged(compareVideoDescription(exif, existing))
	appendIfChanged(CompareRating(exif, existing))
	appendIfChanged(compareVideoLocation("City", exif.City, "XMP-photoshop:City", "City", "-XMP-photoshop:City=%s", existing))
	appendIfChanged(compareVideoLocation("State", exif.State, "XMP-photoshop:State", "State", "-XMP-photoshop:State=%s", existing))
	appendIfChanged(compareVideoLocation("Country", exif.Country, "XMP-photoshop:Country", "Country", "-XMP-photoshop:Country=%s", existing))
	appendIfChanged(compareVideoDateTime(exif, existing))
	appendIfChanged(CompareSimpleString("Make", "Make", exif.Make, existing, true))
	appendIfChanged(CompareSimpleString("Model", "Model", exif.Model, existing, true))
	appendIfChanged(CompareSimpleString("LensModel", "LensModel", exif.LensModel, existing, true))

	return changes
}

func compareVideoDescription(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.Description == nil || *exif.Description == "" {
		return nil
	}

	description := *exif.Description
	if StringMatch(existing["Description"], description) &&
		StringMatch(existing["ImageDescription"], description) &&
		StringMatch(existing["XMP-dc:Description"], description) {
		return nil
	}

	change := &TagChange{
		Args: []string{
			fmt.Sprintf("-Description=%s", description),
			fmt.Sprintf("-ImageDescription=%s", description),
			fmt.Sprintf("-XMP-dc:Description=%s", description),
		},
	}
	change.Diffs = appendStringDiff(change.Diffs, "Description", existing["Description"], description)
	change.Diffs = appendStringDiff(change.Diffs, "ImageDescription", existing["ImageDescription"], description)
	change.Diffs = appendStringDiff(change.Diffs, "XMP-dc:Description", existing["XMP-dc:Description"], description)
	return change
}

func compareVideoLocation(label string, value *string, xmpKey, fallbackKey, xmpArg string, existing ExifTagMap) *TagChange {
	if value == nil || *value == "" {
		return nil
	}

	expected := *value
	if StringMatch(existing[xmpKey], expected) {
		return nil
	}

	tag := label + " (" + xmpKey + ")"
	oldValue := existing[xmpKey]
	if oldValue == nil {
		tag = label
		oldValue = existing[fallbackKey]
	}

	change := &TagChange{
		Args: []string{fmt.Sprintf(xmpArg, expected)},
	}
	change.Diffs = appendStringDiff(change.Diffs, tag, oldValue, expected)
	return change
}

func compareVideoDateTime(exif *model.ExifInfo, existing ExifTagMap) *TagChange {
	if exif.DateTimeOriginal == nil || *exif.DateTimeOriginal == "" {
		return nil
	}

	expected := *exif.DateTimeOriginal
	anchored := false
	if t, ok := CorrectedLocalTime(exif); ok && resolveImmichLocation(exif.TimeZone) != nil {
		expected, anchored = t.Format("2006-01-02T15:04:05.999-07:00"), true
	}

	change := &TagChange{}
	for _, tag := range []string{"DateTimeOriginal", "XMP-exif:DateTimeOriginal", "XMP-xmp:CreateDate"} {
		value, _ := existing[tag].(string)
		if DateTimeStringMatch(existing[tag], expected) && (!anchored || sameOffset(value, expected)) {
			continue
		}
		change.Args = append(change.Args, fmt.Sprintf("-%s=%s", tag, expected))
		change.Diffs = appendStringDiff(change.Diffs, tag, existing[tag], expected)
	}
	if len(change.Args) == 0 {
		return nil
	}
	return change
}

func sameOffset(existing, expected string) bool {
	existingTime, err := ParseDateTime(strings.TrimSpace(existing))
	if err != nil {
		return false
	}
	expectedTime, err := ParseDateTime(expected)
	if err != nil {
		return false
	}
	_, existingOffset := existingTime.Zone()
	_, expectedOffset := expectedTime.Zone()
	return existingOffset == expectedOffset
}
