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
// before upload. The file's own Orientation and pixel dimensions anchor the
// regions, so this must run after the exif read. A file that reports no pixel
// dimensions gets no regions rather than misanchored ones.
func appendFaceRegionChange(client *api.ImmichClient, cfg *model.Config, asset model.AssetResponse, existing exif.ExifTagMap, changes []exif.TagChange) ([]exif.TagChange, []exif.FaceRegion, error) {
	if !wantsFaceRegions(cfg, asset) {
		return changes, nil, nil
	}
	faces, err := client.GetAssetFaces(asset.ID)
	if err != nil {
		if isPermissionDenied(err) {
			return nil, nil, fmt.Errorf("faces read denied — the -faces flag needs the API key's face.read permission: %w", err)
		}
		return nil, nil, err
	}
	regions := exif.BuildFaceRegions(faces, intTag(existing, "Orientation"))
	change := exif.CompareFaceRegions(regions, intTag(existing, "ImageWidth"), intTag(existing, "ImageHeight"), existing)
	if change == nil {
		return changes, nil, nil
	}
	changes = append(changes, *change)
	return changes, regions, nil
}

// isPermissionDenied reports whether an API error carries a 401/403 status, the
// signature of an API key missing the face.read permission.
func isPermissionDenied(err error) bool {
	var status *api.StatusError
	if !errors.As(err, &status) {
		return false
	}
	return status.StatusCode == http.StatusUnauthorized || status.StatusCode == http.StatusForbidden
}

// faceRegionsStale reports whether the server's faces moved away from the
// regions this run is about to embed.
func faceRegionsStale(client *api.ImmichClient, assetID string, existing exif.ExifTagMap, written []exif.FaceRegion) (bool, error) {
	faces, err := client.GetAssetFaces(assetID)
	if err != nil {
		return false, err
	}
	fresh := exif.BuildFaceRegions(faces, intTag(existing, "Orientation"))
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
