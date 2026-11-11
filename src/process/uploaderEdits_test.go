package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

const cropEditsJSON = `{"assetId":"old-id","edits":[{"id":"e1","action":"crop","parameters":{"x":0,"y":0,"width":10,"height":20}},{"id":"e2","action":"rotate","parameters":{"angle":90}}]}`

func editedAsset() *model.AssetResponse {
	return &model.AssetResponse{
		ID:               "old-id",
		OriginalFileName: "asset.jpg",
		FileCreatedAt:    time.Now().UTC(),
		FileModifiedAt:   time.Now().UTC(),
		IsEdited:         true,
	}
}

type editsServer struct {
	calls        []string
	orientation  string
	copySidecar  *bool
	targetEdits  string
	putStatus    int
	readyAfter   int
	dimsRequests int
	uploadStatus string
	targetOrient string
}

func (s *editsServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		call := r.Method + " " + r.URL.Path
		s.calls = append(s.calls, call)
		w.Header().Set("Content-Type", "application/json")
		switch call {
		case "POST /api/assets":
			status := s.uploadStatus
			if status == "" {
				status = "created"
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"` + status + `"}`))
		case "PUT /api/assets/copy":
			var body model.CopyAssetsRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.copySidecar = &body.Sidecar
			w.WriteHeader(http.StatusNoContent)
		case "DELETE /api/assets":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/assets/old-id":
			_, _ = w.Write([]byte(`{"id":"old-id","exifInfo":{"exifImageWidth":40,"exifImageHeight":30,"orientation":"` + s.orientation + `"}}`))
		case "GET /api/assets/old-id/edits":
			_, _ = w.Write([]byte(cropEditsJSON))
		case "GET /api/assets/new-id/edits":
			edits := s.targetEdits
			if edits == "" {
				edits = `{"assetId":"new-id","edits":[]}`
			}
			_, _ = w.Write([]byte(edits))
		case "GET /api/assets/new-id":
			s.dimsRequests++
			if s.dimsRequests <= s.readyAfter {
				_, _ = w.Write([]byte(`{"id":"new-id","exifInfo":{"exifImageWidth":null,"exifImageHeight":null}}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"new-id","isEdited":` + boolJSON(s.targetEdits != "") + `,"exifInfo":{"exifImageWidth":40,"exifImageHeight":30,"orientation":"` + s.targetOrient + `"}}`))
		case "PUT /api/assets/new-id/edits":
			if s.putStatus != 0 {
				w.WriteHeader(s.putStatus)
				_, _ = w.Write([]byte(`{"message":"boom"}`))
				return
			}
			_, _ = w.Write([]byte(strings.Replace(cropEditsJSON, "old-id", "new-id", 1)))
		default:
			t.Fatalf("unexpected request: %s", call)
		}
	}
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func TestModernUploaderCarriesEditsBeforeTrashing(t *testing.T) {
	withStubbedSleep(t)
	state := &editsServer{readyAfter: 1}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key")}
	if _, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCalls(t, state.calls, []string{
		"GET /api/assets/old-id",
		"GET /api/assets/old-id/edits",
		"POST /api/assets",
		"GET /api/assets/new-id",
		"GET /api/assets/new-id",
		"GET /api/assets/new-id/edits",
		"PUT /api/assets/new-id/edits",
		"PUT /api/assets/copy",
		"DELETE /api/assets",
	})
	if state.copySidecar == nil || !*state.copySidecar {
		t.Fatal("a carried replacement keeps the original's sidecar")
	}
}

func TestModernUploaderKeepsOriginalWhenCarryingEditsFails(t *testing.T) {
	withStubbedSleep(t)
	state := &editsServer{putStatus: http.StatusBadRequest}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key")}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "NOT deleted") {
		t.Fatalf("expected a kept-original error, got %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/copy")
	assertNoCall(t, state.calls, "DELETE /api/assets")
}

func TestModernUploaderReadsEditsBeforeUploading(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/api/assets/old-id" {
			_, _ = w.Write([]byte(`{"id":"old-id","exifInfo":{"orientation":"1"}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Missing required permission: asset.edit.get"}`))
	}))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key")}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "asset.edit.get") {
		t.Fatalf("expected a permission hint, got %v", err)
	}
	assertCalls(t, calls, []string{"GET /api/assets/old-id", "GET /api/assets/old-id/edits"})
}

