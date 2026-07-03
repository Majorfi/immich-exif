package model

import "strings"

// SanitizeForTerminal neutralizes server- or file-derived strings before they
// are printed: C0 and C1 control characters (ANSI/OSC escape introducers) are
// stripped, and newlines/tabs are flattened to spaces so a value cannot
// fabricate extra terminal lines, e.g. fake diff rows above the confirm prompt.
func SanitizeForTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func TruncateFilename(name string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(name) <= maxLen {
		return name
	}
	if maxLen <= 3 {
		return name[len(name)-maxLen:]
	}
	return "..." + name[len(name)-maxLen+3:]
}
