package api

import (
	"crypto/sha1"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestReplaceAssetEditsSendsParametersVerbatim(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/assets/new-id/edits" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"assetId":"new-id","edits":[{"id":"e1","action":"crop","parameters":{"x":0,"y":0,"width":10,"height":20}}]}`))
	}))
	defer server.Close()

	edits := []model.AssetEdit{{Action: "crop", Parameters: []byte(`{"x":0,"y":0,"width":10,"height":20}`)}}
	applied, err := NewImmichClient(server.URL, "key").ReplaceAssetEdits("new-id", edits)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != `{"edits":[{"action":"crop","parameters":{"x":0,"y":0,"width":10,"height":20}}]}` {
		t.Fatalf("unexpected body: %s", body)
	}
	if !model.SameEdits(applied, edits) {
		t.Fatalf("response edits differ: %+v", applied)
	}
}

func sha1Sum(data string) []byte {
	sum := sha1.Sum([]byte(data))
	return sum[:]
}

func editedRenderServer(t *testing.T, disposition, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assets/asset-1/original" || r.URL.Query().Get("edited") != "true" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Disposition", disposition)
		_, _ = w.Write([]byte(body))
	}))
}

func TestDownloadEditedAssetNamesFileFromServer(t *testing.T) {
	server := editedRenderServer(t, `inline; filename*=UTF-8''IMG%201.jpeg`, "render")
	defer server.Close()

	dir := t.TempDir()
	path, err := NewImmichClient(server.URL, "key").DownloadEditedAsset("asset-1", dir, sha1Sum("original"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != filepath.Join(dir, "IMG 1.jpeg") {
		t.Fatalf("unexpected path: %s", path)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "render" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestDownloadEditedAssetRefusesOriginalFallback(t *testing.T) {
	server := editedRenderServer(t, `inline; filename*=UTF-8''IMG.jpg`, "original")
	defer server.Close()

	dir := t.TempDir()
	_, err := NewImmichClient(server.URL, "key").DownloadEditedAsset("asset-1", dir, sha1Sum("original"), nil)
	if !errors.Is(err, ErrEditedRenderMissing) {
		t.Fatalf("expected ErrEditedRenderMissing, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "IMG.jpg")); !os.IsNotExist(statErr) {
		t.Fatalf("fallback download must be removed, stat err: %v", statErr)
	}
}

func TestDownloadEditedAssetRejectsUnsafeName(t *testing.T) {
	server := editedRenderServer(t, `inline; filename*=UTF-8''..%2Fescape.jpg`, "render")
	defer server.Close()

	_, err := NewImmichClient(server.URL, "key").DownloadEditedAsset("asset-1", t.TempDir(), sha1Sum("original"), nil)
	if err == nil || !strings.Contains(err.Error(), "unsafe file name") {
		t.Fatalf("expected unsafe name error, got %v", err)
	}
}

func TestGetExifGeometryAppliesOrientation(t *testing.T) {
	type geometry struct {
		width, height int
		orientation   string
	}
	cases := map[string]geometry{
		`{"exifInfo":{"exifImageWidth":4000,"exifImageHeight":3000,"orientation":"1"}}`:  {4000, 3000, "1"},
		`{"exifInfo":{"exifImageWidth":4000,"exifImageHeight":3000,"orientation":"6"}}`:  {3000, 4000, "6"},
		`{"exifInfo":{"exifImageWidth":4000,"exifImageHeight":3000,"orientation":null}}`: {4000, 3000, ""},
		`{"exifInfo":{"exifImageWidth":null,"exifImageHeight":null,"orientation":"8"}}`:  {0, 0, "8"},
		`{"exifInfo":null}`: {0, 0, ""},
	}
	for payload, want := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(payload))
		}))
		width, height, orientation, err := NewImmichClient(server.URL, "key").GetExifGeometry("asset-1")
		server.Close()
		if err != nil || (geometry{width, height, orientation}) != want {
			t.Fatalf("%s: got %dx%d %q (err %v), want %+v", payload, width, height, orientation, err, want)
		}
	}
}
