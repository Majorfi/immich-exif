package process

import (
	"errors"
	"fmt"
	"testing"
)

func TestNonRetryableErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("boom")
	wrapped := nonRetryable(inner)
	if !errors.Is(wrapped, inner) {
		t.Fatal("expected errors.Is to unwrap to the inner error")
	}
	if wrapped.Error() != "boom" {
		t.Fatalf("expected message 'boom', got %q", wrapped.Error())
	}
}

func TestFileChecksumSHA1MissingFile(t *testing.T) {
	if _, err := fileChecksumSHA1("/nonexistent/path/does-not-exist"); err == nil {
		t.Fatal("expected error for a missing file")
	}
}
