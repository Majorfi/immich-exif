package ui

import (
	"os"

	"golang.org/x/term"
)

const (
	ansiReset = "\033[0m"
	seqGreen  = "\033[38;2;40;200;64m" // #28c840
	seqAmber  = "\033[38;2;255;180;0m" // #ffb400
	seqRed    = "\033[38;2;255;95;87m" // #ff5f57
	seqDim    = "\033[2m"
)

var isTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

// colorEnabled reports whether ANSI color should be emitted: only when stdout is
// a real terminal and NO_COLOR is unset (https://no-color.org).
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal()
}

func colorize(seq, s string) string {
	if !colorEnabled() {
		return s
	}
	return seq + s + ansiReset
}

func green(s string) string { return colorize(seqGreen, s) }
func amber(s string) string { return colorize(seqAmber, s) }
func red(s string) string   { return colorize(seqRed, s) }
func dim(s string) string   { return colorize(seqDim, s) }

func diffSymbol(symbol string) string {
	switch symbol {
	case "+":
		return green(symbol)
	case "~":
		return amber(symbol)
	}
	return symbol
}
