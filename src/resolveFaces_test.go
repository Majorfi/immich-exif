package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

func TestResolveAssetIDsFacesGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body model.SearchMetadataRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
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
	defer server.Close()

	client := api.NewImmichClient(server.URL, "key")

	ids, _, err := resolveAssetIDs(client, &model.Config{All: true, Faces: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "faces-only-1" {
		t.Fatalf("-faces must request people and keep faces-only assets, got %v", ids)
	}

	ids, stats, err := resolveAssetIDs(client, &model.Config{All: true}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("without -faces the people-only asset has nothing to write, got %v", ids)
	}
	if stats.NoWritableMetadataSkipped != 1 {
		t.Fatalf("expected 1 no-metadata skip, got %d", stats.NoWritableMetadataSkipped)
	}
}
