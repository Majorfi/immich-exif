package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func TestProcessAssetExportMirrorsAcrossAlbumFolders(t *testing.T) {
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
			"asset-1": {"album-1", "album-2"},
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if result.Message != "exported to 2 album folders" {
		t.Fatalf("unexpected export message: %s", result.Message)
	}

	firstPath := filepath.Join(exportDir, "album-1", "photo.jpg")
	secondPath := filepath.Join(exportDir, "album-2", "photo.jpg")

	firstData, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first mirrored export: %v", err)
	}
	if string(firstData) != "fake-image-data" {
		t.Fatalf("unexpected first mirrored export content: %q", string(firstData))
	}

	secondData, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("read second mirrored export: %v", err)
	}
	if string(secondData) != "fake-image-data" {
		t.Fatalf("unexpected second mirrored export content: %q", string(secondData))
	}
}

func TestProcessAssetExportMirrorsIntoNoAlbumFolder(t *testing.T) {
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
			"asset-1": {"no-album"},
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if result.Message != "exported to 1 album folders" {
		t.Fatalf("unexpected export message: %s", result.Message)
	}

	exportPath := filepath.Join(exportDir, "no-album", "photo.jpg")
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read no-album export: %v", err)
	}
	if string(data) != "fake-image-data" {
		t.Fatalf("unexpected no-album export content: %q", string(data))
	}
}
