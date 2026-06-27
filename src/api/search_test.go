package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestParseNextPage(t *testing.T) {
	testCases := []struct {
		name     string
		nextPage *string
		wantPage int
		wantDone bool
		wantErr  bool
	}{
		{
			name:     "nil token means done",
			nextPage: nil,
			wantDone: true,
		},
		{
			name:     "empty token means done",
			nextPage: stringPtr(""),
			wantDone: true,
		},
		{
			name:     "null token means done",
			nextPage: stringPtr("null"),
			wantDone: true,
		},
		{
			name:     "uppercase null token means done",
			nextPage: stringPtr("NULL"),
			wantDone: true,
		},
		{
			name:     "numeric token parsed",
			nextPage: stringPtr("2"),
			wantPage: 2,
		},
		{
			name:     "numeric token with spaces parsed",
			nextPage: stringPtr("  12  "),
			wantPage: 12,
		},
		{
			name:     "invalid token returns error",
			nextPage: stringPtr("abc"),
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotPage, gotDone, err := parseNextPage(tc.nextPage)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotDone != tc.wantDone {
				t.Fatalf("expected done=%v, got done=%v", tc.wantDone, gotDone)
			}
			if gotPage != tc.wantPage {
				t.Fatalf("expected page=%d, got page=%d", tc.wantPage, gotPage)
			}
		})
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestSearchAssetsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/search/metadata" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body model.SearchMetadataRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Page != 1 || body.Size != 10 {
			t.Fatalf("unexpected page/size: %d/%d", body.Page, body.Size)
		}
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items:    []model.AssetResponse{{ID: "a1"}, {ID: "a2"}},
				NextPage: nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	result, err := c.searchAssetsPage(1, 10, nil, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].ID != "a1" {
		t.Fatalf("expected a1, got %s", result.Items[0].ID)
	}
}

