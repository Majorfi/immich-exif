package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/model"
)

func TestTUIModelInitReturnsNilCommand(t *testing.T) {
	m := newTestTUIModel(1)
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("expected nil init command, got %#v", cmd)
	}
}

func TestTUIModelViewUsesAltScreen(t *testing.T) {
	m := newTestTUIModel(1)
	view := m.View()
	if !view.AltScreen {
		t.Fatal("expected TUI view to enable alt screen")
	}
	if view.Content == "" {
		t.Fatal("expected TUI view content to be non-empty")
	}
}

func TestTUIModelDiffSkipFlow(t *testing.T) {
	m := newTestTUIModel(1)
	m.Update(model.DiffEvent{AssetID: "asset-1", Filename: "photo.jpg"})

	m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.waitingForConfirm {
		t.Fatal("expected waitingForConfirm=false")
	}
	if !m.awaitingNextDiff {
		t.Fatal("expected awaitingNextDiff=true")
	}
	select {
	case action := <-m.responseChan:
		if action != model.ActionSkip {
			t.Fatalf("expected ActionSkip, got %d", action)
		}
	default:
		t.Fatal("expected skip action in response channel")
	}
}

func TestTUIModelDoneKeyTriggersQuit(t *testing.T) {
	m := newTestTUIModel(1)
	m.done = true

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestTUIModelWindowSizeMsg(t *testing.T) {
	m := newTestTUIModel(1)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 42})
	if m.width != 120 || m.height != 42 {
		t.Fatalf("expected size 120x42, got %dx%d", m.width, m.height)
	}
}
