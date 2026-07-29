package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestGetAssetFaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/faces" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("id"); got != "asset-1" {
			t.Fatalf("unexpected id query: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]model.AssetFaceResponse{
			{
				BoundingBoxX1: 100, BoundingBoxY1: 50, BoundingBoxX2: 300, BoundingBoxY2: 250,
				ImageWidth: 1000, ImageHeight: 500,
				Person: &model.PersonResponse{ID: "p1", Name: "Alice"},
			},
			{BoundingBoxX1: 1, BoundingBoxY1: 1, BoundingBoxX2: 2, BoundingBoxY2: 2, ImageWidth: 10, ImageHeight: 10},
		})
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	faces, err := c.GetAssetFaces("asset-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(faces) != 2 {
		t.Fatalf("expected 2 faces, got %d", len(faces))
	}
	if faces[0].Person == nil || faces[0].Person.Name != "Alice" {
		t.Fatalf("unexpected person: %+v", faces[0].Person)
	}
	if faces[1].Person != nil {
		t.Fatalf("expected unassigned face to have nil person, got %+v", faces[1].Person)
	}
}

func TestGetAssetFacesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	if _, err := c.GetAssetFaces("asset-1"); err == nil {
		t.Fatal("expected error on server failure")
	}
}
