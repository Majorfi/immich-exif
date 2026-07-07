package exif

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/majorfi/immich-exif/model"
)

// FaceRegion is one named face rectangle in MWG form: X/Y is the rectangle
// center, all four values are normalized against the stored (unrotated)
// image dimensions.
type FaceRegion struct {
	Name string
	X    float64
	Y    float64
	W    float64
	H    float64
}

// regionCoordTolerance absorbs the %.5f serialization rounding; the effective
// per-axis tolerance is widened to two raster pixels (see coordTolerance)
// because Immich's importer floors region corners to whole pixels, which on
// small images shifts a coordinate by more than this constant.
const regionCoordTolerance = 0.001

// BuildFaceRegions converts Immich face boxes (pixels on the
// orientation-corrected image the ML pipeline analyzed) into regions in the
// stored image's coordinate space. orientation is the file's EXIF Orientation
// value (1-8; anything else means unrotated). Faces without a visible, named
// person are dropped, mirroring what Immich itself imports.
//
// A face with sourceType "exif" is an echo of the file's own regions (the
// server's metadata import). Counting echoes next to the same person's
// detected faces makes the file trail the server forever: each replace
// re-imports the written region and re-detects the face, growing the set by
// one per run. Echoes are therefore dropped when the person also has a
// non-exif face, and kept only as the person's sole source (server without
// ML, regions imported from files).
func BuildFaceRegions(faces []model.AssetFaceResponse, orientation int) []FaceRegion {
	detectedPersonIDs := map[string]bool{}
	for _, face := range faces {
		if face.Person != nil && face.SourceType != "exif" {
			detectedPersonIDs[face.Person.ID] = true
		}
	}

	var regions []FaceRegion
	for _, face := range faces {
		if face.Person == nil || face.Person.IsHidden {
			continue
		}
		if face.SourceType == "exif" && detectedPersonIDs[face.Person.ID] {
			continue
		}
		name := strings.TrimSpace(face.Person.Name)
		if name == "" || face.ImageWidth <= 0 || face.ImageHeight <= 0 {
			continue
		}
		w := float64(face.BoundingBoxX2-face.BoundingBoxX1) / float64(face.ImageWidth)
		h := float64(face.BoundingBoxY2-face.BoundingBoxY1) / float64(face.ImageHeight)
		if w <= 0 || h <= 0 {
			continue
		}
		x := float64(face.BoundingBoxX1+face.BoundingBoxX2) / 2 / float64(face.ImageWidth)
		y := float64(face.BoundingBoxY1+face.BoundingBoxY2) / 2 / float64(face.ImageHeight)
		x, y, w, h = rasterRegion(x, y, w, h, orientation)
		regions = append(regions, FaceRegion{Name: name, X: clamp01(x), Y: clamp01(y), W: clamp01(w), H: clamp01(h)})
	}
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Name != regions[j].Name {
			return regions[i].Name < regions[j].Name
		}
		return regions[i].X < regions[j].X
	})
	return regions
}

// rasterRegion maps a normalized region from the displayed
// (orientation-applied) space back into the stored image's space. It is the
// exact inverse of the forward transform Immich applies when importing file
// regions, so a written region re-imports to the same box.
func rasterRegion(x, y, w, h float64, orientation int) (float64, float64, float64, float64) {
	switch orientation {
	case 2:
		x = 1 - x
	case 3:
		x, y = 1-x, 1-y
	case 4:
		y = 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, 1-x
	case 7:
		x, y = 1-y, 1-x
	case 8:
		x, y = 1-y, x
	}
	if orientation >= 5 && orientation <= 8 {
		w, h = h, w
	}
	return x, y, w, h
}

func clamp01(v float64) float64 {
	return math.Min(1, math.Max(0, v))
}

