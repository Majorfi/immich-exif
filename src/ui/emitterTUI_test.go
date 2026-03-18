package ui

import (
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/model"
)

type testTeaModel struct {
	received chan tea.Msg
}

func (m testTeaModel) Init() tea.Cmd {
	return nil
}

func (m testTeaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	select {
	case m.received <- msg:
	default:
	}
	return m, nil
}

func (m testTeaModel) View() tea.View {
	return tea.NewView("")
}

func TestTUIEmitterEmitDiffAutoConfirm(t *testing.T) {
	emitter := &TUIEmitter{
		autoConfirm:  true,
		responseChan: make(chan model.DiffAction, 1),
	}

	action := emitter.EmitDiff(model.DiffEvent{AssetID: "asset-1"})
	if action != model.ActionConfirm {
		t.Fatalf("expected confirm action, got %d", action)
	}
}

func TestTUIEmitterEmitDiffReturnsQuitWhenDone(t *testing.T) {
	doneChan := make(chan struct{})
	close(doneChan)

	emitter := &TUIEmitter{
		program:      noopTeaProgram{},
		responseChan: make(chan model.DiffAction),
		doneChan:     doneChan,
	}

	action := emitter.EmitDiff(model.DiffEvent{AssetID: "asset-1"})
	if action != model.ActionQuit {
		t.Fatalf("expected quit action, got %d", action)
	}
}

func TestTUIEmitterSendsEventsToProgram(t *testing.T) {
	received := make(chan tea.Msg, 8)
	m := testTeaModel{received: received}
	program := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(io.Discard))

	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()
	defer func() {
		program.Quit()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("unexpected tea program error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for tea program to stop")
		}
	}()

	responseChan := make(chan model.DiffAction, 1)
	emitter := &TUIEmitter{
		program:      program,
		responseChan: responseChan,
		autoConfirm:  false,
	}

	emitter.EmitProgress(model.ProgressEvent{AssetID: "asset-1", Step: "step-1"})
	responseChan <- model.ActionSkip
	action := emitter.EmitDiff(model.DiffEvent{AssetID: "asset-1"})
	emitter.EmitResult(model.ResultEvent{Result: model.ProcessResult{AssetID: "asset-1", Status: model.StatusSuccess}})
	emitter.EmitAllDone(model.AllDoneEvent{Results: []model.ProcessResult{{AssetID: "asset-1", Status: model.StatusSuccess}}})

	if action != model.ActionSkip {
		t.Fatalf("expected skip action, got %d", action)
	}

	assertReceivedMessageType(t, received, model.ProgressEvent{})
	assertReceivedMessageType(t, received, model.DiffEvent{})
	assertReceivedMessageType(t, received, model.ResultEvent{})
	assertReceivedMessageType(t, received, model.AllDoneEvent{})
}

func assertReceivedMessageType(t *testing.T, received chan tea.Msg, expected tea.Msg) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case msg := <-received:
			if messageMatchesExpectedType(msg, expected) {
				return
			}
		case <-timeout:
			t.Fatalf("timeout waiting for message type %T", expected)
		}
	}
}

func messageMatchesExpectedType(msg tea.Msg, expected tea.Msg) bool {
	switch expected.(type) {
	case model.ProgressEvent:
		_, ok := msg.(model.ProgressEvent)
		return ok
	case model.DiffEvent:
		_, ok := msg.(model.DiffEvent)
		return ok
	case model.ResultEvent:
		_, ok := msg.(model.ResultEvent)
		return ok
	case model.AllDoneEvent:
		_, ok := msg.(model.AllDoneEvent)
		return ok
	default:
		return false
	}
}

type noopTeaProgram struct{}

func (noopTeaProgram) Send(tea.Msg) {}

func (noopTeaProgram) Run() (tea.Model, error) {
	return testTeaModel{}, nil
}
