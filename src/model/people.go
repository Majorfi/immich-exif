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
	Person        *PersonResponse `json:"person"`
}

// NamedPeopleNames returns the sorted names of the asset's visible, named
// people. Unnamed ML clusters and hidden people are excluded, mirroring what
// Immich itself is willing to import from file regions.
func NamedPeopleNames(asset AssetResponse) []string {
	var names []string
	for _, person := range asset.People {
		if person.IsHidden {
			continue
		}
		name := strings.TrimSpace(person.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HasFaceRegionsToEmbed reports whether a -faces run has anything to write
// for this asset: regions only apply to image files, and only named people
// are written.
func HasFaceRegionsToEmbed(asset AssetResponse) bool {
	return !IsVideoAsset(asset) && len(NamedPeopleNames(asset)) > 0
}
