package process

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/majorfi/immich-exif/exif"
	"github.com/majorfi/immich-exif/model"
)

// renameSampleTime is an arbitrary fixed moment used to validate a pattern at
// startup without depending on the wall clock.
var renameSampleTime = time.Date(2004, 6, 5, 14, 25, 32, 0, time.UTC)

// renderCaptureName expands a strftime-subset pattern using t. Supported verbs:
// %Y (2004) %y (04) %m (06) %d (05) %H (14) %M (25) %S (32) and %% for a literal
// percent. An unknown or dangling verb is an error so a typo fails loudly rather
// than leaking a raw "%x" into a filename.
func renderCaptureName(pattern string, t time.Time) (string, error) {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' {
			b.WriteByte(pattern[i])
			continue
		}
		i++
		if i >= len(pattern) {
			return "", fmt.Errorf("rename pattern ends with a dangling %%")
		}
		switch pattern[i] {
		case 'Y':
			b.WriteString(t.Format("2006"))
		case 'y':
			b.WriteString(t.Format("06"))
		case 'm':
			b.WriteString(t.Format("01"))
		case 'd':
			b.WriteString(t.Format("02"))
		case 'H':
			b.WriteString(t.Format("15"))
		case 'M':
			b.WriteString(t.Format("04"))
		case 'S':
			b.WriteString(t.Format("05"))
		case '%':
			b.WriteByte('%')
		default:
			return "", fmt.Errorf("unknown rename pattern verb %%%c (supported: %%Y %%y %%m %%d %%H %%M %%S %%%%)", pattern[i])
		}
	}
	return b.String(), nil
}

// ValidateRenamePattern reports whether pattern renders without error and yields
// a filesystem-safe name, so a bad pattern is rejected at startup instead of
// mid-run after some files were already written.
func ValidateRenamePattern(pattern string) error {
	rendered, err := renderCaptureName(pattern, renameSampleTime)
	if err != nil {
		return err
	}
	if _, err := safePathComponent("rename pattern result", rendered+".jpg"); err != nil {
		return fmt.Errorf("rename pattern must not contain path separators: %q", pattern)
	}
	return nil
}

// exportFileName returns the filename to write for an asset in export mode. With
// renaming enabled it derives the name from the corrected capture date and keeps
// the original extension; when the server has no usable date it falls back to
// the original name so the asset is still exported.
func exportFileName(cfg *model.Config, asset *model.AssetResponse, originalSafeName string) (string, error) {
	if !cfg.Rename {
		return originalSafeName, nil
	}
	t, ok := exif.CorrectedLocalTime(asset.ExifInfo)
	if !ok {
		return originalSafeName, nil
	}
	stem, err := renderCaptureName(cfg.RenamePattern, t)
	if err != nil {
		return "", err
	}
	name := stem + filepath.Ext(originalSafeName)
	if _, err := safePathComponent("rename pattern result", name); err != nil {
		return "", err
	}
	return name, nil
}
