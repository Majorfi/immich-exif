package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func peopleOnlySearchServer(t *testing.T, wantWithPeople bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body model.SearchMetadataRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		if body.WithPeople != wantWithPeople {
			t.Errorf("expected withPeople=%v, got %v", wantWithPeople, body.WithPeople)
		}
		items := []model.AssetResponse{}
		if body.Visibility == "timeline" {
			asset := model.AssetResponse{ID: "faces-only-1", OriginalFileName: "photo.jpg"}
			if body.WithPeople {
				asset.People = []model.PersonResponse{{ID: "p1", Name: "Alice"}}
			}
			items = append(items, asset)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(model.SearchMetadataResponse{Assets: model.SearchAssets{Items: items}})
	}))
}

func TestListAllAssetIDsWithFacesKeepsPeopleOnlyAssets(t *testing.T) {
	server := peopleOnlySearchServer(t, true)
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, stats, err := c.ListAllAssetIDs(nil, true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "faces-only-1" {
		t.Fatalf("an asset whose only writable metadata is faces must survive the pre-filter, got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 0 {
		t.Fatalf("expected 0 no-metadata skips, got %d", stats.NoWritableMetadataSkipped)
	}
}

func TestListAllAssetIDsWithoutFacesSkipsPeopleOnlyAssets(t *testing.T) {
	server := peopleOnlySearchServer(t, false)
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	ids, stats, err := c.ListAllAssetIDs(nil, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("without -faces a people-only asset has nothing to write, got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 1 {
		t.Fatalf("expected 1 no-metadata skip, got %d", stats.NoWritableMetadataSkipped)
	}
}
