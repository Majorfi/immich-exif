package exif

import (
	"math"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

// An empty desired set must never rewrite, and so never clear, an existing
// RegionInfo: "cleared" and "recognition never ran" are indistinguishable, so
// the file's own regions are left alone.
func TestCompareFaceRegionsNeverClearsExisting(t *testing.T) {
	existing := existingRegionInfo(regionEntry("Alice", 0.2, 0.3, 0.2, 0.4))
	if tc := CompareFaceRegions(nil, 4000, 2000, existing); tc != nil {
		t.Fatalf("empty desired must leave existing regions untouched, got %+v", tc)
	}
}

// Ground-truth raster boxes computed by hand from the inverse of Immich's
// orientRegionInfo, so a rotated-file mapping cannot pass on an error it shares
// with the test's forward reimplementation (immichDisplayedRegion).
func TestBuildFaceRegionsOrientationGroundTruth(t *testing.T) {
	// Displayed box for this face is X=0.2, Y=0.3, W=0.2, H=0.4.
	face := namedFace("Alice", 100, 50, 300, 250, 1000, 500)
	cases := []struct {
		orientation int
		x, y, w, h  float64
	}{
		{1, 0.2, 0.3, 0.2, 0.4},
		{6, 0.3, 0.8, 0.4, 0.2},
		{8, 0.7, 0.2, 0.4, 0.2},
	}
	for _, tc := range cases {
		regions := BuildFaceRegions([]model.AssetFaceResponse{face}, tc.orientation)
		if len(regions) != 1 {
			t.Fatalf("orientation %d: expected 1 region, got %d", tc.orientation, len(regions))
		}
		got := regions[0]
		if math.Abs(got.X-tc.x) > 1e-9 || math.Abs(got.Y-tc.y) > 1e-9 ||
			math.Abs(got.W-tc.w) > 1e-9 || math.Abs(got.H-tc.h) > 1e-9 {
			t.Fatalf("orientation %d: got (%v,%v,%v,%v), want (%v,%v,%v,%v)",
				tc.orientation, got.X, got.Y, got.W, got.H, tc.x, tc.y, tc.w, tc.h)
		}
	}
}

// A file's RegionInfo can be malformed; parseFaceRegions must skip unusable
// entries rather than panic. A dropped entry lowers the parsed count, which is
// why the wholesale rewrite compares counts.
func TestParseFaceRegionsHandlesMalformed(t *testing.T) {
	if got := parseFaceRegions("not-a-struct"); got != nil {
		t.Fatalf("non-map RegionInfo must parse to nil, got %+v", got)
	}
	if got := parseFaceRegions(map[string]any{"RegionList": "not-a-list"}); got != nil {
		t.Fatalf("non-list RegionList must parse to nil, got %+v", got)
	}
	info := map[string]any{"RegionList": []any{
		map[string]any{"Name": "NoArea", "Type": "Face"},
		"not-a-map",
		regionEntry("Alice", 0.2, 0.3, 0.2, 0.4),
	}}
	got := parseFaceRegions(info)
	if len(got) != 1 || got[0].Name != "Alice" {
		t.Fatalf("malformed entries must be skipped, keeping only Alice, got %+v", got)
	}
}
