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
// signature of an API key missing a face permission (face.read for the GET,
// face.create for the POST).
func isPermissionDenied(err error) bool {
	code, ok := statusCodeOf(err)
	return ok && (code == http.StatusUnauthorized || code == http.StatusForbidden)
}

// isNotFound reports whether an API error carries a 404, which for the faces
// endpoint means the server predates manual face tagging (Immich 1.127).
func isNotFound(err error) bool {
	code, ok := statusCodeOf(err)
	return ok && code == http.StatusNotFound
}

// errFacePreserveUnsupported marks a server too old for the createFace endpoint
// (introduced with manual face tagging in Immich 1.127). The caller warns and
// keeps going instead of failing the replacement.
var errFacePreserveUnsupported = errors.New("server does not support the faces endpoint (needs Immich 1.127+)")

// recreateFaces re-links the source asset's assigned people onto the target
// asset via POST /faces. It backs the video path of -faces: a video's regions
// cannot be embedded as metadata, and on re-upload Immich re-detects boxes on
// the new thumbnail but drops the person link. Every face carrying a person is
// recreated verbatim (named or not — createFace requires a personId, so faces
// with none are skipped); a person already linked on the target is left alone so
// repeated runs do not stack duplicates. Returns the number of faces created.
func recreateFaces(client *api.ImmichClient, sourceID, targetID string) (int, error) {
	// A 404 on this first call means the server predates the faces endpoint
	// (both the GET and the POST arrived with manual tagging in 1.127); the
	// source asset always exists, so it is never a missing-asset 404. Skip with a
	// warning instead of failing the replace. Past this point the endpoint is
	// proven to exist, so a later 404 is a real error and stays fatal.
	sourceFaces, err := client.GetAssetFaces(sourceID)
	if err != nil {
		if isNotFound(err) {
			return 0, errFacePreserveUnsupported
		}
		return 0, err
	}
	pending := facesWithPerson(sourceFaces)
	if len(pending) == 0 {
		return 0, nil
	}

	targetFaces, err := client.GetAssetFaces(targetID)
	if err != nil {
		return 0, err
	}
	linked := linkedPersonIDs(targetFaces)

	created := 0
	for _, face := range pending {
		if linked[face.Person.ID] {
			continue
		}
		req := model.CreateFaceRequest{
			AssetID:     targetID,
			PersonID:    face.Person.ID,
			X:           face.BoundingBoxX1,
			Y:           face.BoundingBoxY1,
			Width:       face.BoundingBoxX2 - face.BoundingBoxX1,
			Height:      face.BoundingBoxY2 - face.BoundingBoxY1,
			ImageWidth:  face.ImageWidth,
			ImageHeight: face.ImageHeight,
		}
		if err := client.CreateFace(req); err != nil {
			if isNotFound(err) {
				return created, errFacePreserveUnsupported
			}
			return created, err
		}
		linked[face.Person.ID] = true
		created++
	}
	return created, nil
}

// facesWithPerson keeps only the faces Immich has assigned to a person; a face
// without one has no personId to re-link.
func facesWithPerson(faces []model.AssetFaceResponse) []model.AssetFaceResponse {
	var kept []model.AssetFaceResponse
	for _, face := range faces {
		if face.Person != nil && face.Person.ID != "" {
			kept = append(kept, face)
		}
	}
	return kept
}

// linkedPersonIDs is the set of people already having a face on an asset.
func linkedPersonIDs(faces []model.AssetFaceResponse) map[string]bool {
	linked := map[string]bool{}
	for _, face := range faces {
		if face.Person != nil && face.Person.ID != "" {
			linked[face.Person.ID] = true
		}
	}
	return linked
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
