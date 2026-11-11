package process

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

// sidecarNote reports a sidecar Immich would apply over the rewritten file
// with a value its database does not hold: a different one, or one Immich has
// cleared. Immich writes its own sidecar next to external files edited in
// Immich; that one agrees and passes. City, state and country are left out:
// Immich derives them from GPS, not from tags. Dates are judged separately,
// by the instant Immich would read.
func sidecarNote(client *api.ImmichClient, cfg *model.Config, asset model.AssetResponse, local string) (string, error) {
	registered, err := client.GetSidecarPaths(asset.ID)
	if code, ok := statusCodeOf(err); ok && code == http.StatusNotFound {
		return "listing an asset's sidecars needs Immich 3.2 or later", nil
	}
	if isPermissionDenied(err) {
		return "", fmt.Errorf("listing sidecars was denied, -in-place-map needs the API key's asset.file.read permission: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("list sidecars: %w", err)
	}
	candidates := []string{local + ".xmp", strings.TrimSuffix(local, filepath.Ext(local)) + ".xmp"}
	for _, serverPath := range registered {
		mapped, _, ok := cfg.MapServerPath(serverPath)
		if !ok {
			return fmt.Sprintf("its sidecar %s is outside the mapped paths", serverPath), nil
		}
		candidates = append(candidates, mapped)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", err
		}
		tags, err := exif.ReadExifTagsFn(candidate)
		if err != nil {
			return "", fmt.Errorf("read sidecar %s: %w", candidate, err)
		}
		if note := sidecarDisagreement(cfg, asset, tags); note != "" {
			return fmt.Sprintf("sidecar %s %s, and Immich would apply it over the file", filepath.Base(candidate), note), nil
		}
	}
	return "", nil
}

func sidecarDisagreement(cfg *model.Config, asset model.AssetResponse, tags exif.ExifTagMap) string {
	if cfg.Faces && tags["XMP-mwg-rs:RegionInfo"] != nil {
		return "holds face regions"
	}
	for _, change := range exif.CompareAssetMetadata(asset, tags) {
		for _, diff := range change.Diffs {
			if diff.Symbol == model.DiffChange && !isLocationTag(diff.Tag) && !slices.Contains(dateDiffTags, diff.Tag) {
				return fmt.Sprintf("disagrees with Immich on %s (%s, Immich has %s)", diff.Tag, diff.Old, diff.New)
			}
		}
	}
	info := asset.ExifInfo
	if info == nil {
		info = &model.ExifInfo{}
	}
	lacking := []struct {
		name      string
		inSidecar bool
		inImmich  bool
	}{
		{"a description", hasText(tags, "Description", "ImageDescription", "XMP-dc:Description"), info.Description != nil && *info.Description != ""},
		{"a GPS position", tags["GPSLatitude"] != nil || tags["XMP-exif:GPSLatitude"] != nil, info.Latitude != nil},
		{"a rating", hasNonZero(tags, "Rating", "XMP-xmp:Rating"), info.Rating != nil && *info.Rating != 0},
	}
	for _, field := range lacking {
		if field.inSidecar && !field.inImmich {
			return "holds " + field.name + " that Immich does not have"
		}
	}
	return sidecarDateDisagreement(info, tags)
}

// sidecarDateDisagreement compares the date Immich would take from the
// sidecar, the first of its date tags in Immich's order, with Immich's own:
// as an instant when the sidecar date carries an offset, as wall-clock time in
// the asset's zone when it does not, which is how Immich reads either.
func sidecarDateDisagreement(info *model.ExifInfo, tags exif.ExifTagMap) string {
	var date string
	for _, key := range immichDateTags {
		if value := tags[key]; value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			date = strings.TrimSpace(fmt.Sprint(value))
			break
		}
	}
	if date == "" {
		return ""
	}
	if !exif.HasInlineOffset(date) {
		if offset, ok := tags["OffsetTimeOriginal"].(string); ok && strings.TrimSpace(offset) != "" {
			date += strings.TrimSpace(offset)
		} else if hours, ok := tags["TimeZoneOffset"].(float64); ok {
			date += fmt.Sprintf("%+03d:00", int(hours))
		}
	}
	if info.DateTimeOriginal == nil || *info.DateTimeOriginal == "" {
		return "holds a capture date that Immich does not have"
	}
	sidecarTime, sidecarErr := exif.ParseDateTime(date)
	immichTime, immichErr := exif.ParseDateTime(*info.DateTimeOriginal)
	local, ok := exif.CorrectedLocalTime(info)
	if sidecarErr == nil && immichErr == nil && ok {
		if exif.HasInlineOffset(date) && sidecarTime.Truncate(time.Second).Equal(immichTime.Truncate(time.Second)) {
			return ""
		}
		if !exif.HasInlineOffset(date) && sidecarTime.Format("2006-01-02 15:04:05") == local.Format("2006-01-02 15:04:05") {
			return ""
		}
	}
	return fmt.Sprintf("holds capture date %s while Immich has %s", date, *info.DateTimeOriginal)
}

var dateDiffTags = []string{"DateTimeOriginal", "OffsetTimeOriginal", "TimeZoneOffset", "XMP-exif:DateTimeOriginal", "XMP-xmp:CreateDate"}

var immichDateTags = []string{
	"SubSecDateTimeOriginal", "SubSecCreateDate", "DateTimeOriginal", "CreationDate", "CreateDate",
	"MediaCreateDate", "DateTimeCreated", "GPSDateTime", "DateTimeUTC", "SonyDateTime2", "SourceImageCreateTime",
}

func hasText(tags exif.ExifTagMap, keys ...string) bool {
	for _, key := range keys {
		if value := tags[key]; value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
}

func hasNonZero(tags exif.ExifTagMap, keys ...string) bool {
	for _, key := range keys {
		if value, ok := tags[key].(float64); ok && value != 0 {
			return true
		}
	}
	return false
}

func isLocationTag(tag string) bool {
	for _, word := range []string{"City", "State", "Province", "Country"} {
		if strings.Contains(tag, word) {
			return true
		}
	}
	return false
}
