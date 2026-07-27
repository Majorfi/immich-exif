package process

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

// recreateFacesServer serves GET /api/faces?id= from a per-asset map and
// records every POST /api/faces body. A non-zero postStatus overrides the
// created response, to exercise the too-old (404) path.
func recreateFacesServer(t *testing.T, facesByAsset map[string][]model.AssetFaceResponse, postStatus int) (*httptest.Server, *[]model.CreateFaceRequest) {
	t.Helper()
	var mu sync.Mutex
	var posted []model.CreateFaceRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/faces" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(facesByAsset[r.URL.Query().Get("id")])
		case http.MethodPost:
			if postStatus != 0 {
				http.Error(w, "nope", postStatus)
				return
			}
			var req model.CreateFaceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode POST body: %v", err)
			}
			mu.Lock()
			posted = append(posted, req)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	return server, &posted
}

func personFace(personID string, x1, y1, x2, y2, iw, ih int) model.AssetFaceResponse {
	face := model.AssetFaceResponse{
		BoundingBoxX1: x1, BoundingBoxY1: y1, BoundingBoxX2: x2, BoundingBoxY2: y2,
		ImageWidth: iw, ImageHeight: ih,
	}
	if personID != "" {
		face.Person = &model.PersonResponse{ID: personID}
	}
	return face
}

func TestRecreateFacesCopiesAssignedFaces(t *testing.T) {
	source := []model.AssetFaceResponse{
		personFace("alice", 100, 50, 300, 250, 1000, 500),
		personFace("bob", 400, 60, 500, 160, 1000, 500),
		personFace("", 1, 1, 2, 2, 10, 10), // unassigned — no personId, must be skipped
	}
	server, posted := recreateFacesServer(t, map[string][]model.AssetFaceResponse{
		"old": source,
		"new": nil, // fresh upload, ML has not run yet
	}, 0)
	defer server.Close()

	created, err := recreateFaces(api.NewImmichClient(server.URL, "key"), "old", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created != 2 {
		t.Fatalf("expected 2 faces created, got %d", created)
	}
	if len(*posted) != 2 {
		t.Fatalf("expected 2 POSTs, got %d", len(*posted))
	}
	alice := (*posted)[0]
	if alice.AssetID != "new" || alice.PersonID != "alice" || alice.Width != 200 || alice.Height != 200 || alice.X != 100 || alice.Y != 50 {
		t.Fatalf("alice face not recreated verbatim: %+v", alice)
	}
}

func TestRecreateFacesSkipsPeopleAlreadyLinked(t *testing.T) {
	server, posted := recreateFacesServer(t, map[string][]model.AssetFaceResponse{
		"old": {personFace("alice", 100, 50, 300, 250, 1000, 500), personFace("bob", 0, 0, 10, 10, 1000, 500)},
		"new": {personFace("alice", 0, 0, 10, 10, 800, 400)}, // recognition already re-linked alice
	}, 0)
	defer server.Close()

	created, err := recreateFaces(api.NewImmichClient(server.URL, "key"), "old", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected only bob created, got %d", created)
	}
	if len(*posted) != 1 || (*posted)[0].PersonID != "bob" {
		t.Fatalf("expected a single bob POST, got %+v", *posted)
	}
}

func TestRecreateFacesNoAssignedFacesDoesNothing(t *testing.T) {
	server, posted := recreateFacesServer(t, map[string][]model.AssetFaceResponse{
		"old": {personFace("", 1, 1, 2, 2, 10, 10)},
	}, 0)
	defer server.Close()

	created, err := recreateFaces(api.NewImmichClient(server.URL, "key"), "old", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created != 0 || len(*posted) != 0 {
		t.Fatalf("expected no work, got created=%d posts=%d", created, len(*posted))
	}
}

func TestRecreateFacesTooOldServer(t *testing.T) {
	server, _ := recreateFacesServer(t, map[string][]model.AssetFaceResponse{
		"old": {personFace("alice", 100, 50, 300, 250, 1000, 500)},
		"new": nil,
	}, http.StatusNotFound)
	defer server.Close()

	_, err := recreateFaces(api.NewImmichClient(server.URL, "key"), "old", "new")
	if !errors.Is(err, errFacePreserveUnsupported) {
		t.Fatalf("expected errFacePreserveUnsupported on a 404, got %v", err)
	}
}
