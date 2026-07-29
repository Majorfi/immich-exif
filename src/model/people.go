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
	// Videos carry MWG regions only in containers exiftool can write; the
	// rotation-anchoring guard that decides whether a given video is embeddable
	// runs later, once the file's Rotation tag is known.
	if IsVideoAsset(asset) && !SupportsVideoMetadataEmbedding(asset) {
		return false
	}
	for _, person := range asset.People {
		if _, ok := NamedVisibleName(person); ok {
			return true
		}
	}
	return false
}
