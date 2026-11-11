package process

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func pathChecksum(path string) string {
	sum := sha1.Sum([]byte("path:" + path))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func externalAssetServer(t *testing.T, fileBody string) *httptest.Server {
	t.Helper()
	desc := "external desc"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/assets/asset-1/original":
			_, _ = w.Write([]byte(fileBody))
		case "/api/assets/asset-1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(model.AssetResponse{
				ID:               "asset-1",
				OriginalFileName: "ext1.jpg",
				OriginalPath:     "/mnt/external/ext1.jpg",
				Checksum:         pathChecksum("/mnt/external/ext1.jpg"),
				LibraryID:        "library-1",
				ExifInfo:         &model.ExifInfo{Description: &desc},
			})
		default:
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
	}))
}

func TestProcessAssetDryRunsExternalLibraryAsset(t *testing.T) {
	server := externalAssetServer(t, "external file bytes")
	defer server.Close()
	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	result := ProcessAsset(api.NewImmichClient(server.URL, "key"), nil, &model.Config{DryRun: true}, "asset-1", 1, 1, &noopEmitter{}, nil)
	if result.Status != model.StatusSuccess || !strings.Contains(result.Message, "dry-run") {
		t.Fatalf("an external asset's path checksum must not fail the download, got %s: %s", result.Status, result.Message)
	}
}
