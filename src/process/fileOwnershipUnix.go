//go:build !windows

package process

import (
	"bytes"
	"fmt"
	"os"
	"strings"
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
// attributes, which also carry POSIX ACLs on Linux, minus any ACL the new
// file inherited from its directory. security.* labels cannot be set without
// privileges, so they must already match.
// Anything it cannot reproduce fails the commit before the original is touched.
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
	if err := clearACL(staged.Name()); err != nil {
		return fmt.Errorf("drop an inherited access control list: %w", err)
	}
	names, err := xattrNames(original)
	if err != nil {
		return fmt.Errorf("list extended attributes: %w", err)
	}
	stagedNames, err := xattrNames(staged.Name())
	if err != nil {
		return fmt.Errorf("list the new file's extended attributes: %w", err)
	}
	for _, name := range stagedNames {
		if strings.HasPrefix(name, "security.") && !sameXattr(original, staged.Name(), name) {
			return fmt.Errorf("the new file would get a different %s label, which only the system can set", name)
		}
	}
	for _, name := range names {
		if strings.HasPrefix(name, "security.") {
			if !sameXattr(original, staged.Name(), name) {
				return fmt.Errorf("the new file would get a different %s label, which only the system can set", name)
			}
			continue
		}
		if err := copyXattr(original, staged.Name(), name); err != nil {
			return fmt.Errorf("keep extended attribute %s: %w", name, err)
		}
	}
	if hasACL(staged.Name()) {
		return fmt.Errorf("the new file kept an access control list the original does not have")
	}
	return nil
}

func xattrNames(path string) ([]string, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil || size == 0 {
		return nil, err
	}
	buffer := make([]byte, size)
	if size, err = unix.Listxattr(path, buffer); err != nil {
		return nil, err
	}
	var names []string
	for _, name := range bytes.Split(buffer[:size], []byte{0}) {
		if len(name) > 0 {
			names = append(names, string(name))
		}
	}
	return names, nil
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

func sameXattr(first, second, name string) bool {
	read := func(path string) ([]byte, bool) {
		size, err := unix.Getxattr(path, name, nil)
		if err != nil {
			return nil, false
		}
		value := make([]byte, size)
		size, err = unix.Getxattr(path, name, value)
		return value[:size], err == nil
	}
	a, okA := read(first)
	b, okB := read(second)
	return okA && okB && bytes.Equal(a, b)
}