func TestListAllAssetIDsPaginates(t *testing.T) {
	callCount := 0
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var body model.SearchMetadataRequest
		json.NewDecoder(r.Body).Decode(&body)

		var resp model.SearchMetadataResponse
		switch body.Page {
		case 1:
			next := "2"
			resp = model.SearchMetadataResponse{
				Assets: model.SearchAssets{
					Items: []model.AssetResponse{
						{ID: "a1", ExifInfo: &model.ExifInfo{Description: &desc}},
						{ID: "a2", ExifInfo: &model.ExifInfo{Description: &desc}},
					},
					NextPage: &next,
				},
			}
		case 2:
			resp = model.SearchMetadataResponse{
				Assets: model.SearchAssets{
					Items:    []model.AssetResponse{{ID: "a3", ExifInfo: &model.ExifInfo{Description: &desc}}},
					NextPage: nil,
				},
			}
		default:
			t.Fatalf("unexpected page: %d", body.Page)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, stats, err := c.ListAllAssetIDs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "a1" || ids[1] != "a2" || ids[2] != "a3" {
		t.Fatalf("unexpected IDs: %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 0 {
		t.Fatalf("expected 0 noWritableMetadata skipped, got %d", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped != 0 {
		t.Fatalf("expected 0 unsupportedVideoSkipped, got %d", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped != 0 {
		t.Fatalf("expected 0 stateSkipped, got %d", stats.StateSkipped)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 API calls, got %d", callCount)
	}
}

func TestListAllAssetIDsDetectsInvalidNextPage(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := "1"
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items:    []model.AssetResponse{{ID: "a1", ExifInfo: &model.ExifInfo{Description: &desc}}},
				NextPage: &next,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	_, _, err := c.ListAllAssetIDs(nil)
	if err == nil {
		t.Fatal("expected error for non-advancing nextPage")
	}
}

func TestGetAlbumAssetsRejectsEmptyAlbumID(t *testing.T) {
	c := NewImmichClient("http://example.com", "key")
	if _, err := c.GetAlbumAssets("   "); err == nil {
		t.Fatal("expected error for empty album ID")
	}
	c.apiV3 = true
	if _, err := c.GetAlbumAssets(""); err == nil {
		t.Fatal("expected error for empty album ID in v3 mode")
	}
}

func TestGetAlbumAssetsV3UsesSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search/metadata" {
			t.Fatalf("v3 album lookup must use search/metadata, got: %s", r.URL.Path)
		}
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{Items: []model.AssetResponse{{ID: "a1"}, {ID: "a2"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	c.apiV3 = true
	ids, err := c.GetAlbumAssets("album-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "a1" || ids[1] != "a2" {
		t.Fatalf("expected [a1 a2], got %v", ids)
	}
}

func TestGetAlbumAssetsLegacyFallsBackWhenAssetsOmitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/albums/album-1":
			json.NewEncoder(w).Encode(model.AlbumResponse{ID: "album-1", AssetCount: 2})
		case "/api/search/metadata":
			json.NewEncoder(w).Encode(model.SearchMetadataResponse{
				Assets: model.SearchAssets{Items: []model.AssetResponse{{ID: "a1"}, {ID: "a2"}}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, err := c.GetAlbumAssets("album-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected fallback to return 2 IDs, got %d (%v)", len(ids), ids)
	}
}

func TestGetAlbumAssetsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/albums/album-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := model.AlbumResponse{
			ID:     "album-1",
			Assets: []model.AssetResponse{{ID: "x1"}, {ID: "x2"}, {ID: "x3"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, err := c.GetAlbumAssets("album-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "x1" || ids[1] != "x2" || ids[2] != "x3" {
		t.Fatalf("unexpected IDs: %v", ids)
	}
}

func TestListAlbumIDsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/albums" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := []model.AlbumResponse{
			{ID: "album-1"},
			{ID: "album-2"},
			{ID: ""},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	albumIDs, err := c.ListAlbumIDs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(albumIDs) != 2 {
		t.Fatalf("expected 2 album IDs, got %d", len(albumIDs))
	}
	if albumIDs[0] != "album-1" || albumIDs[1] != "album-2" {
		t.Fatalf("expected [album-1 album-2], got %v", albumIDs)
	}
}

func TestGetAlbumAssetsEmptyAlbum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.AlbumResponse{ID: "album-empty", Assets: []model.AssetResponse{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, err := c.GetAlbumAssets("album-empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs, got %d", len(ids))
	}
}

func TestListAlbumIDsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	_, err := c.ListAlbumIDs()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetAlbumAssetsIncludesVideoAssets(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.AlbumResponse{
			ID: "album-1",
			Assets: []model.AssetResponse{
				{ID: "photo-1", OriginalFileName: "photo.jpg", OriginalMimeType: "image/jpeg", ExifInfo: &model.ExifInfo{Description: &desc}},
				{ID: "video-1", OriginalFileName: "video.mp4", OriginalMimeType: "video/mp4", ExifInfo: &model.ExifInfo{Description: &desc}},
				{ID: "photo-2", OriginalFileName: "photo2.heic", OriginalMimeType: "image/heic", ExifInfo: &model.ExifInfo{Description: &desc}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, err := c.GetAlbumAssets("album-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "photo-1" || ids[1] != "video-1" || ids[2] != "photo-2" {
		t.Fatalf("expected [photo-1, video-1, photo-2], got %v", ids)
	}
}

func TestGetAlbumAssetsIncludesUnsupportedVideoContainers(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.AlbumResponse{
			ID: "album-1",
			Assets: []model.AssetResponse{
				{ID: "photo-1", OriginalFileName: "photo.jpg", OriginalMimeType: "image/jpeg", ExifInfo: &model.ExifInfo{Description: &desc}},
				{ID: "video-1", OriginalFileName: "clip.webm", OriginalMimeType: "video/webm", ExifInfo: &model.ExifInfo{Description: &desc}},
				{ID: "video-2", OriginalFileName: "clip.mp4", OriginalMimeType: "video/mp4", ExifInfo: &model.ExifInfo{Description: &desc}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, err := c.GetAlbumAssets("album-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "photo-1" || ids[1] != "video-1" || ids[2] != "video-2" {
		t.Fatalf("expected [photo-1, video-1, video-2], got %v", ids)
	}
}

func TestListAllAssetIDsPreFiltersNoMetadata(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items: []model.AssetResponse{
					{ID: "has-meta", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "no-meta", ExifInfo: nil},
					{ID: "empty-meta", ExifInfo: &model.ExifInfo{}},
				},
				NextPage: nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, stats, err := c.ListAllAssetIDs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "has-meta" {
		t.Fatalf("expected [has-meta], got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 2 {
		t.Fatalf("expected 2 noWritableMetadata skipped, got %d", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped != 0 {
		t.Fatalf("expected 0 unsupportedVideoSkipped, got %d", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped != 0 {
		t.Fatalf("expected 0 stateSkipped, got %d", stats.StateSkipped)
	}
}

func TestSearchAssetsReturnsErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	_, err := c.searchAssetsPage(1, 10, nil, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAllAssetIDsReturnsErrorOnSearchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	_, _, err := c.ListAllAssetIDs(nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAllAssetIDsReturnsErrorOnInvalidNextPageToken(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := "abc"
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items:    []model.AssetResponse{{ID: "a1", ExifInfo: &model.ExifInfo{Description: &desc}}},
				NextPage: &next,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	_, _, err := c.ListAllAssetIDs(nil)
	if err == nil {
		t.Fatal("expected error for invalid nextPage token")
	}
}

func TestListAllAssetIDsWithShouldSkip(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items: []model.AssetResponse{
					{ID: "a1", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "a2", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "a3", ExifInfo: &model.ExifInfo{Description: &desc}},
				},
				NextPage: nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	shouldSkip := func(asset model.AssetResponse) bool {
		return asset.ID == "a2"
	}

	ids, stats, err := c.ListAllAssetIDs(shouldSkip)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != "a1" || ids[1] != "a3" {
		t.Fatalf("expected [a1, a3], got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 0 {
		t.Fatalf("expected 0 noWritableMetadata, got %d", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped != 0 {
		t.Fatalf("expected 0 unsupportedVideoSkipped, got %d", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped != 1 {
		t.Fatalf("expected 1 stateSkipped, got %d", stats.StateSkipped)
	}
}

func TestListAllAssetIDsIncludesVideoAssets(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items: []model.AssetResponse{
					{ID: "a1", OriginalFileName: "image.jpg", OriginalMimeType: "image/jpeg", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "v1", OriginalFileName: "clip.mp4", OriginalMimeType: "video/mp4", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "a2", OriginalFileName: "image2.jpg", OriginalMimeType: "image/jpeg", ExifInfo: &model.ExifInfo{Description: &desc}},
				},
				NextPage: nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, stats, err := c.ListAllAssetIDs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}
	if ids[0] != "a1" || ids[1] != "v1" || ids[2] != "a2" {
		t.Fatalf("expected [a1, v1, a2], got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 0 {
		t.Fatalf("expected 0 noWritableMetadata, got %d", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped != 0 {
		t.Fatalf("expected 0 unsupportedVideoSkipped, got %d", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped != 0 {
		t.Fatalf("expected 0 stateSkipped, got %d", stats.StateSkipped)
	}
}

func TestListAllAssetIDsSkipsUnsupportedVideoContainers(t *testing.T) {
	desc := "test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.SearchMetadataResponse{
			Assets: model.SearchAssets{
				Items: []model.AssetResponse{
					{ID: "a1", OriginalFileName: "image.jpg", OriginalMimeType: "image/jpeg", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "v1", OriginalFileName: "clip.webm", OriginalMimeType: "video/webm", ExifInfo: &model.ExifInfo{Description: &desc}},
					{ID: "v2", OriginalFileName: "clip.mp4", OriginalMimeType: "video/mp4", ExifInfo: &model.ExifInfo{Description: &desc}},
				},
				NextPage: nil,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, stats, err := c.ListAllAssetIDs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != "a1" || ids[1] != "v2" {
		t.Fatalf("expected [a1, v2], got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 0 {
		t.Fatalf("expected 0 noWritableMetadata, got %d", stats.NoWritableMetadataSkipped)
	}
	if stats.UnsupportedVideoSkipped != 1 {
		t.Fatalf("expected 1 unsupportedVideoSkipped, got %d", stats.UnsupportedVideoSkipped)
	}
	if stats.StateSkipped != 0 {
		t.Fatalf("expected 0 stateSkipped, got %d", stats.StateSkipped)
	}
}
