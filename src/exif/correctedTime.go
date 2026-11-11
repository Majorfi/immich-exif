package exif

import (
	"time"

	"github.com/majorfi/immich-exif/model"
)

// CorrectedLocalTime returns the asset's capture moment as wall-clock time in
// its own time zone, matching the value this tool embeds into DateTimeOriginal.
// It re-anchors Immich's UTC-serialized dateTimeOriginal onto exifInfo.timeZone
// exactly as CompareDateTime does, so a filename built from it agrees with the
// EXIF written into the same file. ok is false when the server has no usable
// date and the caller should keep the original filename.
func CorrectedLocalTime(info *model.ExifInfo) (time.Time, bool) {
	date, ok := immichDateOf(info)
	return date.local, ok
}
