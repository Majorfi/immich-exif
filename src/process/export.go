package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errDestExists is returned (wrapped) by copyFile when the destination already
// exists, so copyFileToDir can distinguish a name clash from a real I/O error.
var errDestExists = errors.New("destination exists")

// copyFileToDir copies src into dir under baseName. When allowSuffix is set and
// baseName is taken, it inserts "-NNN" before the extension until it finds a
// free name; the O_EXCL create makes each claim atomic, so parallel workers
// exporting distinct assets that share a timestamp never overwrite each other.
// It returns the path actually written.
func copyFileToDir(src, dir, baseName string, allowSuffix bool) (string, error) {
	dest := filepath.Join(dir, baseName)
	err := copyFile(src, dest)
	if err == nil {
		return dest, nil
	}
	if !allowSuffix || !errors.Is(err, errDestExists) {
		return "", err
	}

	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%03d%s", stem, i, ext))
		err := copyFile(src, candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, errDestExists) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free name for %q after 999 suffixes", baseName)
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", errDestExists, dst)
		}
		return err
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(dst)
		return err
	}
	if err := dstFile.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