func TestModernUploaderGivesUpWaitingForDimensions(t *testing.T) {
	withStubbedSleep(t)
	orig := dimensionsWaitAttempts
	dimensionsWaitAttempts = 3
	t.Cleanup(func() { dimensionsWaitAttempts = orig })
	state := &editsServer{readyAfter: 100}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key")}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "after 3 checks") {
		t.Fatalf("expected a dimensions timeout, got %v", err)
	}
	if state.dimsRequests != 3 {
		t.Fatalf("expected 3 dimension checks, got %d", state.dimsRequests)
	}
	assertNoCall(t, state.calls, "DELETE /api/assets")
}

func TestModernUploaderRefusesToOverwriteDuplicateEdits(t *testing.T) {
	withStubbedSleep(t)
	state := &editsServer{uploadStatus: "duplicate", targetEdits: `{"assetId":"new-id","edits":[{"id":"x","action":"rotate","parameters":{"angle":180}}]}`}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), ResolveDuplicate: true}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "different Immich edits") {
		t.Fatalf("expected a refusal, got %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/new-id/edits")
	assertNoCall(t, state.calls, "DELETE /api/assets")
}

func TestModernUploaderAcceptsDuplicateWithSameEdits(t *testing.T) {
	withStubbedSleep(t)
	state := &editsServer{uploadStatus: "duplicate", targetEdits: strings.Replace(cropEditsJSON, "old-id", "new-id", 1)}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), ResolveDuplicate: true}
	if _, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/new-id/edits")
}

func TestModernUploaderDoesNotCarryBakedEdits(t *testing.T) {
	state := &editsServer{}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), ApplyEdits: true}
	if _, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCalls(t, state.calls, []string{"POST /api/assets", "PUT /api/assets/copy", "DELETE /api/assets"})
	assertNoCall(t, state.calls, "GET /api/assets/old-id/edits")
	if state.copySidecar == nil || *state.copySidecar {
		t.Fatal("a baked replacement must not inherit the original's sidecar")
	}
}

func TestModernUploaderRefusesBakedDuplicateThatIsEdited(t *testing.T) {
	state := &editsServer{uploadStatus: "duplicate", targetEdits: cropEditsJSON}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), ResolveDuplicate: true, ApplyEdits: true}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "carries Immich edits") {
		t.Fatalf("expected a refusal, got %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/copy")
	assertNoCall(t, state.calls, "DELETE /api/assets")
}

func assertNoCall(t *testing.T, calls []string, forbidden string) {
	t.Helper()
	for _, call := range calls {
		if call == forbidden {
			t.Fatalf("unexpected %s in %v", forbidden, calls)
		}
	}
}

func TestModernUploaderRefusesEditedDuplicateOfUneditedAsset(t *testing.T) {
	state := &editsServer{uploadStatus: "duplicate", targetEdits: cropEditsJSON}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	asset := editedAsset()
	asset.IsEdited = false
	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), ResolveDuplicate: true}
	_, err := uploader.Upload(writeUploaderFixture(t), asset, &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "carries Immich edits") {
		t.Fatalf("expected a refusal, got %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/copy")
	assertNoCall(t, state.calls, "DELETE /api/assets")
}

func TestModernUploaderRefusesCarryWhenSidecarSetsOrientation(t *testing.T) {
	withStubbedSleep(t)
	state := &editsServer{orientation: "6", targetOrient: "1"}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key")}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "resolves orientation 1 where the original resolved 6") {
		t.Fatalf("expected an orientation refusal, got %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/new-id/edits")
	assertNoCall(t, state.calls, "PUT /api/assets/copy")
	assertNoCall(t, state.calls, "DELETE /api/assets")
}

func TestModernUploaderChecksOrientationBeforeAcceptingMatchingEdits(t *testing.T) {
	withStubbedSleep(t)
	state := &editsServer{uploadStatus: "duplicate", targetEdits: strings.Replace(cropEditsJSON, "old-id", "new-id", 1), targetOrient: "6"}
	server := httptest.NewServer(state.handler(t))
	defer server.Close()

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), ResolveDuplicate: true}
	_, err := uploader.Upload(writeUploaderFixture(t), editedAsset(), &noopEmitter{})
	if err == nil || !strings.Contains(err.Error(), "resolves orientation 6") {
		t.Fatalf("expected an orientation refusal, got %v", err)
	}
	assertNoCall(t, state.calls, "PUT /api/assets/copy")
	assertNoCall(t, state.calls, "DELETE /api/assets")
}
