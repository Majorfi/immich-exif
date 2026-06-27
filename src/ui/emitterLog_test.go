package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestShortID(t *testing.T) {
	testCases := []struct {
		name string
		id   string
		want string
	}{
		{name: "long id truncated", id: "abcdefghij", want: "abcdefgh"},
		{name: "exactly 8", id: "abcdefgh", want: "abcdefgh"},
		{name: "short id unchanged", id: "abc", want: "abc"},
		{name: "empty", id: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := model.ShortID(tc.id)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTruncateFilename(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "short name unchanged", input: "photo.jpg", maxLen: 20, want: "photo.jpg"},
		{name: "exact length unchanged", input: "photo.jpg", maxLen: 9, want: "photo.jpg"},
		{name: "long name truncated with ellipsis", input: "very-long-filename.jpg", maxLen: 15, want: "...filename.jpg"},
		{name: "single char max", input: "abcdef", maxLen: 4, want: "...f"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := model.TruncateFilename(tc.input, tc.maxLen)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEmitDiffAutoConfirmReturnsConfirm(t *testing.T) {
	emitter := &LogEmitter{AutoConfirm: true}
	action := emitter.EmitDiff(model.DiffEvent{
		AssetID:  "asset-1",
		Filename: "photo.jpg",
		Index:    1,
		Total:    5,
		Entries: []model.DiffEntry{
			{Tag: "Make", Symbol: model.DiffAdd, Old: "(none)", New: "Canon"},
		},
	})
	if action != model.ActionConfirm {
		t.Fatalf("expected ActionConfirm, got %d", action)
	}
}

func TestEmitDiffEmptyEntriesReturnsConfirm(t *testing.T) {
	emitter := &LogEmitter{AutoConfirm: false}
	action := emitter.EmitDiff(model.DiffEvent{
		Entries: nil,
	})
	if action != model.ActionConfirm {
		t.Fatalf("expected ActionConfirm for empty entries, got %d", action)
	}
}

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestEmitProgressPrintsStep(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitProgress(model.ProgressEvent{
			AssetID:  "abcdefghij",
			Filename: "photo.jpg",
			Step:     "Downloading...",
		})
	})
	if !strings.Contains(output, "abcdefgh") {
		t.Fatalf("expected short asset ID in output: %s", output)
	}
	if !strings.Contains(output, "photo.jpg") {
		t.Fatalf("expected filename in output: %s", output)
	}
	if !strings.Contains(output, "Downloading...") {
		t.Fatalf("expected step in output: %s", output)
	}
}

func TestEmitProgressWithoutFilename(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitProgress(model.ProgressEvent{
			AssetID: "abcdefghij",
			Step:    "processing",
		})
	})
	if strings.Contains(output, "abcdefgh |") {
		t.Fatalf("should not print asset header without filename: %s", output)
	}
	if !strings.Contains(output, "processing") {
		t.Fatalf("expected step in output: %s", output)
	}
}

func TestEmitProgressGroupsConsecutiveStepsForSameAsset(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitProgress(model.ProgressEvent{
			AssetID:  "abcdefghij",
			Filename: "photo.jpg",
			Step:     "Uploading new asset...",
		})
		emitter.EmitProgress(model.ProgressEvent{
			AssetID:  "abcdefghij",
			Filename: "photo.jpg",
			Step:     "Upload status: duplicate (asset ID: duplicate-123)",
		})
		emitter.EmitProgress(model.ProgressEvent{
			AssetID:  "abcdefghij",
			Filename: "photo.jpg",
			Step:     "Upload rejected. Reason: duplicate (duplicate asset ID: duplicate-123)",
		})
	})

	if strings.Count(output, "=>") != 1 {
		t.Fatalf("expected a single grouped header, got output: %s", output)
	}
	if !strings.Contains(output, "Uploading new asset...") {
		t.Fatalf("expected first step, got output: %s", output)
	}
	if !strings.Contains(output, "Upload rejected. Reason: duplicate") {
		t.Fatalf("expected duplicate rejection step, got output: %s", output)
	}
}

func TestEmitAllDonePrintsSummary(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitAllDone(model.AllDoneEvent{
			Results: []model.ProcessResult{
				{Status: model.StatusSuccess},
				{Status: model.StatusSuccess},
				{Status: model.StatusSkipped},
				{Status: model.StatusFailed, AssetID: "fail-id-123", Message: "download error"},
			},
		})
	})
	if !strings.Contains(output, "2 succeeded") {
		t.Fatalf("expected 2 succeeded in output: %s", output)
	}
	if !strings.Contains(output, "1 skipped") {
		t.Fatalf("expected 1 skipped in output: %s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Fatalf("expected 1 failed in output: %s", output)
	}
	if !strings.Contains(output, "Failed assets") {
		t.Fatalf("expected failed assets section: %s", output)
	}
	if !strings.Contains(output, "download error") {
		t.Fatalf("expected failure message in output: %s", output)
	}
}

func TestEmitResultDoesNotPanic(t *testing.T) {
	emitter := &LogEmitter{}
	emitter.EmitResult(model.ResultEvent{
		Index: 1,
		Total: 5,
		Result: model.ProcessResult{
			AssetID: "test-id",
			Status:  model.StatusSuccess,
			Message: "ok",
		},
	})
}

func TestDecodeKeyMapsExplicitKeys(t *testing.T) {
	cases := []struct {
		b    byte
		want model.DiffAction
	}{
		{'y', model.ActionConfirm},
		{'Y', model.ActionConfirm},
		{'\r', model.ActionConfirm},
		{'\n', model.ActionConfirm},
		{'s', model.ActionSkip},
		{'S', model.ActionSkip},
		{'n', model.ActionSkip},
		{'q', model.ActionQuit},
		{'Q', model.ActionQuit},
		{3, model.ActionQuit},
		{4, model.ActionQuit},
	}

	for _, tc := range cases {
		action, ok := decodeKey(tc.b)
		if !ok {
			t.Fatalf("byte %q should be recognized", rune(tc.b))
		}
		if action != tc.want {
			t.Fatalf("byte %q: expected action %d, got %d", rune(tc.b), tc.want, action)
		}
	}
}

func TestDecodeKeyIgnoresEscapeSequenceBytes(t *testing.T) {
	// Arrow/Home/End/F-keys arrive as multi-byte sequences led by ESC (27).
	// None of those bytes must be read as quit and silently cancel the batch.
	for _, b := range []byte{27, '[', 'A', 'B', 'C', 'D', 'H', 'F', 'O'} {
		if action, ok := decodeKey(b); ok {
			t.Fatalf("byte %d should be ignored, got recognized action %d", b, action)
		}
	}
}

func TestEmitDiffInteractiveReturnsQuitOnNonTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	emitter := &LogEmitter{AutoConfirm: false}
	captureStdout(func() {
		action := emitter.EmitDiff(model.DiffEvent{
			Entries:  []model.DiffEntry{{Tag: "Make", Symbol: model.DiffAdd, Old: "(none)", New: "Canon"}},
			Index:    1,
			Total:    1,
			Filename: "photo.jpg",
		})
		if action != model.ActionQuit {
			t.Fatalf("expected ActionQuit when stdin is not a terminal, got %d", action)
		}
	})
}

func TestEmitAllDoneNoFailedSection(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitAllDone(model.AllDoneEvent{
			Results: []model.ProcessResult{
				{Status: model.StatusSuccess},
			},
		})
	})
	if strings.Contains(output, "Failed assets") {
		t.Fatalf("should not print failed assets section: %s", output)
	}
}
