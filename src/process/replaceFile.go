package process

import (
	"io"
	"os"
	"path/filepath"
)

func replaceFile(destination, source string, mode os.FileMode) error {
	dir := filepath.Dir(destination)
	staged, err := os.CreateTemp(dir, "."+filepath.Base(destination)+".immich-exif-*")
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
		err = staged.Chmod(mode)
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
	if handle, err := os.Open(dir); err == nil {
		_ = handle.Sync()
		handle.Close()
	}
	return nil
}
