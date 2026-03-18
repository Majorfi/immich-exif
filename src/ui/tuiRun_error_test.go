package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/model"
)

type failingTeaProgram struct {
	model tea.Model
	err   error
}

func (p *failingTeaProgram) Send(tea.Msg) {}

func (p *failingTeaProgram) Run() (tea.Model, error) {
	return p.model, p.err
}

func TestRunTUIReturnsProgramError(t *testing.T) {
	originalNewTeaProgram := newTeaProgram
	newTeaProgram = func(m tea.Model) teaProgram {
		return &failingTeaProgram{
			model: m,
			err:   errors.New("boom"),
		}
	}
	t.Cleanup(func() {
		newTeaProgram = originalNewTeaProgram
	})

	results, err := RunTUI(nil, nil, &model.Config{Yes: true, Workers: 1}, nil)
	if err == nil {
		t.Fatal("expected TUI error")
	}
	if err.Error() != "run TUI: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %v", results)
	}
}
