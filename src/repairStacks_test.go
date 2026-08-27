package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

type stackFixture struct {
	stacks      string
	assets      map[string]string
	denied      map[string]int
	updateFails map[string]int
	updates     map[string]string
}

func newStackServer(t *testing.T, fixture *stackFixture) *httptest.Server {
	t.Helper()
	fixture.updates = map[string]string{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/server/about":
			_, _ = w.Write([]byte(`{"version":"v3.1.0"}`))
		case r.URL.Path == "/api/stacks":
			if fixture.stacks == "" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fixture.stacks))
		case strings.HasPrefix(r.URL.Path, "/api/assets/"):
			assetID := strings.TrimPrefix(r.URL.Path, "/api/assets/")
			if status, denied := fixture.denied[assetID]; denied {
				w.WriteHeader(status)
				return
			}
			body, ok := fixture.assets[assetID]
			if !ok {
				t.Fatalf("unexpected asset fetch: %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		case strings.HasPrefix(r.URL.Path, "/api/stacks/"):
			stackID := strings.TrimPrefix(r.URL.Path, "/api/stacks/")
			if status, fails := fixture.updateFails[stackID]; fails {
				w.WriteHeader(status)
				return
			}
			var payload model.UpdateStackRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode stack payload: %v", err)
			}
			fixture.updates[stackID] = payload.PrimaryAssetID
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func runRepair(t *testing.T, fixture *stackFixture, cfg *model.Config) int {
	t.Helper()
	server := newStackServer(t, fixture)
	defer server.Close()

	client := api.NewImmichClient(server.URL, "test-key")
	if err := client.ResolveAPIMode("v3"); err != nil {
		t.Fatalf("resolve api mode: %v", err)
	}
	return repairStacks(client, cfg)
}

func TestRepairStacksPromotesLiveMemberWhenPrimaryIsTrashed(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"stack-id","primaryAssetId":"old-id","assets":[{"id":"new-id","originalFileName":"a.jpg"},{"id":"sibling-id","originalFileName":"b.jpg"}]}]`,
		assets: map[string]string{"old-id": `{"id":"old-id","isTrashed":true}`},
	}
	if code := runRepair(t, fixture, &model.Config{Yes: true}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if fixture.updates["stack-id"] != "new-id" {
		t.Fatalf("expected stack-id re-pointed at new-id, got %q", fixture.updates["stack-id"])
	}
}

func TestRepairStacksLeavesHealthyStackAlone(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"stack-id","primaryAssetId":"primary-id","assets":[{"id":"primary-id"},{"id":"sibling-id"}]}]`,
	}
	if code := runRepair(t, fixture, &model.Config{Yes: true}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(fixture.updates) != 0 {
		t.Fatalf("expected no stack write, got %v", fixture.updates)
	}
}

func TestRepairStacksLeavesHiddenPrimaryAlone(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"stack-id","primaryAssetId":"hidden-id","assets":[{"id":"sibling-id"}]}]`,
		assets: map[string]string{"hidden-id": `{"id":"hidden-id","isTrashed":false,"visibility":"hidden"}`},
	}
	if code := runRepair(t, fixture, &model.Config{Yes: true}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(fixture.updates) != 0 {
		t.Fatalf("expected no stack write, got %v", fixture.updates)
	}
}

func TestRepairStacksSkipsStackWithNoLiveMember(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"stack-id","primaryAssetId":"old-id","assets":[]}]`,
		assets: map[string]string{"old-id": `{"id":"old-id","isTrashed":true}`},
	}
	if code := runRepair(t, fixture, &model.Config{Yes: true}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(fixture.updates) != 0 {
		t.Fatalf("expected no stack write, got %v", fixture.updates)
	}
}

