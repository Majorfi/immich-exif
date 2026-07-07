package exif

import (
	"math"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func namedFace(name string, x1, y1, x2, y2, width, height int) model.AssetFaceResponse {
	return model.AssetFaceResponse{
		BoundingBoxX1: x1,
		BoundingBoxY1: y1,
		BoundingBoxX2: x2,
		BoundingBoxY2: y2,
		ImageWidth:    width,
		ImageHeight:   height,
		Person:        &model.PersonResponse{ID: "p1", Name: name},
	}
}

func TestBuildFaceRegionsNormalizesAndSorts(t *testing.T) {
	faces := []model.AssetFaceResponse{
		namedFace("Zoe", 800, 100, 900, 200, 1000, 500),
		namedFace("Alice", 100, 50, 300, 250, 1000, 500),
	}
	regions := BuildFaceRegions(faces, 1)
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].Name != "Alice" || regions[1].Name != "Zoe" {
		t.Fatalf("expected name-sorted regions, got %v", regions)
	}
	alice := regions[0]
	if math.Abs(alice.X-0.2) > 1e-9 || math.Abs(alice.Y-0.3) > 1e-9 ||
		math.Abs(alice.W-0.2) > 1e-9 || math.Abs(alice.H-0.4) > 1e-9 {
		t.Fatalf("unexpected normalized region: %+v", alice)
	}
}

func TestBuildFaceRegionsDropsUnusableFaces(t *testing.T) {
	hidden := namedFace("Ghost", 0, 0, 10, 10, 100, 100)
	hidden.Person.IsHidden = true
	unnamed := namedFace("  ", 0, 0, 10, 10, 100, 100)
	noPerson := namedFace("x", 0, 0, 10, 10, 100, 100)
	noPerson.Person = nil
	noDims := namedFace("NoDims", 0, 0, 10, 10, 0, 0)
	inverted := namedFace("Inverted", 50, 50, 10, 10, 100, 100)

	regions := BuildFaceRegions([]model.AssetFaceResponse{hidden, unnamed, noPerson, noDims, inverted}, 1)
	if len(regions) != 0 {
		t.Fatalf("expected no regions, got %v", regions)
	}
}

// immichDisplayedRegion is the forward transform Immich's metadata importer
// applies to file regions (orientRegionInfo in metadata.service.ts). The
// regions this tool writes must invert it exactly, so a written region
// re-imports to the same displayed box.
func immichDisplayedRegion(x, y, w, h float64, orientation int) (float64, float64, float64, float64) {
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
		x, y = 1-y, x
	case 7:
		x, y = 1-y, 1-x
	case 8:
		x, y = y, 1-x
	}
	if orientation >= 5 && orientation <= 8 {
		w, h = h, w
	}
	return x, y, w, h
}

func TestBuildFaceRegionsInvertsImmichOrientation(t *testing.T) {
	faces := []model.AssetFaceResponse{namedFace("Alice", 100, 50, 300, 250, 1000, 500)}
	wantX, wantY, wantW, wantH := 0.2, 0.3, 0.2, 0.4

	for orientation := 1; orientation <= 8; orientation++ {
		regions := BuildFaceRegions(faces, orientation)
		if len(regions) != 1 {
			t.Fatalf("orientation %d: expected 1 region, got %d", orientation, len(regions))
		}
		region := regions[0]
		gotX, gotY, gotW, gotH := immichDisplayedRegion(region.X, region.Y, region.W, region.H, orientation)
		if math.Abs(gotX-wantX) > 1e-9 || math.Abs(gotY-wantY) > 1e-9 ||
			math.Abs(gotW-wantW) > 1e-9 || math.Abs(gotH-wantH) > 1e-9 {
			t.Fatalf("orientation %d: Immich would re-import (%f,%f,%f,%f), want (%f,%f,%f,%f)",
				orientation, gotX, gotY, gotW, gotH, wantX, wantY, wantW, wantH)
		}
	}
}

func existingRegionInfo(regions ...map[string]any) ExifTagMap {
	items := make([]any, 0, len(regions))
	for _, region := range regions {
		items = append(items, any(region))
	}
	return ExifTagMap{
		"XMP-mwg-rs:RegionInfo": map[string]any{
			"AppliedToDimensions": map[string]any{"W": float64(4000), "H": float64(2000), "Unit": "pixel"},
			"RegionList":          items,
		},
	}
}

