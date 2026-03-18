package exif

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckExiftool(t *testing.T) {
	emptyPathDir := t.TempDir()
	t.Setenv("PATH", emptyPathDir)
	if err := CheckExiftool(); err == nil {
		t.Fatal("expected missing exiftool error")
	}

	fakeBinDir := t.TempDir()
	fakeToolPath := filepath.Join(fakeBinDir, "exiftool")
	fakeScript := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(fakeToolPath, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("write fake exiftool: %v", err)
	}
	t.Setenv("PATH", fakeBinDir)
	if err := CheckExiftool(); err != nil {
		t.Fatalf("expected exiftool to be found, got %v", err)
	}
}

func TestReadExifTagsWithArgs(t *testing.T) {
	fakeBinDir := setupReadExifFakeTool(t)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	values, err := readExifTagsWithArgs("ok.jpg", []string{"-json", "-n"})
	if err != nil {
		t.Fatalf("read exif tags success case: %v", err)
	}
	if values["Rating"] != float64(5) {
		t.Fatalf("expected rating 5, got %v", values["Rating"])
	}

	emptyValues, err := readExifTagsWithArgs("empty.jpg", []string{"-json", "-n"})
	if err != nil {
		t.Fatalf("read exif tags empty case: %v", err)
	}
	if len(emptyValues) != 0 {
		t.Fatalf("expected empty values map, got %v", emptyValues)
	}

	if _, err := readExifTagsWithArgs("bad.jpg", []string{"-json", "-n"}); err == nil {
		t.Fatal("expected parse error for bad json output")
	}
	if _, err := readExifTagsWithArgs("fail.jpg", []string{"-json", "-n"}); err == nil {
		t.Fatal("expected command error for failing exiftool")
	}
}

func TestReadExifTagsMergesSupplementalTags(t *testing.T) {
	fakeBinDir := setupMergeExifFakeTool(t)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	values, err := ReadExifTags("file.jpg")
	if err != nil {
		t.Fatalf("read exif tags merge case: %v", err)
	}

	if values["Rating"] != float64(4) {
		t.Fatalf("expected base rating, got %v", values["Rating"])
	}
	if values["XMP-dc:Description"] != "sample-description" {
		t.Fatalf("expected supplemental description, got %v", values["XMP-dc:Description"])
	}
}

func TestReadExifTagsIgnoresSupplementalErrors(t *testing.T) {
	fakeBinDir := setupSupplementalFailureExifFakeTool(t)
	t.Setenv("PATH", fakeBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	values, err := ReadExifTags("file.jpg")
	if err != nil {
		t.Fatalf("base exif read should still succeed when supplemental fails: %v", err)
	}
	if values["Rating"] != float64(3) {
		t.Fatalf("expected base rating, got %v", values["Rating"])
	}
	if _, exists := values["XMP-dc:Description"]; exists {
		t.Fatalf("did not expect supplemental tags after supplemental error, got %v", values)
	}
}

func setupReadExifFakeTool(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
set -eu
for lastArg; do true; done
baseName=$(basename "$lastArg")
case "$baseName" in
  ok.jpg)
    echo '[{"Rating":5}]'
    ;;
  empty.jpg)
    echo '[]'
    ;;
  bad.jpg)
    echo 'not-json'
    ;;
  fail.jpg)
    echo 'boom' >&2
    exit 1
    ;;
  *)
    echo '[{}]'
    ;;
esac
`
	return writeFakeExiftoolScript(t, script)
}

func setupMergeExifFakeTool(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
set -eu
args="$*"
if echo "$args" | grep -q -- "-a"; then
  echo '[{"XMP-dc:Description":"sample-description"}]'
  exit 0
fi
echo '[{"Rating":4}]'
`
	return writeFakeExiftoolScript(t, script)
}

func setupSupplementalFailureExifFakeTool(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
set -eu
args="$*"
if echo "$args" | grep -q -- "-a"; then
  echo 'supplemental failed' >&2
  exit 1
fi
echo '[{"Rating":3}]'
`
	return writeFakeExiftoolScript(t, script)
}

func writeFakeExiftoolScript(t *testing.T, scriptContent string) string {
	t.Helper()
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "exiftool")
	if err := os.WriteFile(scriptPath, []byte(strings.TrimSpace(scriptContent)+"\n"), 0o755); err != nil {
		t.Fatalf("write fake exiftool script: %v", err)
	}
	return binDir
}
