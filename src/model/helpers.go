package model

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
