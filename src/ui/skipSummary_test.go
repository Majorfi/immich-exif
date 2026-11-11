package ui

import (
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func skipped(id, message string) model.ProcessResult {
	return model.ProcessResult{AssetID: id, Status: model.StatusSkipped, Message: message}
}

func TestSkippedSummaryGroupsByReasonMostFrequentFirst(t *testing.T) {
	results := []model.ProcessResult{
		skipped("aaaaaaaa-1", "re-run shortly"),
		{AssetID: "ok", Status: model.StatusSuccess},
		skipped("bbbbbbbb-1", "metadata already matches"),
		skipped("cccccccc-1", "metadata already matches"),
		{AssetID: "dddddddd-1", Status: model.StatusSkipped, Message: "upload status=duplicate", DuplicateID: "x"},
		{AssetID: "eeeeeeee-1", Status: model.StatusFailed, Message: "boom"},
	}
	lines := skippedSummaryLines(results)
	want := []string{
		"2 × metadata already matches (bbbbbbbb, cccccccc)",
		"1 × re-run shortly (aaaaaaaa)",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestSkippedSummaryCapsListedIDs(t *testing.T) {
	var results []model.ProcessResult
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7"} {
		results = append(results, skipped(id, "metadata already matches"))
	}
	lines := skippedSummaryLines(results)
	if len(lines) != 1 || !strings.HasSuffix(lines[0], "(a1, a2, a3, a4, a5 +2 more)") || !strings.HasPrefix(lines[0], "7 × ") {
		t.Fatalf("unexpected line: %v", lines)
	}
}

func TestEmitAllDoneListsSkipReasons(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitAllDone(model.AllDoneEvent{Results: []model.ProcessResult{
			skipped("asset-123", "asset changed less than a minute ago; re-run shortly"),
		}})
	})
	if !strings.Contains(output, "Skipped assets:") || !strings.Contains(output, "1 × asset changed less than a minute ago; re-run shortly (asset-12") {
		t.Fatalf("expected the skip reason in the summary: %s", output)
	}
}

func TestEmitAllDoneOmitsSkippedSectionWithoutSkips(t *testing.T) {
	emitter := &LogEmitter{}
	output := captureStdout(func() {
		emitter.EmitAllDone(model.AllDoneEvent{Results: []model.ProcessResult{{Status: model.StatusSuccess}}})
	})
	if strings.Contains(output, "Skipped assets") {
		t.Fatalf("no skipped section expected: %s", output)
	}
}
