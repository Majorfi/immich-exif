//go:build windows

package process

import "os"

func fileOwnership(os.FileInfo) (uint64, int, bool) {
	return 0, 0, false
}
