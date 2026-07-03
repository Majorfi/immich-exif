package model

import "strings"

// SanitizeForTerminal strips ASCII control characters (except newline and tab)
// from server- or file-derived strings so they cannot inject ANSI/OSC escape
// sequences when printed, e.g. to spoof the interactive confirm prompt.
func SanitizeForTerminal(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
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
