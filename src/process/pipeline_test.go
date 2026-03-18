package process

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func TestCopyFileFailsWhenDestinationExists(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "src.jpg")
	dstPath := filepath.Join(tempDir, "dst.jpg")

	if err := os.WriteFile(srcPath, []byte("source"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(dstPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("write destination: %v", err)
	}

	err := copyFile(srcPath, dstPath)
	if err == nil {
		t.Fatalf("expected error when destination exists")
	}
	if !strings.Contains(err.Error(), "destination exists") {
		t.Fatalf("expected destination exists error, got: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(got) != "existing" {
		t.Fatalf("destination content changed unexpectedly: %q", string(got))
	}
}

func TestCopyFileSuccess(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "src.jpg")
	dstPath := filepath.Join(tempDir, "sub", "dst.jpg")

	if err := os.MkdirAll(filepath.Join(tempDir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("data"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "data" {
		t.Fatalf("expected 'data', got %q", string(got))
	}
}

func TestCopyFileSourceNotFound(t *testing.T) {
	err := copyFile("/nonexistent/file", filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestCopyFileRemovesDestinationWhenCopyFails(t *testing.T) {
	tempDir := t.TempDir()
	srcDirPath := filepath.Join(tempDir, "srcDir")
	if err := os.MkdirAll(srcDirPath, 0755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	dstPath := filepath.Join(tempDir, "dst.jpg")

	err := copyFile(srcDirPath, dstPath)
	if err == nil {
		t.Fatal("expected copy error")
	}
	if _, statErr := os.Stat(dstPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected destination to be removed after copy failure, stat err=%v", statErr)
	}
}

func TestProcessAssetFailsOnGetAssetError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{}
	emitter := &noopEmitter{}

	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, emitter)
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "fetch asset") {
		t.Fatalf("expected fetch asset error, got: %s", result.Message)
	}
}

func TestProcessAssetSkipsWhenNoMetadataToEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			ExifInfo:         nil,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{}
	emitter := &noopEmitter{}

	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, emitter)
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "no metadata") {
		t.Fatalf("expected 'no metadata' message, got: %s", result.Message)
	}
}

func TestProcessAssetProcessesVideoAssetInDryRun(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/original") {
			w.Write([]byte("fake-video-data"))
			return
		}
		asset := model.AssetResponse{
			ID:               "asset-video-1",
			OriginalFileName: "clip.mp4",
			OriginalMimeType: "video/mp4",
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
	cfg := &model.Config{DryRun: true}
	emitter := &noopEmitter{}

	result := ProcessAsset(client, nil, cfg, "asset-video-1", 1, 1, emitter)
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if result.Message == "video asset skipped" {
		t.Fatalf("expected video asset to be processed, got: %s", result.Message)
	}
}

func TestProcessAssetSkipsUnsupportedVideoAsset(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := model.AssetResponse{
			ID:               "asset-video-1",
			OriginalFileName: "clip.webm",
			OriginalMimeType: "video/webm",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-video-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "unsupported video container") {
		t.Fatalf("expected unsupported video message, got: %s", result.Message)
	}
}

type skipEmitter struct {
	noopEmitter
}

func (e *skipEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	return model.ActionSkip
}

type quitEmitter struct {
	noopEmitter
}

func (e *quitEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	return model.ActionQuit
}

func withMockExiftool(readFn func(string) (exif.ExifTagMap, error), writeFn func(string, []string) error) func() {
	origRead := exif.ReadExifTagsFn
	origWrite := exif.WriteExifTagsFn
	exif.ReadExifTagsFn = readFn
	exif.WriteExifTagsFn = writeFn
	return func() {
		exif.ReadExifTagsFn = origRead
		exif.WriteExifTagsFn = origWrite
	}
}

func assetServerWithExif() *httptest.Server {
	desc := "Test Description"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/original") {
			w.Write([]byte("fake-image-data"))
			return
		}
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
}

func TestProcessAssetDownloadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/original") {
			http.Error(w, "download failed", http.StatusInternalServerError)
			return
		}
		desc := "Test"
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "download") {
		t.Fatalf("expected download error, got: %s", result.Message)
	}
}

func TestProcessAssetReadExifFails(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return nil, fmt.Errorf("exiftool crashed") },
		nil,
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "read exif") {
		t.Fatalf("expected read exif error, got: %s", result.Message)
	}
}

func TestProcessAssetSkipsWhenMetadataAlreadyMatches(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) {
			return exif.ExifTagMap{
				"ImageDescription":      "Test Description",
				"XPComment":             "Test Description",
				"XMP-dc:Description":    "Test Description",
				"IPTC:Caption-Abstract": "Test Description",
			}, nil
		},
		nil,
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "metadata already matches") {
		t.Fatalf("expected 'metadata already matches', got: %s", result.Message)
	}
}

func TestProcessAssetUserSkips(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		nil,
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &skipEmitter{})
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "user skipped") {
		t.Fatalf("expected user skipped, got: %s", result.Message)
	}
}

