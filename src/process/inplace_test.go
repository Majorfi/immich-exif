package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

type inPlaceServer struct {
	served          string
	sidecars        string
	refreshed       bool
	gets            int
	ignoreRefreshes int
	refreshRequests int
}

func (s *inPlaceServer) handler(t *testing.T) http.HandlerFunc {
	desc := "external desc"
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/assets/asset-1/original":
			_, _ = w.Write([]byte(s.served))
		case r.URL.Path == "/api/asset-files":
			sidecars := s.sidecars
			if sidecars == "" {
				sidecars = "[]"
			}
			_, _ = w.Write([]byte(sidecars))
		case r.URL.Path == "/api/assets/jobs" && r.Method == http.MethodPost:
			s.refreshRequests++
			s.refreshed = s.refreshRequests > s.ignoreRefreshes
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/assets/asset-1":
			s.gets++
			modified := time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC)
			if s.refreshed {
				modified = modified.Add(time.Hour)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(model.AssetResponse{
				ID:               "asset-1",
				OriginalFileName: "ext1.jpg",
				OriginalPath:     "/mnt/external/sub/ext1.jpg",
				Checksum:         pathChecksum("/mnt/external/sub/ext1.jpg"),
				LibraryID:        "library-1",
				FileModifiedAt:   modified,
				ExifInfo:         &model.ExifInfo{Description: &desc},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}
}

func inPlaceLibrary(t *testing.T, content string) (string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(root, "sub", "ext1.jpg")
	if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte(content), 0640); err != nil {
		t.Fatal(err)
	}
	return root, local
}

func runInPlace(t *testing.T, state *inPlaceServer, root string, write func(string)) model.ProcessResult {
	t.Helper()
	withStubbedSleep(t)
	server := httptest.NewServer(state.handler(t))
	t.Cleanup(server.Close)
	restore := withMockExiftool(func(path string) (exif.ExifTagMap, error) {
		if strings.HasSuffix(path, ".xmp") {
			return exif.ExifTagMap{"Description": "stale desc", "XMP-dc:Description": "stale desc"}, nil
		}
		return exif.ExifTagMap{}, nil
	}, func(path string, _ []string) error {
		write(path)
		return nil
	})
	t.Cleanup(restore)
	cfg := &model.Config{TmpDir: t.TempDir(), InPlaceMaps: []model.PathMapping{{Server: "/mnt/external", Local: root}}}
	return ProcessAsset(api.NewImmichClient(server.URL, "key"), nil, cfg, "asset-1", 1, 1, &noopEmitter{}, nil)
}

func appendTags(path string) {
	file, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = file.WriteString("+tags")
	file.Close()
}

func TestInPlaceRewritesTheLibraryFileAndKeepsTheAsset(t *testing.T) {
	root, local := inPlaceLibrary(t, "original bytes")
	state := &inPlaceServer{served: "original bytes"}
	result := runInPlace(t, state, root, appendTags)

	if result.Status != model.StatusSuccess || result.Message != "updated in place" || result.NewID != "" {
		t.Fatalf("expected an in-place success under the same ID, got %+v", result)
	}
	data, _ := os.ReadFile(local)
	if string(data) != "original bytes+tags" {
		t.Fatalf("library file not rewritten: %q", data)
	}
	info, _ := os.Stat(local)
	if info.Mode().Perm() != 0640 {
		t.Fatalf("file mode not preserved: %v", info.Mode().Perm())
	}
	if !state.refreshed {
		t.Fatal("Immich must be asked to re-read the file")
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(local), ".*immich-exif*"))
	if len(leftovers) != 0 {
		t.Fatalf("staged files left behind: %v", leftovers)
	}
}

func TestInPlaceRefusesAFileThatIsNotTheAsset(t *testing.T) {
	root, local := inPlaceLibrary(t, "some other photo")
	result := runInPlace(t, &inPlaceServer{served: "original bytes"}, root, appendTags)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "differs from the file Immich serves") {
		t.Fatalf("expected a wrong-mapping skip, got %+v", result)
	}
	if data, _ := os.ReadFile(local); string(data) != "some other photo" {
		t.Fatalf("an unrelated file must stay untouched: %q", data)
	}
}

