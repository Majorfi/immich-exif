package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInPlaceMap(t *testing.T) {
	local, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mapping, err := parseInPlaceMap("/mnt/photos/=" + local)
	if err != nil || mapping.Server != "/mnt/photos" || mapping.Local != local {
		t.Fatalf("unexpected mapping %+v (err %v)", mapping, err)
	}
	for raw, want := range map[string]string{
		"/mnt/photos":                    "expected SERVER_PATH=LOCAL_DIR",
		"relative=" + local:              "must be absolute",
		"/mnt/photos=" + local + "/none": "no such file",
	} {
		if _, err := parseInPlaceMap(raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: expected %q, got %v", raw, want, err)
		}
	}
}

func TestParseConfigRejectsInPlaceWithExportOrBake(t *testing.T) {
	local := t.TempDir()
	for _, extra := range []string{"-export-dir=/tmp/out", "-apply-edits"} {
		restore := setupConfigTest([]string{"immich-exif", "-url=https://immich.example", "-api-key=k", "-in-place-map=/mnt=" + local, extra, "asset-1"})
		_, err := parseConfig()
		restore()
		if err == nil || !strings.Contains(err.Error(), "in-place-map") {
			t.Fatalf("%s: expected a conflict error, got %v", extra, err)
		}
	}
}
