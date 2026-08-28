package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestUpdateStackPrimaryUsesPatchOnV3AndPutOnLegacy(t *testing.T) {
	for _, testCase := range []struct {
		mode       string
		wantMethod string
	}{
		{mode: "v3", wantMethod: http.MethodPatch},
		{mode: "legacy", wantMethod: http.MethodPut},
	} {
		t.Run(testCase.mode, func(t *testing.T) {
			var gotMethod, gotPath string
			var payload model.UpdateStackRequest

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/server/about" {
					_, _ = w.Write([]byte(`{"version":"v3.1.0"}`))
					return
				}
				gotMethod, gotPath = r.Method, r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode payload: %v", err)
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			client := NewImmichClient(server.URL, "test-key")
			if err := client.ResolveAPIMode(testCase.mode); err != nil {
				t.Fatalf("resolve api mode: %v", err)
			}
			if err := client.UpdateStackPrimary("stack-id", "new-id"); err != nil {
				t.Fatalf("update stack primary: %v", err)
			}

			if gotMethod != testCase.wantMethod {
				t.Fatalf("expected %s, got %s", testCase.wantMethod, gotMethod)
			}
			if gotPath != "/api/stacks/stack-id" {
				t.Fatalf("unexpected path %s", gotPath)
			}
			if payload.PrimaryAssetID != "new-id" {
				t.Fatalf("expected primaryAssetId new-id, got %q", payload.PrimaryAssetID)
			}
		})
	}
}

func TestUpdateStackPrimaryReturnsErrorOnForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
	}))
	defer server.Close()

	if err := NewImmichClient(server.URL, "test-key").UpdateStackPrimary("stack-id", "new-id"); err == nil {
		t.Fatal("expected an error for a 403 response")
	}
}

func TestListStacksDecodesStacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stacks" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"stack-id","primaryAssetId":"old-id","assets":[{"id":"live-id","originalFileName":"a.jpg"}]}]`))
	}))
	defer server.Close()

	stacks, err := NewImmichClient(server.URL, "test-key").ListStacks()
	if err != nil {
		t.Fatalf("list stacks: %v", err)
	}
	if len(stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(stacks))
	}
	if stacks[0].PrimaryAssetID != "old-id" || len(stacks[0].Assets) != 1 || stacks[0].Assets[0].ID != "live-id" {
		t.Fatalf("unexpected stack payload: %+v", stacks[0])
	}
}