func TestInPlaceSkipsADisagreeingSidecar(t *testing.T) {
	root, local := inPlaceLibrary(t, "original bytes")
	if err := os.WriteFile(local+".xmp", []byte("xmp"), 0644); err != nil {
		t.Fatal(err)
	}
	result := runInPlace(t, &inPlaceServer{served: "original bytes"}, root, appendTags)
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "disagrees with Immich") {
		t.Fatalf("expected a sidecar skip, got %+v", result)
	}
	if data, _ := os.ReadFile(local); string(data) != "original bytes" {
		t.Fatalf("file must stay untouched: %q", data)
	}
}

func TestInPlaceSkipsAFileChangedWhileProcessing(t *testing.T) {
	root, local := inPlaceLibrary(t, "original bytes")
	state := &inPlaceServer{served: "original bytes"}
	result := runInPlace(t, state, root, func(path string) {
		appendTags(path)
		_ = os.WriteFile(local, []byte("edited elsewhere"), 0640)
	})
	if result.Status != model.StatusSkipped || !strings.Contains(result.Message, "changed on disk") {
		t.Fatalf("expected a changed-on-disk skip, got %+v", result)
	}
	if data, _ := os.ReadFile(local); string(data) != "edited elsewhere" {
		t.Fatalf("the concurrent edit must win: %q", data)
	}
	if state.refreshed {
		t.Fatal("nothing was committed, so nothing must be refreshed")
	}
}

func TestCheckLocalFileRefusesSymlinksAndHardLinks(t *testing.T) {
	root, local := inPlaceLibrary(t, "x")
	if note := checkLocalFile(root, local); note != "" {
		t.Fatalf("a plain file must pass: %s", note)
	}
	link := filepath.Join(root, "link.jpg")
	if err := os.Symlink(local, link); err != nil {
		t.Fatal(err)
	}
	if note := checkLocalFile(root, link); !strings.Contains(note, "symlink") {
		t.Fatalf("expected a symlink refusal, got %q", note)
	}
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if note := checkLocalFile(root, filepath.Join(root, "dirlink", "ext1.jpg")); !strings.Contains(note, "symlink") {
		t.Fatalf("expected a refusal for a symlinked directory, got %q", note)
	}
	hard := filepath.Join(root, "hard.jpg")
	if err := os.Link(local, hard); err != nil {
		t.Fatal(err)
	}
	if note := checkLocalFile(root, local); !strings.Contains(note, "hard links") {
		t.Fatalf("expected a hard-link refusal, got %q", note)
	}
	if note := checkLocalFile(root, filepath.Join(root, "..", "escape.jpg")); !strings.Contains(note, "outside") {
		t.Fatalf("expected an outside-root refusal, got %q", note)
	}
}

func TestMapServerPathPicksTheLongestPrefixOnComponentBoundaries(t *testing.T) {
	cfg := &model.Config{InPlaceMaps: []model.PathMapping{{Server: "/mnt", Local: "/a"}, {Server: "/mnt/photos", Local: "/b"}}}
	if local, _, ok := cfg.MapServerPath("/mnt/photos/x/y.jpg"); !ok || local != "/b/x/y.jpg" {
		t.Fatalf("longest prefix not used: %q %v", local, ok)
	}
	if local, _, ok := cfg.MapServerPath("/mnt/photosextra/y.jpg"); !ok || local != "/a/photosextra/y.jpg" {
		t.Fatalf("prefix must stop at a path component: %q %v", local, ok)
	}
	if _, _, ok := cfg.MapServerPath("/data/y.jpg"); ok {
		t.Fatal("an unmapped path must not resolve")
	}
}

func TestInPlaceAsksAgainWhenImmichReadAStaleView(t *testing.T) {
	root, _ := inPlaceLibrary(t, "original bytes")
	state := &inPlaceServer{served: "original bytes", ignoreRefreshes: 1}
	result := runInPlace(t, state, root, appendTags)
	if result.Status != model.StatusSuccess {
		t.Fatalf("a refresh that read a stale view must be requested again, got %+v", result)
	}
	if state.refreshRequests < 2 {
		t.Fatalf("expected the refresh to be requested again, got %d requests", state.refreshRequests)
	}
}

