package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/model"
	"github.com/majorfi/immich-exif/process"
)

type teaProgram interface {
	Send(tea.Msg)
	Run() (tea.Model, error)
}

var newTeaProgram = func(m tea.Model) teaProgram {
	return tea.NewProgram(m)
}

type tuiModel struct {
	total             int
	completed         int
	current           model.ProgressEvent
	diff              *model.DiffEvent
	results           []model.ResultEvent
	done              bool
	allDone           model.AllDoneEvent
	width             int
	height            int
	waitingForConfirm bool
	awaitingNextDiff  bool
	responseChan      chan model.DiffAction
	cancelRequested   bool
	cancelFunc        func()
	autoConfirm       bool
}

func (m *tuiModel) Init() tea.Cmd {
	return nil
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		if m.waitingForConfirm {
			switch key {
			case "y", "enter":
				m.waitingForConfirm = false
				m.awaitingNextDiff = true
				m.current = model.ProgressEvent{}
				m.diff = nil
				m.responseChan <- model.ActionConfirm
			case "s", "n":
				m.waitingForConfirm = false
				m.awaitingNextDiff = true
				m.current = model.ProgressEvent{}
				m.diff = nil
				m.responseChan <- model.ActionSkip
			case "q", "escape":
				m.waitingForConfirm = false
				m.awaitingNextDiff = true
				m.current = model.ProgressEvent{Step: "Cancellation requested, finishing in-flight work..."}
				m.diff = nil
				m.cancelRequested = true
				if m.cancelFunc != nil {
					m.cancelFunc()
				}
				m.responseChan <- model.ActionQuit
			}
			return m, nil
		}

		if m.done {
			if key == "enter" || key == " " || key == "escape" || key == "q" || key == "ctrl+c" {
				return m, tea.Quit
			}
		}
		if key == "q" || key == "ctrl+c" {
			if !m.cancelRequested {
				m.cancelRequested = true
				m.current = model.ProgressEvent{Step: "Cancellation requested, finishing in-flight work..."}
				if m.cancelFunc != nil {
					m.cancelFunc()
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case model.ProgressEvent:
		if !m.awaitingNextDiff {
			m.current = msg
		}

	case model.DiffEvent:
		if m.cancelRequested {
			m.awaitingNextDiff = true
			m.current = model.ProgressEvent{Step: "Cancellation requested, finishing in-flight work..."}
			m.responseChan <- model.ActionQuit
			return m, nil
		}
		m.diff = &msg
		m.waitingForConfirm = true
		m.awaitingNextDiff = false

	case model.ResultEvent:
		m.results = append(m.results, msg)
		m.completed = len(m.results)
		m.current = model.ProgressEvent{}
		m.diff = nil

	case model.AllDoneEvent:
		m.done = true
		m.allDone = msg
	}

	return m, nil
}

func (m *tuiModel) View() tea.View {
	v := tea.NewView(tuiRender(m))
	v.AltScreen = true
	return v
}

func RunTUI(client *api.ImmichClient, uploader process.Uploader, cfg *model.Config, assetIDs []string) ([]model.ProcessResult, error) {
	if !cfg.Yes {
		cfg.Workers = 1
	}

	responseChan := make(chan model.DiffAction, 1)
	doneChan := make(chan struct{})
	resultChan := make(chan []model.ProcessResult, 1)

	m := &tuiModel{
		total:        len(assetIDs),
		responseChan: responseChan,
		autoConfirm:  cfg.Yes,
	}

	p := newTeaProgram(m)
	emitter := &TUIEmitter{program: p, responseChan: responseChan, doneChan: doneChan, autoConfirm: cfg.Yes}
	pool := process.NewWorkerPool(client, uploader, cfg, emitter)
	m.cancelFunc = pool.Cancel

	go func() {
		results := pool.Process(assetIDs)
		emitter.EmitAllDone(model.AllDoneEvent{Results: results})
		resultChan <- results
	}()

	finalModel, err := p.Run()
	close(doneChan)
	if err != nil {
		pool.Cancel()
		results := <-resultChan
		return results, fmt.Errorf("run TUI: %w", err)
	}
	processResults := <-resultChan

	final := finalModel.(*tuiModel)

	results := final.allDone.Results
	if len(results) == 0 {
		for _, re := range final.results {
			results = append(results, re.Result)
		}
	}
	if len(results) == 0 {
		results = processResults
	}

	logEmitter := &LogEmitter{}
	logEmitter.EmitAllDone(model.AllDoneEvent{Results: results})

	return results, nil
}
