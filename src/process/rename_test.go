package process

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func TestRenderCaptureNameAllVerbs(t *testing.T) {
	tm := time.Date(2004, 6, 5, 14, 25, 32, 0, time.UTC)
	cases := map[string]string{
		"%Y%m%d-%H%M%S": "20040605-142532",
		"%y":            "04",
		"IMG_%Y":        "IMG_2004",
		"100%% done":    "100% done",
	}
	for pattern, want := range cases {
		got, err := renderCaptureName(pattern, tm)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", pattern, err)
		}
		if got != want {
			t.Fatalf("%q: got %q, want %q", pattern, got, want)
		}
	}
}

func TestRenderCaptureNameUnknownVerb(t *testing.T) {
	if _, err := renderCaptureName("%Q", time.Now()); err == nil {
		t.Fatal("expected error for unknown verb %Q")
	}
}

func TestRenderCaptureNameDanglingPercent(t *testing.T) {
	if _, err := renderCaptureName("prefix-%", time.Now()); err == nil {
		t.Fatal("expected error for dangling %")
	}
}

func TestValidateRenamePattern(t *testing.T) {
	if err := ValidateRenamePattern("%Y%m%d-%H%M%S"); err != nil {
		t.Fatalf("expected valid pattern, got %v", err)
	}
	if err := ValidateRenamePattern("%Q"); err == nil {
		t.Fatal("expected error for unknown verb")
	}
	if err := ValidateRenamePattern("%Y/%m"); err == nil {
		t.Fatal("expected error for a pattern containing a path separator")
	}
	if err := ValidateRenamePattern(""); err == nil {
		t.Fatal("expected error for an empty pattern")
	}
	if err := ValidateRenamePattern("   "); err == nil {
		t.Fatal("expected error for a whitespace-only pattern")
	}
}

func exportTestAsset(date, tz string) *model.AssetResponse {
	info := &model.ExifInfo{}
	if date != "" {
		info.DateTimeOriginal = &date
	}
	if tz != "" {
		info.TimeZone = &tz
	}
	return &model.AssetResponse{ExifInfo: info}
}

func TestExportFileNameRenamesWithExtension(t *testing.T) {
	cfg := &model.Config{Rename: true, RenamePattern: "%Y%m%d-%H%M%S"}
	asset := exportTestAsset("2004-06-05T12:25:32.000Z", "UTC+02:00")
	got, err := exportFileName(cfg, asset, "IMG_45698.JPG")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "20040605-142532.JPG" {
		t.Fatalf("got %q, want 20040605-142532.JPG", got)
	}
}

func TestExportFileNameDisabledKeepsOriginal(t *testing.T) {
	cfg := &model.Config{Rename: false, RenamePattern: "%Y%m%d-%H%M%S"}
	got, err := exportFileName(cfg, exportTestAsset("2004-06-05T12:25:32.000Z", ""), "IMG_1.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "IMG_1.jpg" {
		t.Fatalf("got %q, want IMG_1.jpg", got)
	}
}

func TestExportFileNameNoDateFallsBackToOriginal(t *testing.T) {
	cfg := &model.Config{Rename: true, RenamePattern: "%Y%m%d-%H%M%S"}
	got, err := exportFileName(cfg, exportTestAsset("", ""), "scan_001.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "scan_001.png" {
		t.Fatalf("expected fallback to original name, got %q", got)
	}
}

func TestCopyFileToDirSuffixesOnCollision(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}

	first, err := copyFileToDir(src, out, "2004.jpg", true)
	if err != nil {
		t.Fatalf("first copy: %v", err)
	}
	if filepath.Base(first) != "2004.jpg" {
		t.Fatalf("first name got %q, want 2004.jpg", filepath.Base(first))
	}

	second, err := copyFileToDir(src, out, "2004.jpg", true)
	if err != nil {
		t.Fatalf("second copy: %v", err)
	}
	if filepath.Base(second) != "2004-001.jpg" {
		t.Fatalf("second name got %q, want 2004-001.jpg", filepath.Base(second))
	}
}

func TestCopyFileToDirNoSuffixFailsOnCollision(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := copyFileToDir(src, dir, "src.txt", false); err == nil {
		t.Fatal("expected collision error when allowSuffix is false")
	}
}
