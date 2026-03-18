package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/majorfi/immich-exif/api"
	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

func withMockExiftool(readFn func(string) (exif.ExifTagMap, error), writeFn func(string, []string) error) func() {
	origRead := exif.ReadExifTagsFn
	origWrite := exif.WriteExifTagsFn
	exif.ReadExifTagsFn = readFn
	exif.WriteExifTagsFn = writeFn
	return func() {
		exif.ReadExifTagsFn = origRead
		exif.WriteExifTagsFn = origWrite
	}
}

func assetServerWithExif() *httptest.Server {
	desc := "Test Description"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/original") {
			w.Write([]byte("fake-image-data"))
			return
		}
		asset := model.AssetResponse{
			ID:               "asset-1",
			OriginalFileName: "photo.jpg",
			ExifInfo:         &model.ExifInfo{Description: &desc},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asset)
	}))
}

type fakeTeaProgram struct {
	model    *tuiModel
	doneChan chan struct{}
	doneOnce sync.Once
}

func newFakeTeaProgram(m *tuiModel) *fakeTeaProgram {
	return &fakeTeaProgram{
		model:    m,
		doneChan: make(chan struct{}),
	}
}

func (p *fakeTeaProgram) Send(msg tea.Msg) {
	switch message := msg.(type) {
	case model.ResultEvent:
		p.model.results = append(p.model.results, message)
	case model.AllDoneEvent:
		p.model.done = true
		p.model.allDone = message
		p.doneOnce.Do(func() {
			close(p.doneChan)
		})
	}
}

func (p *fakeTeaProgram) Run() (tea.Model, error) {
	<-p.doneChan
	return p.model, nil
}

func TestRunTUIAutoConfirmHappyPath(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	originalNewTeaProgram := newTeaProgram
	var capturedProgram *fakeTeaProgram
	newTeaProgram = func(m tea.Model) teaProgram {
		tuiM, ok := m.(*tuiModel)
		if !ok {
			t.Fatalf("expected *tuiModel, got %T", m)
		}
		capturedProgram = newFakeTeaProgram(tuiM)
		return capturedProgram
	}
	t.Cleanup(func() {
		newTeaProgram = originalNewTeaProgram
	})

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{
		Yes:     true,
		Workers: 1,
		DryRun:  true,
	}

	results, err := RunTUI(client, nil, cfg, []string{"asset-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d (%v)", len(results), results)
	}

	result := results[0]
	if result.AssetID != "asset-1" {
		t.Fatalf("expected asset ID asset-1, got %q", result.AssetID)
	}
	if result.Status != model.StatusSuccess {
		t.Fatalf("expected success status, got %s", result.Status)
	}
	if !strings.Contains(result.Message, "dry-run") {
		t.Fatalf("expected dry-run message, got %q", result.Message)
	}
	if capturedProgram == nil {
		t.Fatal("expected fake TUI program to be used")
	}
	if !capturedProgram.model.done {
		t.Fatal("expected TUI model to be marked done")
	}
}

type fallbackTeaProgram struct {
	model    *tuiModel
	doneChan chan struct{}
	doneOnce sync.Once
}

func newFallbackTeaProgram(m *tuiModel) *fallbackTeaProgram {
	return &fallbackTeaProgram{
		model:    m,
		doneChan: make(chan struct{}),
	}
}

func (p *fallbackTeaProgram) Send(msg tea.Msg) {
	switch message := msg.(type) {
	case model.ResultEvent:
		p.model.results = append(p.model.results, message)
	case model.AllDoneEvent:
		p.model.done = true
		p.model.allDone = model.AllDoneEvent{}
		p.doneOnce.Do(func() {
			close(p.doneChan)
		})
	}
}

func (p *fallbackTeaProgram) Run() (tea.Model, error) {
	<-p.doneChan
	return p.model, nil
}

func TestRunTUIFallsBackToResultEventsWhenAllDoneIsEmpty(t *testing.T) {
	server := assetServerWithExif()
	defer server.Close()

	defer withMockExiftool(
		func(string) (exif.ExifTagMap, error) { return exif.ExifTagMap{}, nil },
		func(string, []string) error { return nil },
	)()

	originalNewTeaProgram := newTeaProgram
	var capturedProgram *fallbackTeaProgram
	newTeaProgram = func(m tea.Model) teaProgram {
		tuiM, ok := m.(*tuiModel)
		if !ok {
			t.Fatalf("expected *tuiModel, got %T", m)
		}
		capturedProgram = newFallbackTeaProgram(tuiM)
		return capturedProgram
	}
	t.Cleanup(func() {
		newTeaProgram = originalNewTeaProgram
	})

	client := api.NewImmichClient(server.URL, "key")
	cfg := &model.Config{
		Yes:     true,
		Workers: 1,
		DryRun:  true,
	}

	results, err := RunTUI(client, nil, cfg, []string{"asset-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d (%v)", len(results), results)
	}
	if results[0].Status != model.StatusSuccess {
		t.Fatalf("expected success status, got %s", results[0].Status)
	}
	if !strings.Contains(results[0].Message, "dry-run") {
		t.Fatalf("expected dry-run message, got %q", results[0].Message)
	}
	if capturedProgram == nil {
		t.Fatal("expected fallback TUI program to be used")
	}
	if len(capturedProgram.model.allDone.Results) != 0 {
		t.Fatalf("expected empty allDone results in fallback program, got %v", capturedProgram.model.allDone.Results)
	}
	if len(capturedProgram.model.results) == 0 {
		t.Fatal("expected fallback program to capture result events")
	}
}
