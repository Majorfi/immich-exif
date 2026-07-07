package process

import (
	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func wantsFaceRegions(cfg *model.Config, asset model.AssetResponse) bool {
	return cfg.Faces && model.HasFaceRegionsToEmbed(asset)
}

// appendFaceRegionChange fetches the asset's face boxes and appends the
// region rewrite when the file disagrees with Immich. The file's own
// Orientation and pixel dimensions anchor the regions, so this must run after
// the exif read. A file that reports no pixel dimensions gets no regions
// rather than misanchored ones.
func appendFaceRegionChange(client *api.ImmichClient, cfg *model.Config, asset model.AssetResponse, existing exif.ExifTagMap, changes []exif.TagChange) ([]exif.TagChange, error) {
	if !wantsFaceRegions(cfg, asset) {
		return changes, nil
	}
	faces, err := client.GetAssetFaces(asset.ID)
	if err != nil {
		return nil, err
	}
	regions := exif.BuildFaceRegions(faces, intTag(existing, "Orientation"))
	change := exif.CompareFaceRegions(regions, intTag(existing, "ImageWidth"), intTag(existing, "ImageHeight"), existing)
	if change != nil {
		changes = append(changes, *change)
	}
	return changes, nil
}

// intTag reads a numeric exiftool tag (-n makes them JSON numbers).
func intTag(existing exif.ExifTagMap, key string) int {
	value, ok := existing[key].(float64)
	if !ok {
		return 0
	}
	return int(value)
}
