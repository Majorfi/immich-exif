package ui

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"

	"github.com/majorfi/immich-exif/model"
)

type LogEmitter struct {
	AutoConfirm bool

	mu           sync.Mutex
	lastAssetID  string
	lastFilename string
}

func (e *LogEmitter) EmitProgress(event model.ProgressEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if event.Filename != "" && (event.AssetID != e.lastAssetID || event.Filename != e.lastFilename) {
		fmt.Printf("%s %s | %s\n", dim("=>"), model.ShortID(event.AssetID), model.TruncateFilename(event.Filename, 60))
		e.lastAssetID = event.AssetID
		e.lastFilename = event.Filename
	}
	fmt.Printf("   %s\n", event.Step)
}

func (e *LogEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	e.mu.Lock()
	defer e.mu.Unlock()

	if event.AssetID != "" {
		e.lastAssetID = event.AssetID
		e.lastFilename = event.Filename
	}

	if len(event.Entries) == 0 {
		return model.ActionConfirm
	}
	fmt.Printf("[%d/%d] %d EXIF mismatch found for %s:\n", event.Index, event.Total, len(event.Entries), model.TruncateFilename(event.Filename, 60))
	for _, d := range event.Entries {
		fmt.Printf("    %s %-22s %s %s %s\n", diffSymbol(string(d.Symbol)), d.Tag, dim(fmt.Sprintf("%-20s", d.Old)), dim("->"), d.New)
	}
	if e.AutoConfirm {
		fmt.Println()
		return model.ActionConfirm
	}
	fmt.Printf("\n[%s] confirm  [%s] skip  [%s] quit: ", green("y"), amber("s"), red("q"))
	action, err := readSingleKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nInput error: %v\n", err)
		return model.ActionQuit
	}
	fmt.Println()
	fmt.Println()
	return action
}

func (e *LogEmitter) EmitAllDone(event model.AllDoneEvent) {
	e.mu.Lock()
	e.lastAssetID = ""
	e.lastFilename = ""
	e.mu.Unlock()

	var succeeded, skipped, failed int
	for _, r := range event.Results {
		switch r.Status {
		case model.StatusSuccess:
			succeeded++
		case model.StatusSkipped:
			skipped++
		case model.StatusFailed:
			failed++
		}
	}

	failedText := fmt.Sprintf("%d failed", failed)
	if failed > 0 {
		failedText = red(failedText)
	}
	fmt.Printf("\nDone: %s, %d skipped, %s\n", green(fmt.Sprintf("%d succeeded", succeeded)), skipped, failedText)

	if failed > 0 {
		fmt.Println("\n" + red("Failed assets:"))
		for _, r := range event.Results {
			if r.Status == model.StatusFailed {
				fmt.Printf("  %s: %s\n", red(model.ShortID(r.AssetID)), r.Message)
			}
		}
	}
}

func readSingleKey() (model.DiffAction, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return model.ActionQuit, fmt.Errorf("interactive prompt unavailable; use -y for non-interactive mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 1)
	for {
		if _, err := os.Stdin.Read(buf); err != nil {
			return model.ActionQuit, fmt.Errorf("read key: %w", err)
		}
		if action, ok := decodeKey(buf[0]); ok {
			return action, nil
		}
	}
}

func decodeKey(b byte) (model.DiffAction, bool) {
	switch b {
	case 'y', 'Y', '\r', '\n':
		return model.ActionConfirm, true
	case 's', 'S', 'n', 'N':
		return model.ActionSkip, true
	case 'q', 'Q', 3, 4:
		return model.ActionQuit, true
	}
	return model.ActionQuit, false
}
