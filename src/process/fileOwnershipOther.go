//go:build !linux && !darwin

package process

import (
	"errors"
	"os"
)

func fileOwnership(os.FileInfo) (uint64, int, bool) {
	return 0, 0, false
}

func preserveAttributes(string, os.FileInfo, *os.File) error {
	return errors.New("in-place updates are only supported on Linux and macOS")
}

func hasACL(string) bool {
	return true
}
