package model

import "testing"

func TestMapServerPath(t *testing.T) {
	cfg := &Config{InPlaceMaps: []PathMapping{{Server: "/mnt", Local: "/a"}, {Server: "/mnt/photos", Local: "/b"}}}
	cases := map[string]struct {
		local, root string
		ok          bool
	}{
		"/mnt/photos/x/y.jpg":      {"/b/x/y.jpg", "/b", true},
		"/mnt/photos":              {"/b", "/b", true},
		"/mnt/photosextra/y.jpg":   {"/a/photosextra/y.jpg", "/a", true},
		"/mnt/photos/../other.jpg": {"/a/other.jpg", "/a", true},
		"/data/y.jpg":              {"", "", false},
	}
	for server, want := range cases {
		local, root, ok := cfg.MapServerPath(server)
		if local != want.local || root != want.root || ok != want.ok {
			t.Fatalf("%s: got %q %q %v, want %+v", server, local, root, ok, want)
		}
	}
	if _, _, ok := (&Config{}).MapServerPath("/mnt/a.jpg"); ok {
		t.Fatal("no mapping must resolve nothing")
	}
}

func TestBakesEditsAndWillReplace(t *testing.T) {
	edited := AssetResponse{IsEdited: true}
	if !(&Config{ApplyEdits: true}).BakesEdits(edited) || (&Config{}).BakesEdits(edited) || (&Config{ApplyEdits: true}).BakesEdits(AssetResponse{}) {
		t.Fatal("only an edited asset under -apply-edits is baked")
	}
	if !(&Config{}).WillReplace() || (&Config{DryRun: true}).WillReplace() || (&Config{ExportDir: "/out"}).WillReplace() {
		t.Fatal("only runs that are neither dry-run nor export replace assets")
	}
}
