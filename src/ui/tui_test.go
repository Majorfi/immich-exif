package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/model"
)

func newTestTUIModel(total int) *tuiModel {
	return &tuiModel{
		total:        total,
		responseChan: make(chan model.DiffAction, 4),
	}
}

func TestTUIModelResultEventTracksCompletedCount(t *testing.T) {
	m := newTestTUIModel(3)

	m.Update(model.ResultEvent{Index: 3, Total: 3, Result: model.ProcessResult{AssetID: "a", Status: model.StatusSuccess}})
	if m.completed != 1 {
		t.Fatalf("expected completed=1 after first result, got %d", m.completed)
	}

	m.Update(model.ResultEvent{Index: 1, Total: 3, Result: model.ProcessResult{AssetID: "b", Status: model.StatusSkipped}})
	if m.completed != 2 {
		t.Fatalf("expected completed=2 after second result, got %d", m.completed)
	}
}

func TestTUIModelDiffConfirmFlow(t *testing.T) {
	m := newTestTUIModel(1)

	m.Update(model.DiffEvent{
		AssetID:  "asset-1",
		Filename: "photo.jpg",
		Entries:  []model.DiffEntry{{Tag: "Make", Symbol: model.DiffAdd, Old: "(none)", New: "Canon"}},
	})
	if !m.waitingForConfirm {
		t.Fatal("expected waitingForConfirm=true")
	}

	m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.waitingForConfirm {
		t.Fatal("expected waitingForConfirm=false")
	}
	if !m.awaitingNextDiff {
		t.Fatal("expected awaitingNextDiff=true")
	}
	select {
	case action := <-m.responseChan:
		if action != model.ActionConfirm {
			t.Fatalf("expected ActionConfirm, got %d", action)
		}
	default:
		t.Fatal("expected confirm action in response channel")
	}
}

func TestTUIModelCancelFromDiffSendsQuit(t *testing.T) {
	m := newTestTUIModel(1)
	cancelCalled := false
	m.cancelFunc = func() {
		cancelCalled = true
	}

	m.Update(model.DiffEvent{
		AssetID:  "asset-1",
		Filename: "photo.jpg",
		Entries:  []model.DiffEntry{{Tag: "Make", Symbol: model.DiffAdd, Old: "(none)", New: "Canon"}},
	})
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	if !m.cancelRequested {
		t.Fatal("expected cancelRequested=true")
	}
	if !cancelCalled {
		t.Fatal("expected cancelFunc to be called")
	}
	select {
	case action := <-m.responseChan:
		if action != model.ActionQuit {
			t.Fatalf("expected ActionQuit, got %d", action)
		}
	default:
		t.Fatal("expected quit action in response channel")
	}
}

func TestTUIModelCancelledBeforeDiffAutoQuitsDiff(t *testing.T) {
	m := newTestTUIModel(1)

	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !m.cancelRequested {
		t.Fatal("expected cancelRequested=true")
	}

	m.Update(model.DiffEvent{
		AssetID:  "asset-1",
		Filename: "photo.jpg",
		Entries:  []model.DiffEntry{{Tag: "Make", Symbol: model.DiffAdd, Old: "(none)", New: "Canon"}},
	})

	if !m.awaitingNextDiff {
		t.Fatal("expected awaitingNextDiff=true when already cancelled")
	}
	select {
	case action := <-m.responseChan:
		if action != model.ActionQuit {
			t.Fatalf("expected ActionQuit, got %d", action)
		}
	default:
		t.Fatal("expected quit action in response channel")
	}
}

func TestTUIRenderSmoke(t *testing.T) {
	m := &tuiModel{
		total:             2,
		completed:         1,
		width:             24,
		height:            8,
		waitingForConfirm: true,
		responseChan:      make(chan model.DiffAction, 1),
		current: model.ProgressEvent{
			AssetID:  "asset-1234567890",
			Filename: strings.Repeat("very-long-name-", 6) + ".jpg",
			Step:     "Processing",
		},
		diff: &model.DiffEvent{
			AssetID:  "asset-1",
			Filename: strings.Repeat("x", 120) + ".jpg",
			Entries: []model.DiffEntry{
				{Tag: "City", Symbol: model.DiffChange, Old: "Paris", New: "Lyon"},
			},
		},
	}

	out := tuiRender(m)
	if out == "" {
		t.Fatal("expected non-empty render output")
	}
	if !strings.Contains(out, "EXIF mismatch") {
		t.Fatalf("expected mismatch section in output, got: %s", out)
	}
}

func TestTUIRenderSummarySmoke(t *testing.T) {
	m := &tuiModel{
		total:        2,
		completed:    2,
		done:         true,
		width:        32,
		height:       10,
		responseChan: make(chan model.DiffAction, 1),
		allDone: model.AllDoneEvent{
			Results: []model.ProcessResult{
				{AssetID: "a", Status: model.StatusSuccess, Message: "uploaded"},
				{AssetID: "b", Status: model.StatusFailed, Message: "failed"},
			},
		},
	}

	out := tuiRender(m)
	if !strings.Contains(out, "Press Enter") {
		t.Fatalf("expected close hint in output, got: %s", out)
	}
	if !strings.Contains(out, "failed") {
		t.Fatalf("expected failure summary in output, got: %s", out)
	}
}
