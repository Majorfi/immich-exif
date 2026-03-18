package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/model"
)

type TUIEmitter struct {
	program      teaProgram
	responseChan <-chan model.DiffAction
	doneChan     <-chan struct{}
	autoConfirm  bool
}

func (e *TUIEmitter) EmitProgress(event model.ProgressEvent) {
	e.send(event)
}

func (e *TUIEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	if e.autoConfirm {
		return model.ActionConfirm
	}
	e.send(event)
	select {
	case action := <-e.responseChan:
		return action
	case <-e.doneChan:
		return model.ActionQuit
	}
}

func (e *TUIEmitter) EmitResult(event model.ResultEvent) {
	e.send(event)
}

func (e *TUIEmitter) EmitAllDone(event model.AllDoneEvent) {
	e.send(event)
}

func (e *TUIEmitter) send(msg tea.Msg) {
	select {
	case <-e.doneChan:
		return
	default:
		e.program.Send(msg)
	}
}
