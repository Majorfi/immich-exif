package model

import (
	"sort"
	"strings"
)

type PersonResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsHidden bool   `json:"isHidden"`
}

// AssetFaceResponse is one detected face box from GET /api/faces. The
// coordinates are pixels on the orientation-corrected image the ML pipeline
// analyzed (usually the preview), whose size is ImageWidth x ImageHeight.
type AssetFaceResponse struct {
	BoundingBoxX1 int             `json:"boundingBoxX1"`
	BoundingBoxY1 int             `json:"boundingBoxY1"`
	BoundingBoxX2 int             `json:"boundingBoxX2"`
	BoundingBoxY2 int             `json:"boundingBoxY2"`
	ImageWidth    int             `json:"imageWidth"`
	ImageHeight   int             `json:"imageHeight"`
	SourceType    string          `json:"sourceType"`
	Person        *PersonResponse `json:"person"`
}

// CreateFaceRequest is the POST /faces payload that links a person to an asset
// at a pixel box. The coordinates and image dimensions mirror what
// GetAssetFaces returns, so a box read from one asset re-creates verbatim on
// another. Immich records it with sourceType "manual", which survives later ML
// detection jobs (Immich 1.127+).
type CreateFaceRequest struct {
	AssetID     string `json:"assetId"`
	PersonID    string `json:"personId"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ImageWidth  int    `json:"imageWidth"`
	ImageHeight int    `json:"imageHeight"`
}

// NamedVisibleName returns a person's write-eligible region name and whether
// they have one: visible and named, mirroring what Immich itself is willing to
// import from file regions. It is the single definition of that eligibility,
// shared by the search pre-filter and the region writer.
func NamedVisibleName(person PersonResponse) (string, bool) {
	if person.IsHidden {
		return "", false
	}
	name := strings.TrimSpace(person.Name)
	return name, name != ""
}

// NamedPeopleNames returns the sorted names of the asset's visible, named
// people. Unnamed ML clusters and hidden people are excluded.
func NamedPeopleNames(asset AssetResponse) []string {
	var names []string
	for _, person := range asset.People {
		if name, ok := NamedVisibleName(person); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// HasFaceRegionsToEmbed reports whether a -faces run has anything to write
// for this asset: regions only apply to image files, and only named people
// are written. It answers the presence question without allocating and sorting
// the name list, for the per-asset -all pre-filter.
func HasFaceRegionsToEmbed(asset AssetResponse) bool {
	if IsVideoAsset(asset) {
		return false
	}
	for _, person := range asset.People {
		if _, ok := NamedVisibleName(person); ok {
			return true
		}
	}
	return false
}