func regionEntry(name string, x, y, w, h any) map[string]any {
	return map[string]any{
		"Name": name,
		"Type": "Face",
		"Area": map[string]any{"X": x, "Y": y, "W": w, "H": h, "Unit": "normalized"},
	}
}

func TestCompareFaceRegionsNothingToWrite(t *testing.T) {
	if tc := CompareFaceRegions(nil, 4000, 2000, ExifTagMap{}); tc != nil {
		t.Fatalf("no regions must produce no change, got %+v", tc)
	}
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	if tc := CompareFaceRegions(regions, 0, 2000, ExifTagMap{}); tc != nil {
		t.Fatalf("missing raster dims must produce no change, got %+v", tc)
	}
}

func TestCompareFaceRegionsSkipsWhenMatching(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	existing := existingRegionInfo(regionEntry("Alice", 0.20003, 0.29998, 0.2, 0.4))
	if tc := CompareFaceRegions(regions, 4000, 2000, existing); tc != nil {
		t.Fatalf("expected match within tolerance, got %+v", tc)
	}
}

func TestCompareFaceRegionsParsesStringFloats(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	existing := existingRegionInfo(regionEntry("Alice", "0.2000000000000000111", "0.3", "0.2", "0.4"))
	if tc := CompareFaceRegions(regions, 4000, 2000, existing); tc != nil {
		t.Fatalf("string-serialized floats must still match, got %+v", tc)
	}
}

func TestCompareFaceRegionsRewritesOnMismatch(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	existing := existingRegionInfo(regionEntry("Bob", 0.2, 0.3, 0.2, 0.4))

	tc := CompareFaceRegions(regions, 4000, 2000, existing)
	if tc == nil {
		t.Fatal("expected a change")
	}
	want := "-XMP-mwg-rs:RegionInfo={AppliedToDimensions={W=4000,H=2000,Unit=pixel},RegionList=[{Area={X=0.20000,Y=0.30000,W=0.20000,H=0.40000,Unit=normalized},Name=Alice,Type=Face}]}"
	if len(tc.Args) != 1 || tc.Args[0] != want {
		t.Fatalf("unexpected args:\n got %v\nwant %s", tc.Args, want)
	}
	if len(tc.Diffs) != 1 || tc.Diffs[0].Symbol != model.DiffChange || tc.Diffs[0].Old != "Bob" || tc.Diffs[0].New != "Alice" {
		t.Fatalf("unexpected diff: %+v", tc.Diffs)
	}
}

func TestCompareFaceRegionsAddsWhenFileHasNone(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	tc := CompareFaceRegions(regions, 4000, 2000, ExifTagMap{})
	if tc == nil {
		t.Fatal("expected a change")
	}
	if len(tc.Diffs) != 1 || tc.Diffs[0].Symbol != model.DiffAdd || tc.Diffs[0].Old != "(none)" {
		t.Fatalf("unexpected diff: %+v", tc.Diffs)
	}
}

func TestCompareFaceRegionsCountMismatchRewrites(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	existing := existingRegionInfo(
		regionEntry("Alice", 0.2, 0.3, 0.2, 0.4),
		regionEntry("Bob", 0.7, 0.3, 0.1, 0.2),
	)
	tc := CompareFaceRegions(regions, 4000, 2000, existing)
	if tc == nil {
		t.Fatal("a file region absent from Immich must trigger a rewrite")
	}
	if tc.Diffs[0].Old != "Alice, Bob" {
		t.Fatalf("unexpected old names: %q", tc.Diffs[0].Old)
	}
}

func TestBuildFaceRegionsDropsExifEchoesWhenDetected(t *testing.T) {
	// The echo lands on a DIFFERENT person record with the same name: the
	// importer resolves region names to people by name, so with split person
	// records the echo and the detection do not share an ID.
	echo := namedFace(" alice ", 102, 52, 302, 252, 1000, 500)
	echo.SourceType = "exif"
	echo.Person.ID = "p2"
	detected := namedFace("Alice", 100, 50, 300, 250, 1000, 500)
	detected.SourceType = "machine-learning"

	regions := BuildFaceRegions([]model.AssetFaceResponse{echo, detected}, 1)
	if len(regions) != 1 {
		t.Fatalf("the exif echo of a detected name must be dropped, got %d regions", len(regions))
	}
	if math.Abs(regions[0].X-0.2) > 1e-9 {
		t.Fatalf("the detected box must win over the echo, got X=%f", regions[0].X)
	}
}

