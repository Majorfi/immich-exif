package api

import (
	"encoding/json"
	"errors"
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

func TestCreateFace(t *testing.T) {
	var received model.CreateFaceRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/faces" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	face := model.CreateFaceRequest{
		AssetID: "new-asset", PersonID: "p1",
		X: 100, Y: 50, Width: 200, Height: 200,
		ImageWidth: 1000, ImageHeight: 500,
	}
	if err := c.CreateFace(face); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != face {
		t.Fatalf("server received %+v, want %+v", received, face)
	}
}

func TestCreateFaceUnsupportedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	err := c.CreateFace(model.CreateFaceRequest{AssetID: "a", PersonID: "p"})
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a 404 StatusError (server too old to skip gracefully), got %v", err)
	}
}
