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
	lastCounter  string
	liveOrder    []string
	liveLines    map[string]string
	drawnRows    int
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
	e.clearLiveLocked()
	if event.Filename != "" && (event.AssetID != e.lastAssetID || event.Filename != e.lastFilename) {
		fmt.Printf("%s %s | %s\n", dim("=>"), model.ShortID(event.AssetID), model.SanitizeForTerminal(model.TruncateFilename(event.Filename, 60)))
		e.lastAssetID = event.AssetID
		e.lastFilename = event.Filename
	}
	fmt.Printf("%s\n", dim(model.SanitizeForTerminal(event.Step)))
}

var terminalWidthFn = func() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func (e *LogEmitter) printCounterLocked(event model.ProgressEvent) {
	if isTerminalFn() {
		if event.Done {
			e.removeLiveLocked(event.AssetID)
			return
		}
		e.upsertLiveLocked(event)
		return
	}
	if event.Done {
		return
	}
	// Piped output cannot be rewritten in place: print one line per step and
	// drop percent-only updates, or a single download would emit 100 lines.
	line := fmt.Sprintf("[%d/%d] %s", event.Index, event.Total, model.SanitizeForTerminal(event.Step))
	if line == e.lastCounter {
		return
	}
	e.lastCounter = line
	fmt.Printf("%s\n", dim(line))
}

// The live region holds one counter line per in-flight asset (several when
// -y runs parallel workers) and is redrawn as a block: cursor up over the
// rows drawn last time, erase to screen end, reprint.
func (e *LogEmitter) upsertLiveLocked(event model.ProgressEvent) {
	line := fmt.Sprintf("[%d/%d] %s", event.Index, event.Total, model.SanitizeForTerminal(event.Step))
	if event.Percent > 0 {
		line = fmt.Sprintf("%s %d%%", line, event.Percent)
	}
	if e.liveLines == nil {
		e.liveLines = make(map[string]string)
	}
	if _, exists := e.liveLines[event.AssetID]; !exists {
		e.liveOrder = append(e.liveOrder, event.AssetID)
	}
	e.liveLines[event.AssetID] = line
	e.redrawLiveLocked()
}

func (e *LogEmitter) removeLiveLocked(assetID string) {
	if _, exists := e.liveLines[assetID]; !exists {
		return
	}
	delete(e.liveLines, assetID)
	for i, id := range e.liveOrder {
		if id == assetID {
			e.liveOrder = append(e.liveOrder[:i], e.liveOrder[i+1:]...)
			break
		}
	}
	e.redrawLiveLocked()
}

func (e *LogEmitter) redrawLiveLocked() {
	if e.drawnRows > 0 {
		fmt.Printf("\033[%dA\033[J", e.drawnRows)
	}
	width := terminalWidthFn()
	for _, id := range e.liveOrder {
		fmt.Printf("%s\n", dim(fitToWidth(e.liveLines[id], width)))
	}
	e.drawnRows = len(e.liveOrder)
}

// clearLiveLocked erases the live region; it must run before any other stdout
// write or the counter lines bleed into that output. The region content is
// kept and repainted on the next counter event.
func (e *LogEmitter) clearLiveLocked() {
	if e.drawnRows == 0 {
		return
	}
	fmt.Printf("\033[%dA\033[J", e.drawnRows)
	e.drawnRows = 0
}

// fitToWidth keeps a live line from wrapping: a wrapped line occupies two
// terminal rows and breaks the cursor-up arithmetic of the region redraw.
func fitToWidth(line string, width int) string {
	if width <= 1 {
		return line
	}
	runes := []rune(line)
	if len(runes) < width {
		return line
	}
	return string(runes[:width-1])
}

func (e *LogEmitter) EmitDiff(event model.DiffEvent) model.DiffAction {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.clearLiveLocked()

	if len(event.Entries) == 0 {
		e.rememberAsset(event.AssetID, event.Filename)
		return model.ActionConfirm
	}

	if e.lastAssetID != "" && event.AssetID != "" && event.AssetID != e.lastAssetID {
		fmt.Print("\n\n")
	}
	e.rememberAsset(event.AssetID, event.Filename)

	fmt.Printf("[%d/%d] %d EXIF mismatch found for %s:\n", event.Index, event.Total, len(event.Entries), model.SanitizeForTerminal(model.TruncateFilename(event.Filename, 60)))
	// Column widths grow to the block's longest tag and old value so no row
	// pushes the arrow out of line; the old column is capped so one huge
	// value (e.g. a long description) cannot blow up every row.
	tagWidth := 22
	oldWidth := 20
	for _, d := range event.Entries {
		if len(d.Tag) > tagWidth {
			tagWidth = len(d.Tag)
		}
		if l := len(model.SanitizeForTerminal(d.Old)); l > oldWidth {
			oldWidth = l
		}
	}
	if oldWidth > 45 {
		oldWidth = 45
	}
	for _, d := range event.Entries {
		// Old/New carry server- and file-derived values; sanitize them so they
		// cannot inject escape sequences that redraw or spoof the prompt below.
		oldArrow := dim(fmt.Sprintf("%-*s ->", oldWidth, model.SanitizeForTerminal(d.Old)))
		fmt.Printf("    %s %-*s %s %s\n", diffSymbol(string(d.Symbol)), tagWidth, d.Tag, oldArrow, model.SanitizeForTerminal(d.New))
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
	e.clearLiveLocked()
	e.lastAssetID = ""
	e.lastFilename = ""
	e.lastCounter = ""
	e.liveOrder = nil
	e.liveLines = nil
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