func TestRepairStacksDryRunWritesNothing(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"stack-id","primaryAssetId":"old-id","assets":[{"id":"new-id","originalFileName":"a.jpg"}]}]`,
		assets: map[string]string{"old-id": `{"id":"old-id","isTrashed":true}`},
	}
	if code := runRepair(t, fixture, &model.Config{DryRun: true, Yes: true}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(fixture.updates) != 0 {
		t.Fatalf("dry run must not write, got %v", fixture.updates)
	}
}

func TestRepairStacksSkipsUnreadablePrimaryAndRepairsTheRest(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"locked-stack","primaryAssetId":"locked-id","assets":[{"id":"sibling-id"}]},` +
			`{"id":"stack-id","primaryAssetId":"old-id","assets":[{"id":"new-id","originalFileName":"a.jpg"}]}]`,
		assets: map[string]string{"old-id": `{"id":"old-id","isTrashed":true}`},
		denied: map[string]int{"locked-id": http.StatusBadRequest},
	}
	if code := runRepair(t, fixture, &model.Config{Yes: true}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if fixture.updates["stack-id"] != "new-id" {
		t.Fatalf("the readable broken stack must still be repaired, got %q", fixture.updates["stack-id"])
	}
	if _, written := fixture.updates["locked-stack"]; written {
		t.Fatal("the stack with an unreadable primary must not be written to")
	}
}

func TestConfirmRepairStacksAcceptsAnAffirmativeAnswer(t *testing.T) {
	var writer bytes.Buffer

	if !confirmRepairStacks(strings.NewReader("y\n"), &writer) {
		t.Fatal("expected y to confirm")
	}
	if !strings.Contains(writer.String(), "Repair these stacks? [y/N]: ") {
		t.Fatalf("unexpected prompt output: %q", writer.String())
	}
}

func TestConfirmRepairStacksRefusesAnythingElse(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", "later\n", ""} {
		var writer bytes.Buffer
		if confirmRepairStacks(strings.NewReader(answer), &writer) {
			t.Fatalf("expected %q to decline", answer)
		}
	}
}

func TestRepairStacksDeclinedAtThePromptWritesNothing(t *testing.T) {
	stdio, err := os.CreateTemp(t.TempDir(), "stdio")
	if err != nil {
		t.Fatalf("create temp stdio file: %v", err)
	}
	defer stdio.Close()
	if _, err := stdio.WriteString("n\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if _, err := stdio.Seek(0, 0); err != nil {
		t.Fatalf("rewind stdin: %v", err)
	}

	restoreStdin, restoreStdout := withFakeStdio(stdio, stdio)
	defer restoreStdin()
	defer restoreStdout()

	fixture := &stackFixture{
		stacks: `[{"id":"stack-id","primaryAssetId":"old-id","assets":[{"id":"new-id","originalFileName":"a.jpg"}]}]`,
		assets: map[string]string{"old-id": `{"id":"old-id","isTrashed":true}`},
	}
	if code := runRepair(t, fixture, &model.Config{}); code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if len(fixture.updates) != 0 {
		t.Fatalf("a declined prompt must not write, got %v", fixture.updates)
	}

	output, err := os.ReadFile(stdio.Name())
	if err != nil {
		t.Fatalf("read captured stdio: %v", err)
	}
	if !strings.Contains(string(output), "Aborted.") {
		t.Fatalf("expected the run to report the abort, got: %q", output)
	}
}

func TestRepairStacksReturnsOneWhenListingStacksFails(t *testing.T) {
	if code := runRepair(t, &stackFixture{}, &model.Config{Yes: true}); code != 1 {
		t.Fatalf("expected exit 1 when the stack listing fails, got %d", code)
	}
}

func TestRepairStacksReportsFailedWritesAndKeepsGoing(t *testing.T) {
	fixture := &stackFixture{
		stacks: `[{"id":"denied-stack","primaryAssetId":"old-a","assets":[{"id":"live-a","originalFileName":"a.jpg"}]},` +
			`{"id":"stack-id","primaryAssetId":"old-b","assets":[{"id":"live-b","originalFileName":"b.jpg"}]}]`,
		assets: map[string]string{
			"old-a": `{"id":"old-a","isTrashed":true}`,
			"old-b": `{"id":"old-b","isTrashed":true}`,
		},
		updateFails: map[string]int{"denied-stack": http.StatusForbidden},
	}
	if code := runRepair(t, fixture, &model.Config{Yes: true}); code != 1 {
		t.Fatalf("expected exit 1 when a repair write fails, got %d", code)
	}
	if fixture.updates["stack-id"] != "live-b" {
		t.Fatalf("the remaining stack must still be repaired, got %q", fixture.updates["stack-id"])
	}
}
