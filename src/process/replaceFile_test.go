//go:build !windows

package process

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReplaceFileKeepsModeAndExtendedAttributes(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "photo.jpg")
	source := filepath.Join(dir, "rewritten.jpg")
	if err := os.WriteFile(destination, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	name := "user.immich-exif-test"
	if err := unix.Setxattr(destination, name, []byte("kept"), 0); err != nil {
		t.Skipf("extended attributes unavailable here: %v", err)
	}
	original, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}

	if err := replaceFile(destination, source, original); err != nil {
		t.Fatalf("replace: %v", err)
	}
	data, _ := os.ReadFile(destination)
	info, _ := os.Stat(destination)
	if string(data) != "new" || info.Mode().Perm() != 0640 {
		t.Fatalf("got %q mode %v", data, info.Mode().Perm())
	}
	value := make([]byte, 16)
	size, err := unix.Getxattr(destination, name, value)
	if err != nil || string(value[:size]) != "kept" {
		t.Fatalf("extended attribute lost: %q %v", value[:size], err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".immich-exif-*")); len(leftovers) != 0 {
		t.Fatalf("staged file left behind: %v", leftovers)
	}
}
