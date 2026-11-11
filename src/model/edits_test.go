package model

import (
	"fmt"
	"testing"
)

func crop(width, height int) AssetEdit {
	return AssetEdit{Action: "crop", Parameters: []byte(fmt.Sprintf(`{"x":0,"y":0,"width":%d,"height":%d}`, width, height))}
}

func rotate(angle int) AssetEdit {
	return AssetEdit{Action: "rotate", Parameters: []byte(fmt.Sprintf(`{"angle":%d}`, angle))}
}

func TestEditedSize(t *testing.T) {
	mirror := AssetEdit{Action: "mirror", Parameters: []byte(`{"axis":"horizontal"}`)}
	cases := []struct {
		name          string
		edits         []AssetEdit
		width, height int
	}{
		{"no edits", nil, 4000, 3000},
		{"crop", []AssetEdit{crop(1200, 800)}, 1200, 800},
		{"quarter turn swaps", []AssetEdit{rotate(90)}, 3000, 4000},
		{"half turn keeps", []AssetEdit{rotate(180)}, 4000, 3000},
		{"crop then three-quarter turn", []AssetEdit{crop(1200, 800), rotate(270)}, 800, 1200},
		{"mirror keeps", []AssetEdit{mirror}, 4000, 3000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			width, height := EditedSize(tc.edits, 4000, 3000)
			if width != tc.width || height != tc.height {
				t.Fatalf("got %dx%d, want %dx%d", width, height, tc.width, tc.height)
			}
		})
	}
}

func TestSameEditsComparesJSONValues(t *testing.T) {
	reordered := AssetEdit{Action: "crop", Parameters: []byte(`{"height":20,"width":10,"y":0,"x":0}`)}
	if !SameEdits([]AssetEdit{crop(10, 20)}, []AssetEdit{reordered}) {
		t.Fatal("key order must not matter")
	}
	extra := AssetEdit{Action: "crop", Parameters: []byte(`{"x":0,"y":0,"width":10,"height":20,"feather":3}`)}
	if SameEdits([]AssetEdit{crop(10, 20)}, []AssetEdit{extra}) {
		t.Fatal("an unknown parameter must not be ignored")
	}
	if !SameEdits([]AssetEdit{crop(10, 20), rotate(90)}, []AssetEdit{crop(10, 20), rotate(90)}) {
		t.Fatal("equal edits must compare equal")
	}
	if SameEdits([]AssetEdit{rotate(90)}, []AssetEdit{rotate(270)}) {
		t.Fatal("different angles must differ")
	}
	if SameEdits([]AssetEdit{crop(10, 20), rotate(90)}, []AssetEdit{rotate(90), crop(10, 20)}) {
		t.Fatal("order matters")
	}
	if !SameEdits(nil, []AssetEdit{}) {
		t.Fatal("nil and empty are both no edits")
	}
}

func TestDescribeEdits(t *testing.T) {
	got := DescribeEdits([]AssetEdit{crop(1200, 800), rotate(90), {Action: "mirror", Parameters: []byte(`{"axis":"vertical"}`)}})
	if got != "crop 1200x800, rotate 90°, mirror vertical" {
		t.Fatalf("got %q", got)
	}
}
