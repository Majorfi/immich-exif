//go:build windows

package process

import (
	"errors"
	"os"
)

func fileOwnership(os.FileInfo) (uint64, int, bool) {
	return 0, 0, false
}

func preserveAttributes(string, os.FileInfo, *os.File) error {
	return errors.New("in-place updates are not supported on Windows")
}