// CompareFaceRegions builds the change that replaces the file's XMP-mwg-rs
// RegionInfo with the Immich face regions. Immich is the source of truth, so
// a disagreeing structure is rewritten wholesale; when Immich has no named
// faces nothing is written and existing file regions are left alone (there is
// no way to tell "cleared" from "recognition never ran").
func CompareFaceRegions(regions []FaceRegion, rasterWidth, rasterHeight int, existing ExifTagMap) *TagChange {
	if len(regions) == 0 || rasterWidth <= 0 || rasterHeight <= 0 {
		return nil
	}
	current := parseFaceRegions(existing["XMP-mwg-rs:RegionInfo"])
	if FaceRegionsMatch(current, regions, rasterWidth, rasterHeight) {
		return nil
	}

	tc := &TagChange{Args: []string{regionInfoArg(regions, rasterWidth, rasterHeight)}}
	entry := model.DiffEntry{Tag: "Face regions", Symbol: model.DiffAdd, Old: "(none)", New: regionNames(regions)}
	if len(current) > 0 {
		entry.Symbol = model.DiffChange
		entry.Old = regionNames(current)
	}
	// A geometry-only rewrite keeps the name lists identical; without a marker
	// the confirmation diff would read "Alice -> Alice".
	if entry.Old == entry.New {
		entry.New += " (face boxes moved)"
	}
	tc.Diffs = append(tc.Diffs, entry)
	return tc
}

// FaceRegionsMatch reports whether two sorted region sets agree within the
// round-trip noise: %.5f serialization rounding plus the up-to-two-pixel
// quantization Immich's importer introduces by flooring region corners.
func FaceRegionsMatch(current, desired []FaceRegion, rasterWidth, rasterHeight int) bool {
	if len(current) != len(desired) {
		return false
	}
	tolX := coordTolerance(rasterWidth)
	tolY := coordTolerance(rasterHeight)
	for i := range desired {
		if current[i].Name != desired[i].Name {
			return false
		}
		if math.Abs(current[i].X-desired[i].X) > tolX ||
			math.Abs(current[i].Y-desired[i].Y) > tolY ||
			math.Abs(current[i].W-desired[i].W) > tolX ||
			math.Abs(current[i].H-desired[i].H) > tolY {
			return false
		}
	}
	return true
}

func coordTolerance(rasterDim int) float64 {
	if rasterDim <= 0 {
		return regionCoordTolerance
	}
	return math.Max(regionCoordTolerance, 2/float64(rasterDim))
}

// parseFaceRegions reads the RegionInfo structure exiftool returns for a
// -struct read. Every region counts, whatever its Type: the write replaces
// the whole structure, so any region the file has and Immich does not is a
// difference.
func parseFaceRegions(value any) []FaceRegion {
	info, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	items, ok := info["RegionList"].([]any)
	if !ok {
		return nil
	}

	var regions []FaceRegion
	for _, item := range items {
		region, ok := item.(map[string]any)
		if !ok {
			continue
		}
		area, ok := region["Area"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := region["Name"].(string)
		regions = append(regions, FaceRegion{
			Name: strings.TrimSpace(name),
			X:    floatTag(area["X"]),
			Y:    floatTag(area["Y"]),
			W:    floatTag(area["W"]),
			H:    floatTag(area["H"]),
		})
	}
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Name != regions[j].Name {
			return regions[i].Name < regions[j].Name
		}
		return regions[i].X < regions[j].X
	})
	return regions
}

// floatTag converts an exiftool JSON value to a float. exiftool serializes
// floats with many decimals as strings, so both forms must parse.
func floatTag(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func regionInfoArg(regions []FaceRegion, rasterWidth, rasterHeight int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-XMP-mwg-rs:RegionInfo={AppliedToDimensions={W=%d,H=%d,Unit=pixel},RegionList=[", rasterWidth, rasterHeight)
	for i, region := range regions {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "{Area={X=%.5f,Y=%.5f,W=%.5f,H=%.5f,Unit=normalized},Name=%s,Type=Face}",
			region.X, region.Y, region.W, region.H, escapeStructValue(region.Name))
	}
	b.WriteString("]}")
	return b.String()
}

// escapeStructValue escapes the characters exiftool treats as structure
// syntax so person names survive serialization verbatim.
func escapeStructValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ',', '[', ']', '{', '}', '|', '=':
			b.WriteByte('|')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func regionNames(regions []FaceRegion) string {
	names := make([]string, 0, len(regions))
	for _, region := range regions {
		name := region.Name
		if name == "" {
			name = "(unnamed)"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}
