package ui

import "testing"

func TestColorEnabledWrapsWithAnsi(t *testing.T) {
	orig := isTerminal
	isTerminal = func() bool { return true }
	defer func() { isTerminal = orig }()
	t.Setenv("NO_COLOR", "")

	if got := green("x"); got != seqGreen+"x"+ansiReset {
		t.Fatalf("expected ANSI-wrapped green, got %q", got)
	}
	if diffSymbol("+") != green("+") {
		t.Fatal("+ should be green")
	}
	if diffSymbol("~") != amber("~") {
		t.Fatal("~ should be amber")
	}
	if diffSymbol("=") != "=" {
		t.Fatalf("unknown symbol should be left plain, got %q", diffSymbol("="))
	}
}

func TestColorDisabledByNoColor(t *testing.T) {
	orig := isTerminal
	isTerminal = func() bool { return true }
	defer func() { isTerminal = orig }()
	t.Setenv("NO_COLOR", "1")

	if got := red("x"); got != "x" {
		t.Fatalf("NO_COLOR must disable color even on a terminal, got %q", got)
	}
}

func TestColorDisabledWithoutTerminal(t *testing.T) {
	orig := isTerminal
	isTerminal = func() bool { return false }
	defer func() { isTerminal = orig }()
	t.Setenv("NO_COLOR", "")

	if got := dim("x"); got != "x" {
		t.Fatalf("expected plain text when stdout is not a terminal, got %q", got)
	}
}
