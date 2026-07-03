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
		// v3 is the primary target: unrecognizable versions assume v3.
		{"", true},
		{"garbage", true},
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

func TestResolveAPIModeAutoFailsOnUnreachableServer(t *testing.T) {
	c := NewImmichClient("http://localhost:1", "key")
	if err := c.ResolveAPIMode("auto"); err == nil {
		t.Fatal("expected connectivity error")
	}
}

// A forced mode must not require the server.about permission (or the endpoint
// at all): a least-privilege API key with -immich-api v3/legacy still works.
func TestResolveAPIModeForcedModesTolerateAboutFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	cV3 := NewImmichClient(server.URL, "key")
	if err := cV3.ResolveAPIMode("v3"); err != nil {
		t.Fatalf("forced v3 must not fail on about error: %v", err)
	}
	if !cV3.apiV3 {
		t.Fatal("expected apiV3=true for forced v3")
	}

	cLegacy := NewImmichClient(server.URL, "key")
	if err := cLegacy.ResolveAPIMode("legacy"); err != nil {
		t.Fatalf("forced legacy must not fail on about error: %v", err)
	}
	if cLegacy.apiV3 {
		t.Fatal("expected apiV3=false for forced legacy")
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

func TestClientRefusesCrossOriginRedirect(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the redirect target must never be contacted")
	}))
	defer other.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/assets/x", http.StatusMovedPermanently)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "secret-key")
	req, _ := c.newRequest(http.MethodGet, "/assets/x", nil)
	_, err := c.doRequest(req)
	if err == nil {
		t.Fatal("expected cross-origin redirect to be refused")
	}
	if !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("expected redirect refusal, got: %v", err)
	}
}

func TestClientFollowsSameOriginRedirect(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	mux.HandleFunc("/api/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+"/api/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/api/new", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/old", nil)
	resp, err := c.doRequest(req)
	if err != nil {
		t.Fatalf("same-origin redirect must be allowed: %v", err)
	}
	resp.Body.Close()
}

func TestDoRequestTruncatesAndSanitizesErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("\x1b[2Jbad "))
		_, _ = w.Write(make([]byte, 32*1024))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "key")
	req, _ := c.newRequest(http.MethodGet, "/test", nil)
	_, err := c.doRequest(req)
	if err == nil {
		t.Fatal("expected error for 502")
	}
	if strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("expected escape sequences to be stripped from the error body")
	}
	if len(err.Error()) > 10*1024 {
		t.Fatalf("expected the error body to be capped, got %d bytes", len(err.Error()))
	}
}

func TestParseVersionMajorMinor(t *testing.T) {
	cases := []struct {
		version string
		major   int
		minor   int
		ok      bool
	}{
		{"1.133.0", 1, 133, true},
		{"v1.119.0", 1, 119, true},
		{"2.5.6", 2, 5, true},
		{"3.0.0-rc.3", 3, 0, true},
		{"3", 3, 0, true},
		{"garbage", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tc := range cases {
		major, minor, ok := parseVersionMajorMinor(tc.version)
		if major != tc.major || minor != tc.minor || ok != tc.ok {
			t.Fatalf("parseVersionMajorMinor(%q) = (%d, %d, %v), want (%d, %d, %v)", tc.version, major, minor, ok, tc.major, tc.minor, tc.ok)
		}
	}
}
