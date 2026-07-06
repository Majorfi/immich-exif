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
	transient    bool
	lastCounter  string
}

func (e *LogEmitter) EmitProgress(event model.ProgressEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if event.Total > 0 {
		// -y keeps piped output diff-only, but on a terminal the live counter
		// still shows: it is transient and erased before any block prints.
		if e.AutoConfirm && !isTerminalFn() {
			return
		}
		e.printCounterLocked(event)
		return
	}

	if e.AutoConfirm {
		return
	}
	e.clearTransientLocked()
	if event.Filename != "" && (event.AssetID != e.lastAssetID || event.Filename != e.lastFilename) {
		fmt.Printf("%s %s | %s\n", dim("=>"), model.ShortID(event.AssetID), model.SanitizeForTerminal(model.TruncateFilename(event.Filename, 60)))
		e.lastAssetID = event.AssetID
		e.lastFilename = event.Filename
	}
	fmt.Printf("%s\n", dim(model.SanitizeForTerminal(event.Step)))
}

func (e *LogEmitter) printCounterLocked(event model.ProgressEvent) {
	line := fmt.Sprintf("[%d/%d] %s", event.Index, event.Total, model.SanitizeForTerminal(event.Step))
	if isTerminalFn() {
		if event.Percent > 0 {
			line = fmt.Sprintf("%s %d%%", line, event.Percent)
		}
		fmt.Printf("\r\033[K%s", dim(line))
		e.transient = true
		return
	}
	// Piped output cannot be rewritten in place: print one line per step and
	// drop percent-only updates, or a single download would emit 100 lines.
	if line == e.lastCounter {
		return
	}
	e.lastCounter = line
	fmt.Printf("%s\n", dim(line))
}

// clearTransientLocked erases the pending in-place counter line; it must run
// before any other stdout write or the counter bleeds into that output.
func (e *LogEmitter) clearTransientLocked() {
	if !e.transient {
		return
	}
	fmt.Print("\r\033[K")
	e.transient = false
}

func (e *LogEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clearTransientLocked()

	if len(event.Entries) == 0 {
		e.rememberAsset(event.AssetID, event.Filename)
		return model.ActionConfirm
	}

	if e.lastAssetID != "" && event.AssetID != "" && event.AssetID != e.lastAssetID {
		fmt.Print("\n\n")
	}
	e.rememberAsset(event.AssetID, event.Filename)

	fmt.Printf("[%d/%d] %d EXIF mismatch found for %s:\n", event.Index, event.Total, len(event.Entries), model.SanitizeForTerminal(model.TruncateFilename(event.Filename, 60)))
	for _, d := range event.Entries {
		// Old/New carry server- and file-derived values; sanitize them so they
		// cannot inject escape sequences that redraw or spoof the prompt below.
		oldArrow := dim(fmt.Sprintf("%-20s ->", model.SanitizeForTerminal(d.Old)))
		fmt.Printf("    %s %-22s %s %s\n", diffSymbol(string(d.Symbol)), d.Tag, oldArrow, model.SanitizeForTerminal(d.New))
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
	if action == model.ActionConfirm {
		fmt.Println()
	}
	return action
}

func (e *LogEmitter) rememberAsset(assetID, filename string) {
	if assetID == "" {
		return
	}
	e.lastAssetID = assetID
	e.lastFilename = filename
}

func (e *LogEmitter) EmitAllDone(event model.AllDoneEvent) {
	e.mu.Lock()
	e.clearTransientLocked()
	e.lastAssetID = ""
	e.lastFilename = ""
	e.lastCounter = ""
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
				fmt.Printf("  %s: %s\n", red(model.ShortID(r.AssetID)), model.SanitizeForTerminal(r.Message))
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
