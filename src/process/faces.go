package process

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func wantsFaceRegions(cfg *model.Config, asset model.AssetResponse) bool {
	return cfg.Faces && model.HasFaceRegionsToEmbed(asset)
}

// appendFaceRegionChange fetches the asset's face boxes and appends the
// region rewrite when the file disagrees with Immich, returning the regions
// being embedded (nil when the file already matches) so they can be re-checked
// before upload. The file's own orientation and pixel dimensions anchor the
// regions, so this must run after the exif read. A file that reports no pixel
// dimensions, or a video whose rotation cannot be safely anchored, gets no
// regions rather than misanchored ones.
func appendFaceRegionChange(client *api.ImmichClient, cfg *model.Config, asset model.AssetResponse, existing exif.ExifTagMap, changes []exif.TagChange) ([]exif.TagChange, []exif.FaceRegion, error) {
	if !wantsFaceRegions(cfg, asset) {
		return changes, nil, nil
	}
	orientation, ok := regionOrientation(asset, existing)
	if !ok {
		return changes, nil, nil
	}
	faces, err := client.GetAssetFaces(asset.ID)
	if err != nil {
		if isPermissionDenied(err) {
			return nil, nil, fmt.Errorf("faces read denied — the -faces flag needs the API key's face.read permission: %w", err)
		}
		return nil, nil, err
	}
	regions := exif.BuildFaceRegions(faces, orientation)
	change := exif.CompareFaceRegions(regions, intTag(existing, "ImageWidth"), intTag(existing, "ImageHeight"), existing)
	if change == nil {
		return changes, nil, nil
	}
	changes = append(changes, *change)
	return changes, regions, nil
}

// regionOrientation returns the EXIF-orientation value to anchor face regions
// with, and whether they can be anchored at all. Images use their EXIF
// Orientation. Videos have no EXIF Orientation; their display rotation is read
// from the QuickTime Rotation tag and mapped to the equivalent orientation.
func regionOrientation(asset model.AssetResponse, existing exif.ExifTagMap) (int, bool) {
	if model.IsVideoAsset(asset) {
		return videoRotationToOrientation(intTag(existing, "Rotation"))
	}
	return intTag(existing, "Orientation"), true
}

// videoRotationToOrientation maps a QuickTime display rotation to the EXIF
// orientation whose inverse transform Immich applies to video regions on
// import (verified against a live server): 0->1, 90->6, 270->8. A 180 or
// non-cardinal rotation returns ok=false — Immich does not re-orient 180 video
// regions the way it does 90/270, so those are skipped rather than misanchored.
func videoRotationToOrientation(rotation int) (int, bool) {
	switch ((rotation % 360) + 360) % 360 {
	case 0:
		return 1, true
	case 90:
		return 6, true
	case 270:
		return 8, true
	}
	return 0, false
}

// statusCodeOf extracts the HTTP status from an API error, reporting false when
// the error is not an *api.StatusError (e.g. a transport failure).
func statusCodeOf(err error) (int, bool) {
	var status *api.StatusError
	if !errors.As(err, &status) {
		return 0, false
	}
	return status.StatusCode, true
}

// isPermissionDenied reports whether an API error carries a 401/403 status, the
// signature of an API key missing the face.read permission.
func isPermissionDenied(err error) bool {
	code, ok := statusCodeOf(err)
	return ok && (code == http.StatusUnauthorized || code == http.StatusForbidden)
}

// faceRegionsStale reports whether the server's faces moved away from the
// regions this run is about to embed.
func faceRegionsStale(client *api.ImmichClient, asset model.AssetResponse, existing exif.ExifTagMap, written []exif.FaceRegion) (bool, error) {
	orientation, _ := regionOrientation(asset, existing)
	faces, err := client.GetAssetFaces(asset.ID)
	if err != nil {
		return false, err
	}
	fresh := exif.BuildFaceRegions(faces, orientation)
	return !exif.FaceRegionsMatch(fresh, written, intTag(existing, "ImageWidth"), intTag(existing, "ImageHeight")), nil
}

// intTag reads a numeric exiftool tag (-n makes them JSON numbers).
func intTag(existing exif.ExifTagMap, key string) int {
	value, ok := existing[key].(float64)
	if !ok {
		return 0
	}
	return int(value)
}