func TestSidecarDisagreementCatchesValuesImmichCleared(t *testing.T) {
	cfg := &model.Config{}
	asset := model.AssetResponse{ExifInfo: &model.ExifInfo{}}
	for name, tags := range map[string]exif.ExifTagMap{
		"description": {"Description": "old caption"},
		"gps":         {"GPSLatitude": -33.8},
		"rating":      {"Rating": float64(3)},
		"create date": {"CreateDate": "2019:01:02 03:04:05"},
	} {
		if note := sidecarDisagreement(cfg, asset, tags); note == "" {
			t.Fatalf("%s: a sidecar value Immich no longer has must block", name)
		}
	}
	if note := sidecarDisagreement(cfg, asset, exif.ExifTagMap{"Rating": float64(0)}); note != "" {
		t.Fatalf("Immich writes rating 0 for unrated assets, which must pass: %s", note)
	}
}

func TestInPlaceReportsSuccessWhenImmichIsSlowToReread(t *testing.T) {
	root, local := inPlaceLibrary(t, "original bytes")
	orig := refreshWaitAttempts
	refreshWaitAttempts = 3
	t.Cleanup(func() { refreshWaitAttempts = orig })
	state := &inPlaceServer{served: "original bytes", ignoreRefreshes: 100}
	result := runInPlace(t, state, root, appendTags)
	if result.Status != model.StatusSuccess || !strings.Contains(result.Message, "next library scan") {
		t.Fatalf("a written file must not be reported as failed, got %+v", result)
	}
	if data, _ := os.ReadFile(local); string(data) != "original bytes+tags" {
		t.Fatalf("file not rewritten: %q", data)
	}
}

func TestSidecarDisagreementFollowsImmichDatePrecedence(t *testing.T) {
	cfg := &model.Config{}
	date, zone := "2021-05-06T09:08:09+00:00", "UTC+10"
	asset := model.AssetResponse{ExifInfo: &model.ExifInfo{DateTimeOriginal: &date, TimeZone: &zone}}
	for name, tags := range map[string]exif.ExifTagMap{
		"immich-written": {"DateTimeOriginal": "2021:05:06 19:08:09.000+10:00", "XMP-exif:DateTimeOriginal": "2021:05:06 19:08:09.000+10:00"},
		"wall clock":     {"DateTimeOriginal": "2021:05:06 19:08:09"},
	} {
		if note := sidecarDisagreement(cfg, asset, tags); note != "" {
			t.Fatalf("%s: a sidecar date matching Immich must pass, got %q", name, note)
		}
	}
	for name, tags := range map[string]exif.ExifTagMap{
		"stale create date": {"CreateDate": "2019:01:02 03:04:05"},
		"stale subsec date": {"SubSecDateTimeOriginal": "2019:01:02 03:04:05.120+00:00", "DateTimeOriginal": "2021:05:06 19:08:09.000+10:00"},
		"source image time": {"SourceImageCreateTime": "2019:01:02 03:04:05Z"},
	} {
		if note := sidecarDisagreement(cfg, asset, tags); note == "" {
			t.Fatalf("%s: the date Immich would pick from the sidecar differs and must block", name)
		}
	}
}

func TestSidecarDisagreementJudgesDatesByInstant(t *testing.T) {
	date, zone := "2021-05-06T09:08:09+00:00", "Pacific/Rarotonga"
	asset := model.AssetResponse{ExifInfo: &model.ExifInfo{DateTimeOriginal: &date, TimeZone: &zone}}
	tags := exif.ExifTagMap{"DateTimeOriginal": "2021:05:06 19:08:09.000+10:00", "XMP-exif:DateTimeOriginal": "2021:05:06 19:08:09.000+10:00"}
	if note := sidecarDisagreement(&model.Config{}, asset, tags); note != "" {
		t.Fatalf("a sidecar date at the same instant in another offset leaves Immich's date unchanged and must pass, got %q", note)
	}
}