func TestProcessAssetUserQuits(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		nil,
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &quitEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !result.Cancelled {
		t.Fatal("expected Cancelled=true")
	}
}

func TestProcessAssetWriteExifFails(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return fmt.Errorf("write failed") },
	)()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "write exif") {
		t.Fatalf("expected write exif error, got: %s", result.Message)
	}
}

func TestProcessAssetDryRun(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{DryRun: true}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "dry-run") {
		t.Fatalf("expected dry-run message, got: %s", result.Message)
	}
}

func TestProcessAssetExportMode(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	exportDir := t.TempDir()
	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{ExportDir: exportDir}
	result := ProcessAsset(client, nil, cfg, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "exported to") {
		t.Fatalf("expected export message, got: %s", result.Message)
	}
}

type mockUploader struct {
	outcome   UploadOutcome
	returnErr error
	uploadFn  func(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error)
}

func (u *mockUploader) Upload(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
	if u.uploadFn != nil {
		return u.uploadFn(filePath, asset, emitter)
	}
	return u.outcome, u.returnErr
}

func TestProcessAssetUploadSuccess(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	uploader := &mockUploader{outcome: UploadOutcome{NewID: "new-id", Cacheable: true}}
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if result.NewID != "new-id" {
		t.Fatalf("expected new-id, got %s", result.NewID)
	}
	if !strings.Contains(result.Message, "new-id") {
		t.Fatalf("expected new ID in message, got: %s", result.Message)
	}
}

func TestProcessAssetUploadReplacedInPlace(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	uploader := &mockUploader{outcome: UploadOutcome{NewID: "asset-1", Cacheable: true}}
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "replaced in-place") {
		t.Fatalf("expected replaced in-place, got: %s", result.Message)
	}
}

func TestCopyFileDestinationDirectoryPermissionError(t *testing.T) {
	tempDir := t.TempDir()
	srcPath := filepath.Join(tempDir, "src.txt")
	os.WriteFile(srcPath, []byte("data"), 0644)

	readOnlyDir := filepath.Join(tempDir, "readonly")
	os.MkdirAll(readOnlyDir, 0555)
	defer os.Chmod(readOnlyDir, 0755)

	err := copyFile(srcPath, filepath.Join(readOnlyDir, "dst.txt"))
	if err == nil {
		t.Fatal("expected permission error")
	}
}

func TestProcessAssetErrorMessageContainsAssetID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, nil, &model.Config{}, "abcdef12-3456-7890-abcd-ef1234567890", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "[abcdef12]") {
		t.Fatalf("expected asset ID prefix in message, got: %s", result.Message)
	}
}

func TestProcessAssetRetriesUploadOnTransientFailure(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	attempts := 0
	uploader := &mockUploader{
		uploadFn: func(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
			attempts++
			if attempts < 3 {
				return UploadOutcome{}, fmt.Errorf("transient error")
			}
			return UploadOutcome{NewID: "new-id", Cacheable: true}, nil
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success after retry, got %s: %s", result.Status, result.Message)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestProcessAssetRetriesExhausted(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	attempts := 0
	uploader := &mockUploader{
		uploadFn: func(filePath string, asset *model.AssetResponse, emitter model.EventEmitter) (UploadOutcome, error) {
			attempts++
			return UploadOutcome{}, fmt.Errorf("persistent error")
		},
	}

	client := api.NewImmichClient(server.URL, "key")
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed after retries exhausted, got %s", result.Status)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if !strings.Contains(result.Message, "after 3 attempts") {
		t.Fatalf("expected retry count in message, got: %s", result.Message)
	}
}

func TestProcessAssetUploadNonCacheableIsSkipped(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	uploader := &mockUploader{
		outcome: UploadOutcome{
			NewID:       "dup-id",
			Message:     "upload status=duplicate (asset ID: dup-id), skipped copy/delete",
			Cacheable:   false,
			DuplicateID: "dup-id",
		},
	}

	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusSkipped {
		t.Fatalf("expected skipped, got %s", result.Status)
	}
	if result.NewID != "" {
		t.Fatalf("expected no new ID for non-cacheable outcome, got %s", result.NewID)
	}
	if result.DuplicateID != "dup-id" {
		t.Fatalf("expected duplicate ID dup-id, got %s", result.DuplicateID)
	}
	if !strings.Contains(result.Message, "duplicate") {
		t.Fatalf("expected duplicate message, got: %s", result.Message)
	}
}

func TestProcessAssetUploadFails(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	client := api.NewImmichClient(server.URL, "key")
	uploader := &mockUploader{returnErr: fmt.Errorf("upload boom")}
	result := ProcessAsset(client, uploader, &model.Config{}, "asset-1", 1, 1, &noopEmitter{})
	if result.Status != model.StatusFailed {
		t.Fatalf("expected failed, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "upload") {
		t.Fatalf("expected upload error, got: %s", result.Message)
	}
}
