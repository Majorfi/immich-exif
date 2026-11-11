//go:build linux

package process

import (
	"errors"

	"golang.org/x/sys/unix"
)

func hasACL(string) bool {
	return false
}

func clearACL(path string) error {
	err := unix.Removexattr(path, "system.posix_acl_access")
	if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		return nil
	}
	return err
}
