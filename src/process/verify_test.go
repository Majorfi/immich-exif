package process

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
)

func TestDecodeChecksum(t *testing.T) {
	raw := sha1.Sum([]byte("hello"))
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "base64 sha1", input: base64.StdEncoding.EncodeToString(raw[:])},
		{name: "hex sha1", input: hex.EncodeToString(raw[:])},
		{name: "empty", input: "", wantErr: true},
		{name: "garbage", input: "not-a-checksum!!", wantErr: true},
		{name: "wrong length base64", input: base64.StdEncoding.EncodeToString([]byte("short")), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeChecksum(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != string(raw[:]) {
				t.Fatalf("decoded checksum mismatch")
			}
		})
	}
}

func verifyUploadServer(t *testing.T, returnedChecksum string, deleteCalled *bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/assets":
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/assets/new-id":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(model.AssetResponse{ID: "new-id", Checksum: returnedChecksum})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/assets":
			*deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func TestModernUploaderVerifyUploadBlocksDeleteOnMismatch(t *testing.T) {
	wrong := sha1.Sum([]byte("totally-different-bytes"))
	deleteCalled := false
	server := verifyUploadServer(t, base64.StdEncoding.EncodeToString(wrong[:]), &deleteCalled)
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(filePath, []byte("the-real-uploaded-bytes"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), VerifyUpload: true}
	asset := &model.AssetResponse{ID: "old-id", OriginalFileName: "f.jpg", FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

	_, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err == nil {
		t.Fatal("expected verification failure")
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("expected verification error, got: %v", err)
	}
	if deleteCalled {
		t.Fatal("DELETE must NOT be called when checksum verification fails — the original must be preserved")
	}
}

func TestModernUploaderVerifyUploadBlocksDeleteOnGetError(t *testing.T) {
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/assets":
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/assets/new-id":
			http.Error(w, "transient failure", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/assets":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(filePath, []byte("bytes"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), VerifyUpload: true}
	asset := &model.AssetResponse{ID: "old-id", OriginalFileName: "f.jpg", FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

	if _, err := uploader.Upload(filePath, asset, &noopEmitter{}); err == nil {
		t.Fatal("expected error when the verification GET fails")
	}
	if deleteCalled {
		t.Fatal("DELETE must NOT be called when the verification GET fails — original must be preserved")
	}
}

func TestModernUploaderVerifyUploadProceedsOnMatch(t *testing.T) {
	content := []byte("the-real-uploaded-bytes")
	sum := sha1.Sum(content)
	deleteCalled := false
	server := verifyUploadServer(t, base64.StdEncoding.EncodeToString(sum[:]), &deleteCalled)
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "f.jpg")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), VerifyUpload: true}
	asset := &model.AssetResponse{ID: "old-id", OriginalFileName: "f.jpg", FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

	outcome, err := uploader.Upload(filePath, asset, &noopEmitter{})
	if err != nil {
		t.Fatalf("expected success on matching checksum, got: %v", err)
	}
	if outcome.NewID != "new-id" {
		t.Fatalf("expected new-id, got %s", outcome.NewID)
	}
	if !deleteCalled {
		t.Fatal("expected DELETE of the original after a verified upload")
	}
}

func TestModernUploaderDeleteForceFollowsVerification(t *testing.T) {
	content := []byte("the-real-uploaded-bytes")
	sum := sha1.Sum(content)

	cases := []struct {
		name      string
		verify    bool
		wantForce bool
	}{
		{name: "verified upload deletes permanently", verify: true, wantForce: true},
		{name: "unverified upload moves to trash", verify: false, wantForce: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotForce bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/assets":
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"id":"new-id","status":"created"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/api/assets/new-id":
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(model.AssetResponse{ID: "new-id", Checksum: base64.StdEncoding.EncodeToString(sum[:])})
				case r.Method == http.MethodDelete && r.URL.Path == "/api/assets":
					var payload model.DeleteAssetsRequest
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Fatalf("decode delete payload: %v", err)
					}
					gotForce = payload.Force
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()

			filePath := filepath.Join(t.TempDir(), "f.jpg")
			if err := os.WriteFile(filePath, content, 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			uploader := &ModernUploader{Client: api.NewImmichClient(server.URL, "key"), VerifyUpload: tc.verify}
			asset := &model.AssetResponse{ID: "old-id", OriginalFileName: "f.jpg", FileCreatedAt: time.Now(), FileModifiedAt: time.Now()}

			if _, err := uploader.Upload(filePath, asset, &noopEmitter{}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotForce != tc.wantForce {
				t.Fatalf("expected delete force=%v, got %v", tc.wantForce, gotForce)
			}
		})
	}
}
