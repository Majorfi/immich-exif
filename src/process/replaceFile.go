package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errNotDurable = errors.New("the rename may not survive a power loss")

func replaceFile(destination, source string, original os.FileInfo) error {
	dir := filepath.Dir(destination)
	staged, err := os.CreateTemp(dir, ".immich-exif-*")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(staged.Name())
		}
	}()
	input, err := os.Open(source)
	if err != nil {
		staged.Close()
		return err
	}
	_, err = io.Copy(staged, input)
	input.Close()
	if err == nil {
		err = preserveAttributes(destination, original, staged)
	}
	if err == nil {
		err = staged.Sync()
	}
	if closeErr := staged.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(staged.Name(), destination); err != nil {
		return err
	}
	committed = true
	handle, err := os.Open(dir)
	if err == nil {
		err = handle.Sync()
		handle.Close()
	}
	if err != nil {
		return fmt.Errorf("%w: syncing %s failed: %v", errNotDurable, dir, err)
	}
	return nil
}