func TestBuildFaceRegionsKeepsExifFacesOfUndetectedNames(t *testing.T) {
	imported := namedFace("Bob", 100, 50, 300, 250, 1000, 500)
	imported.SourceType = "exif"
	detected := namedFace("Alice", 600, 50, 800, 250, 1000, 500)
	detected.SourceType = "machine-learning"

	regions := BuildFaceRegions([]model.AssetFaceResponse{imported, detected}, 1)
	if len(regions) != 2 {
		t.Fatalf("an exif face whose name has no detection must be kept, got %d regions", len(regions))
	}
}

func TestBuildFaceRegionsKeepsExifOnlyFaces(t *testing.T) {
	imported := namedFace("Alice", 100, 50, 300, 250, 1000, 500)
	imported.SourceType = "exif"

	regions := BuildFaceRegions([]model.AssetFaceResponse{imported}, 1)
	if len(regions) != 1 {
		t.Fatalf("a person whose only face is exif-sourced must be kept, got %d regions", len(regions))
	}
}

func TestBuildFaceRegionsKeepsMultipleDetectedFaces(t *testing.T) {
	first := namedFace("Alice", 100, 50, 300, 250, 1000, 500)
	first.SourceType = "machine-learning"
	second := namedFace("Alice", 600, 50, 800, 250, 1000, 500)
	second.SourceType = "machine-learning"

	regions := BuildFaceRegions([]model.AssetFaceResponse{first, second}, 1)
	if len(regions) != 2 {
		t.Fatalf("two detected faces of the same person are both real, got %d regions", len(regions))
	}
}

func TestFaceRegionsMatchWidensToleranceOnSmallImages(t *testing.T) {
	desired := []FaceRegion{{Name: "Alice", X: 0.6166667, Y: 0.3, W: 0.2, H: 0.4}}
	current := []FaceRegion{{Name: "Alice", X: 0.61771, Y: 0.3, W: 0.2, H: 0.4}}

	if !FaceRegionsMatch(current, desired, 800, 600) {
		t.Fatal("a one-pixel floor() shift on a 800px-wide image must stay within tolerance")
	}
	if FaceRegionsMatch(current, desired, 8000, 6000) {
		t.Fatal("the same absolute shift on a large image is a real move and must mismatch")
	}
}

func TestCompareFaceRegionsMarksGeometryOnlyChanges(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.5, Y: 0.3, W: 0.2, H: 0.4}}
	existing := existingRegionInfo(regionEntry("Alice", 0.2, 0.3, 0.2, 0.4))

	tc := CompareFaceRegions(regions, 4000, 2000, existing)
	if tc == nil {
		t.Fatal("expected a change")
	}
	if tc.Diffs[0].Old == tc.Diffs[0].New {
		t.Fatalf("a geometry-only rewrite must not render an identical Old -> New, got %q -> %q", tc.Diffs[0].Old, tc.Diffs[0].New)
	}
	if !strings.Contains(tc.Diffs[0].New, "face boxes moved") {
		t.Fatalf("expected the geometry marker, got %q", tc.Diffs[0].New)
	}
}

func TestCompareFaceRegionsShowsUnnamedExistingRegions(t *testing.T) {
	regions := []FaceRegion{{Name: "Alice", X: 0.2, Y: 0.3, W: 0.2, H: 0.4}}
	existing := existingRegionInfo(regionEntry("", 0.7, 0.3, 0.1, 0.2))

	tc := CompareFaceRegions(regions, 4000, 2000, existing)
	if tc == nil {
		t.Fatal("expected a change")
	}
	if tc.Diffs[0].Old != "(unnamed)" {
		t.Fatalf("an unnamed existing region must be visible in the diff, got Old=%q", tc.Diffs[0].Old)
	}
}

func TestEscapeStructValue(t *testing.T) {
	got := escapeStructValue("Doe, J{r}=[x]|y")
	want := "Doe|, J|{r|}|=|[x|]||y"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if escapeStructValue("plain name") != "plain name" {
		t.Fatal("plain names must pass through unchanged")
	}
}

func TestRegionInfoArgEscapesNames(t *testing.T) {
	arg := regionInfoArg([]FaceRegion{{Name: "Doe, John", X: 0.5, Y: 0.5, W: 0.1, H: 0.1}}, 100, 50)
	if !strings.Contains(arg, "Name=Doe|, John,Type=Face") {
		t.Fatalf("expected escaped comma in name, got %s", arg)
	}
}
