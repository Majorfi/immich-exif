package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/majorfi/immich-exif/model"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	failStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	addStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	changeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

func tuiRender(m *tuiModel) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("immich-exif"))
	b.WriteString("\n\n")

	counter := fmt.Sprintf("  %d/%d", m.completed, m.total)
	b.WriteString(renderProgressBar(m.completed, m.total, m.width-len(counter)))
	b.WriteString(counter)
	b.WriteString("\n\n")

	if m.current.AssetID != "" {
		header := fmt.Sprintf("=> %s", model.ShortID(m.current.AssetID))
		if m.current.Filename != "" {
			header += fmt.Sprintf(" | %s", model.TruncateFilename(m.current.Filename, 50))
		}
		b.WriteString(header)
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("   %s\n", m.current.Step))
	}

	if m.diff != nil {
		maxFilenameLen := m.width - 40
		if maxFilenameLen < 20 {
			maxFilenameLen = 20
		}
		b.WriteString(fmt.Sprintf("   %d EXIF mismatch found for %s:\n", len(m.diff.Entries), model.TruncateFilename(m.diff.Filename, maxFilenameLen)))
		for _, d := range m.diff.Entries {
			style := changeStyle
			if d.Symbol == model.DiffAdd {
				style = addStyle
			}
			line := fmt.Sprintf("     %s %-22s %-20s -> %s", string(d.Symbol), d.Tag, d.Old, d.New)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}
	}

	if m.done {
		renderResultsSummary(&b, m)
		b.WriteString(dimStyle.Render("\n  Press Enter, Space or Escape to close"))
	} else if !m.autoConfirm && !m.waitingForConfirm && !m.awaitingNextDiff {
		renderResultsRolling(&b, m)
	}

	if !m.done && !m.autoConfirm {
		if m.waitingForConfirm {
			b.WriteString("\n  ")
			b.WriteString(successStyle.Render("[y]"))
			b.WriteString(" confirm  ")
			b.WriteString(warnStyle.Render("[s]"))
			b.WriteString(" skip  ")
			b.WriteString(failStyle.Render("[q]"))
			b.WriteString(" quit")
		} else if m.cancelRequested {
			b.WriteString(dimStyle.Render("\n  Cancellation requested, waiting for final summary..."))
		} else {
			b.WriteString(dimStyle.Render("\n  Press q to cancel remaining work"))
		}
	}

	return b.String()
}

func renderResultsRolling(b *strings.Builder, m *tuiModel) {
	var visible []model.ResultEvent
	for _, re := range m.results {
		if re.Result.Status == model.StatusSkipped {
			continue
		}
		visible = append(visible, re)
	}
	if len(visible) == 0 {
		return
	}
	b.WriteString("\n")
	start := 0
	if len(visible) > 10 {
		start = len(visible) - 10
	}
	maxMsgLen := m.width - 30
	if maxMsgLen < 30 {
		maxMsgLen = 30
	}
	for _, re := range visible[start:] {
		renderResultLine(b, re, m.total, maxMsgLen)
	}
}

func renderResultsSummary(b *strings.Builder, m *tuiModel) {
	var succeeded, skipped, failed int
	for _, r := range m.allDone.Results {
		switch r.Status {
		case model.StatusSuccess:
			succeeded++
		case model.StatusSkipped:
			skipped++
		case model.StatusFailed:
			failed++
		}
	}

	maxMsgLen := m.width - 30
	if maxMsgLen < 30 {
		maxMsgLen = 30
	}
	for i, r := range m.allDone.Results {
		re := model.ResultEvent{Index: i + 1, Total: m.total, Result: r}
		renderResultLine(b, re, m.total, maxMsgLen)
	}

	b.WriteString(fmt.Sprintf("\n  %s %d  %s %d  %s %d\n",
		successStyle.Render("succeeded"), succeeded,
		warnStyle.Render("skipped"), skipped,
		failStyle.Render("failed"), failed,
	))
}

func renderResultLine(b *strings.Builder, re model.ResultEvent, total, maxMsgLen int) {
	r := re.Result
	var style lipgloss.Style
	var label string
	switch r.Status {
	case model.StatusSuccess:
		style = successStyle
		label = model.TruncateFilename(r.Message, maxMsgLen)
	case model.StatusSkipped:
		style = warnStyle
		label = fmt.Sprintf("skipped (%s)", model.TruncateFilename(r.Message, maxMsgLen))
	case model.StatusFailed:
		style = failStyle
		label = fmt.Sprintf("FAILED (%s)", model.TruncateFilename(r.Message, maxMsgLen))
	}
	b.WriteString(style.Render(fmt.Sprintf("  [%d/%d] %s: %s", re.Index, re.Total, model.ShortID(r.AssetID), label)))
	b.WriteString("\n")
}

func renderProgressBar(completed, total, width int) string {
	if width < 10 {
		width = 10
	}
	if total == 0 {
		return strings.Repeat("░", width)
	}

	filled := width * completed / total
	if filled > width {
		filled = width
	}
	empty := width - filled

	bar := successStyle.Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", empty))
	return bar
}
