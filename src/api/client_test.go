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

func TestPingSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/server/about" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Fatal("expected x-api-key header")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"version":"1.0"}`))
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "my-key")
	if err := c.Ping(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPingFailsOnUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewImmichClient(server.URL, "bad-key")
	err := c.Ping()
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
}

func TestPingFailsOnBadURL(t *testing.T) {
	c := NewImmichClient("http://localhost:1", "key")
	err := c.Ping()
	if err == nil {
		t.Fatal("expected error for unreachable server")
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
