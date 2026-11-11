package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetSidecarPathsListsTheAssetSidecars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/asset-files" || r.URL.Query().Get("assetId") != "asset-1" || r.URL.Query().Get("type") != "sidecar" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`[{"path":"/mnt/a.jpg.xmp","type":"sidecar"},{"path":"/mnt/a.xmp","type":"sidecar"}]`))
	}))
	defer server.Close()

	paths, err := NewImmichClient(server.URL, "key").GetSidecarPaths("asset-1")
	if err != nil || strings.Join(paths, ",") != "/mnt/a.jpg.xmp,/mnt/a.xmp" {
		t.Fatalf("got %v (err %v)", paths, err)
	}
}

func TestRefreshMetadataQueuesTheJob(t *testing.T) {
	var body struct {
		AssetIDs []string `json:"assetIds"`
		Name     string   `json:"name"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/assets/jobs" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewImmichClient(server.URL, "key").RefreshMetadata("asset-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body.Name != "refresh-metadata" || len(body.AssetIDs) != 1 || body.AssetIDs[0] != "asset-1" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestRefreshMetadataReportsADenial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	if err := NewImmichClient(server.URL, "key").RefreshMetadata("asset-1"); err == nil {
		t.Fatal("expected the denial to surface")
	}
}

func TestEditReadersDecodeTheServerResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/assets/asset-1/edits":
			_, _ = w.Write([]byte(`{"assetId":"asset-1","edits":[{"id":"e1","action":"rotate","parameters":{"angle":90}}]}`))
		case r.URL.Path == "/api/assets/asset-1/thumbnail" && r.URL.Query().Get("size") == "preview" && r.URL.Query().Get("edited") == "false":
			_, _ = io.WriteString(w, "preview bytes")
		case r.URL.Path == "/api/server/ping":
			w.Header().Set("Date", "Wed, 11 Nov 2026 10:00:00 GMT")
			_, _ = w.Write([]byte(`{"res":"pong"}`))
		default:
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
	}))
	defer server.Close()
	client := NewImmichClient(server.URL, "key")

	edits, err := client.GetAssetEdits("asset-1")
	if err != nil || len(edits) != 1 || edits[0].Action != "rotate" || edits[0].Params().Angle != 90 {
		t.Fatalf("edits: %+v (err %v)", edits, err)
	}
	preview, err := client.GetUneditedPreview("asset-1")
	if err != nil || string(preview) != "preview bytes" {
		t.Fatalf("preview: %q (err %v)", preview, err)
	}
	now, err := client.ServerTime()
	if err != nil || !now.Equal(time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("server time: %v (err %v)", now, err)
	}
}
