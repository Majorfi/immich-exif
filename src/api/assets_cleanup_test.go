package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingReadCloser struct {
	readCount int
}

func (r *failingReadCloser) Read(p []byte) (int, error) {
	if r.readCount == 0 {
		r.readCount++
		return copy(p, "partial-data"), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func (r *failingReadCloser) Close() error {
	return nil
}

func TestDownloadAssetRemovesPartialFileOnCopyError(t *testing.T) {
	client := NewImmichClient("https://example.com", "test-key")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       &failingReadCloser{},
				Header:     make(http.Header),
			}, nil
		}),
	}

	destPath := filepath.Join(t.TempDir(), "downloaded.jpg")
	err := client.DownloadAsset("asset-123", destPath, "")
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected copy error, got %v", err)
	}
	if _, statErr := os.Stat(destPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected partial file to be removed, got stat err %v", statErr)
	}
}
