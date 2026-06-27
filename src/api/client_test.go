package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestNewImmichClientSetsFields(t *testing.T) {
	c := NewImmichClient("https://example.com", "my-key")
	if c.baseURL != "https://example.com" {
		t.Fatalf("expected baseURL https://example.com, got %s", c.baseURL)
	}
	if c.apiKey != "my-key" {
		t.Fatalf("expected apiKey my-key, got %s", c.apiKey)
	}
	if c.httpClient == nil {
		t.Fatal("expected httpClient to be set")
	}
}

func TestNewRequestSetsAPIKeyHeader(t *testing.T) {
	c := NewImmichClient("https://example.com", "secret-key")
	req, err := c.newRequest(http.MethodGet, "/assets/123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL.String() != "https://example.com/api/assets/123" {
		t.Fatalf("unexpected URL: %s", req.URL.String())
	}
	if req.Header.Get("x-api-key") != "secret-key" {
		t.Fatalf("expected x-api-key header, got %q", req.Header.Get("x-api-key"))
	}
}

func TestDoRequestReturnsErrorOn4xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found detail", http.StatusNotFound)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	_, err := c.doRequest(req)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not found detail") {
		t.Fatalf("expected body in error, got: %v", err)
	}
}

func TestDoRequestReturnsErrorOn5xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	_, err := c.doRequest(req)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 in error, got: %v", err)
	}
}

func TestDoRequestSucceedsOn200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	resp, err := c.doRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDoJSONDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","status":"created"}`))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	var resp model.UploadResponse
	if err := c.doJSON(req, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "abc" {
		t.Fatalf("expected id abc, got %s", resp.ID)
	}
	if resp.Status != "created" {
		t.Fatalf("expected status created, got %s", resp.Status)
	}
}

func TestNewRequestInvalidURL(t *testing.T) {
	c := NewImmichClient("://invalid", "key")
	_, err := c.newRequest(http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestIsV3Version(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"3.0.0-rc.3", true},
		{"v3.1.0", true},
		{"3", true},
		{"4.2.0", true},
		{"2.9.9", false},
		{"1.119.0", false},
		{"", false},
		{"garbage", false},
	}
	for _, tc := range cases {
		if got := isV3Version(tc.version); got != tc.want {
			t.Fatalf("isV3Version(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}

func aboutServer(version string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/server/about" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
	}))
}

func TestResolveAPIModeAutoDetectsV3(t *testing.T) {
	server := aboutServer("3.0.0-rc.3")
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	if err := c.ResolveAPIMode("auto"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.apiV3 {
		t.Fatal("expected apiV3=true for a 3.x server")
	}
}

func TestResolveAPIModeAutoDetectsLegacy(t *testing.T) {
	server := aboutServer("1.119.0")
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	if err := c.ResolveAPIMode("auto"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.apiV3 {
		t.Fatal("expected apiV3=false for a 1.x server")
	}
}

func TestResolveAPIModeOverridesIgnoreVersion(t *testing.T) {
	legacyServer := aboutServer("1.0.0")
	defer legacyServer.Close()
	cV3 := NewImmichClient(legacyServer.URL, "key")
	if err := cV3.ResolveAPIMode("v3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cV3.apiV3 {
		t.Fatal("v3 override must win over a 1.x version")
	}

	v3Server := aboutServer("3.0.0")
	defer v3Server.Close()
	cLegacy := NewImmichClient(v3Server.URL, "key")
	if err := cLegacy.ResolveAPIMode("legacy"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cLegacy.apiV3 {
		t.Fatal("legacy override must win over a 3.x version")
	}
}

func TestResolveAPIModeFailsOnUnreachableServer(t *testing.T) {
	c := NewImmichClient("http://localhost:1", "key")
	if err := c.ResolveAPIMode("legacy"); err == nil {
		t.Fatal("expected connectivity error")
	}
}

func TestDoJSONReturnsErrorOnBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	var resp model.UploadResponse
	if err := c.doJSON(req, &resp); err == nil {
		t.Fatal("expected error for bad JSON")
	}
}
