package model

import "testing"

func TestShortID(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "long ID truncated", input: "abcdefghij", want: "abcdefgh"},
		{name: "exactly 8", input: "abcdefgh", want: "abcdefgh"},
		{name: "short ID unchanged", input: "abc", want: "abc"},
		{name: "empty", input: "", want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := ShortID(testCase.input)
			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestTruncateFilename(t *testing.T) {
	testCases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "short name unchanged", input: "photo.jpg", maxLen: 20, want: "photo.jpg"},
		{name: "exact length unchanged", input: "photo.jpg", maxLen: 9, want: "photo.jpg"},
		{name: "long name truncated with ellipsis", input: "very-long-filename.jpg", maxLen: 15, want: "...filename.jpg"},
		{name: "single char suffix", input: "abcdef", maxLen: 4, want: "...f"},
		{name: "maxLen 3 keeps suffix", input: "abcdef", maxLen: 3, want: "def"},
		{name: "maxLen 2 keeps suffix", input: "abcdef", maxLen: 2, want: "ef"},
		{name: "maxLen 1 keeps suffix", input: "abcdef", maxLen: 1, want: "f"},
		{name: "maxLen 0 returns empty", input: "abcdef", maxLen: 0, want: ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := TruncateFilename(testCase.input, testCase.maxLen)
			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestSanitizeForTerminal(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain text untouched", input: "photo.jpg (1)", want: "photo.jpg (1)"},
		{name: "ansi escape stripped", input: "\x1b[2Jspoofed", want: "[2Jspoofed"},
		{name: "carriage return stripped", input: "line\rredraw", want: "lineredraw"},
		{name: "bell and del stripped", input: "a\x07b\x7fc", want: "abc"},
		{name: "newline and tab kept", input: "a\n\tb", want: "a\n\tb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeForTerminal(tc.input); got != tc.want {
				t.Fatalf("SanitizeForTerminal(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
