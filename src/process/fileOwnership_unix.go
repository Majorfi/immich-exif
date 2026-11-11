//go:build !windows

package process

import (
	"os"
	"syscall"
)

func fileOwnership(info os.FileInfo) (uint64, int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Nlink), int(stat.Uid), true
}
