package exif

import (
	"fmt"

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
	if DateTimeStringMatch(existing["DateTimeOriginal"], expected) &&
		DateTimeStringMatch(existing["XMP-exif:DateTimeOriginal"], expected) &&
		DateTimeStringMatch(existing["XMP-xmp:CreateDate"], expected) {
		return nil
	}

	change := &TagChange{}
	if !DateTimeStringMatch(existing["DateTimeOriginal"], expected) {
		change.Args = append(change.Args, fmt.Sprintf("-DateTimeOriginal=%s", expected))
		change.Diffs = appendDateTimeDiff(change.Diffs, "DateTimeOriginal", existing["DateTimeOriginal"], expected)
	}
	if !DateTimeStringMatch(existing["XMP-exif:DateTimeOriginal"], expected) {
		change.Args = append(change.Args, fmt.Sprintf("-XMP-exif:DateTimeOriginal=%s", expected))
		change.Diffs = appendDateTimeDiff(change.Diffs, "XMP-exif:DateTimeOriginal", existing["XMP-exif:DateTimeOriginal"], expected)
	}
	if !DateTimeStringMatch(existing["XMP-xmp:CreateDate"], expected) {
		change.Args = append(change.Args, fmt.Sprintf("-XMP-xmp:CreateDate=%s", expected))
		change.Diffs = appendDateTimeDiff(change.Diffs, "XMP-xmp:CreateDate", existing["XMP-xmp:CreateDate"], expected)
	}
	if len(change.Args) == 0 {
		return nil
	}
	return change
}

func appendDateTimeDiff(diffs []model.DiffEntry, tag string, existing any, expected string) []model.DiffEntry {
	return appendDiff(diffs, tag, existing, expected, func(existing any, expected string) bool {
		return DateTimeStringMatch(existing, expected)
	})
}
