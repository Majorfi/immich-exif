//go:build !windows

package process

import (
	"bytes"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func fileOwnership(info os.FileInfo) (uint64, int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Nlink), int(stat.Uid), true
}

// preserveAttributes gives the staged file what the rename would otherwise
// drop: the original's group, permission and special bits, and extended
// attributes, which also carry POSIX ACLs on Linux. Any it cannot reproduce
// fails the commit before the original is touched.
func preserveAttributes(original string, info os.FileInfo, staged *os.File) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot read the file's group")
	}
	if err := staged.Chown(-1, int(stat.Gid)); err != nil {
		return fmt.Errorf("keep the file's group: %w", err)
	}
	if err := staged.Chmod(info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)); err != nil {
		return fmt.Errorf("keep the file's mode: %w", err)
	}
	size, err := unix.Listxattr(original, nil)
	if err != nil || size == 0 {
		return err
	}
	names := make([]byte, size)
	if size, err = unix.Listxattr(original, names); err != nil {
		return fmt.Errorf("list extended attributes: %w", err)
	}
	for _, name := range bytes.Split(names[:size], []byte{0}) {
		if len(name) == 0 {
			continue
		}
		if err := copyXattr(original, staged.Name(), string(name)); err != nil {
			return fmt.Errorf("keep extended attribute %s: %w", name, err)
		}
	}
	return nil
}

func copyXattr(source, destination, name string) error {
	size, err := unix.Getxattr(source, name, nil)
	if err != nil {
		return err
	}
	value := make([]byte, size)
	if size, err = unix.Getxattr(source, name, value); err != nil {
		return err
	}
	return unix.Setxattr(destination, name, value[:size], 0)
}
