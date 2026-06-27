package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func TestSafePathComponent(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain filename", input: "photo.jpg", want: "photo.jpg"},
		{name: "name with spaces and parens", input: "my photo (1).jpg", want: "my photo (1).jpg"},
		{name: "leading dot kept", input: ".hidden.jpg", want: ".hidden.jpg"},
		{name: "parent traversal rejected", input: "../../../../etc/passwd", wantErr: true},
		{name: "subdir component rejected", input: "sub/photo.jpg", wantErr: true},
		{name: "trailing slash rejected", input: "photo.jpg/", wantErr: true},
		{name: "lone dot rejected", input: ".", wantErr: true},
		{name: "lone dotdot rejected", input: "..", wantErr: true},
		{name: "empty rejected", input: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safePathComponent("asset filename", tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestProcessAssetRejectsPathTraversalFilename(t *testing.T) {
	desc := "Test Description"
	downloadHit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/original") {
			downloadHit = true
			w.Write([]byte("fake-image-data"))
			return
		}
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "../../../../escape.jpg",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed for traversal filename, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "unsafe") {
		t.Fatalf("expected 'unsafe' in message, got: %s", result.Message)
	}
	if downloadHit {
		t.Fatal("download must not be attempted for an unsafe filename")
	}
}

func TestProcessAssetRejectsTraversalAlbumID(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	exportDir := t.TempDir()
	cfg := &model.Config{
		ExportDir: exportDir,
		ExportAlbumIDsByAsset: map[string][]string{
			"asset-1": {"../escape"},
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed for traversal album ID, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "unsafe") {
		t.Fatalf("expected 'unsafe' in message, got: %s", result.Message)
	}
}

func TestProcessAssetDryRunDoesNotWriteAlbumExport(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	exportDir := t.TempDir()
	cfg := &model.Config{
		DryRun:    true,
		ExportDir: exportDir,
		ExportAlbumIDsByAsset: map[string][]string{
			"asset-1": {"album-1"},
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "dry-run") {
		t.Fatalf("expected dry-run message, got: %s", result.Message)
	}

	entries, err := os.ReadDir(exportDir)
	if err != nil {
		t.Fatalf("read export dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected export dir to stay empty in dry-run with album mapping, found %d entries", len(entries))
	}
}

func TestProcessAssetDryRunDoesNotWriteToExportDir(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	exportDir := t.TempDir()
	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{DryRun: true, ExportDir: exportDir}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "dry-run") {
		t.Fatalf("expected dry-run message, got: %s", result.Message)
	}

	entries, err := os.ReadDir(exportDir)
	if err != nil {
		t.Fatalf("read export dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected export dir to stay empty in dry-run, found %d entries", len(entries))
	}
}
