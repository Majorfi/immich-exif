package state

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/majorfi/immich-exif/model"
)

func TestStateDBPathUsesConfigDirectory(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("HOME", configDir)

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("resolve user config dir: %v", err)
	}

	dbPath, err := StateDBPath()
	if err != nil {
		t.Fatalf("stateDBPath error: %v", err)
	}
	wantPath := filepath.Join(userConfigDir, "immich-exif", "state.db")

	if dbPath != wantPath {
		t.Fatalf("expected state DB path %q, got %q", wantPath, dbPath)
	}
	if info, err := os.Stat(filepath.Dir(dbPath)); err != nil || !info.IsDir() {
		t.Fatalf("expected state DB directory to exist, err=%v", err)
	}
}

func TestStateDBPathFailsWhenUserConfigDirFails(t *testing.T) {
	restore := withStateDBHooks()
	defer restore()

	UserConfigDirFn = func() (string, error) {
		return "", errors.New("config-dir-failure")
	}

	_, err := StateDBPath()
	if err == nil {
		t.Fatal("expected stateDBPath to fail when user config dir fails")
	}
	if !strings.Contains(err.Error(), "resolve config dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStateDBPathFailsWhenCreateDirectoryFails(t *testing.T) {
	restore := withStateDBHooks()
	defer restore()

	UserConfigDirFn = func() (string, error) {
		return t.TempDir(), nil
	}
	MkdirStateDBDirFn = func(path string, mode os.FileMode) error {
		return errors.New("mkdir-failure")
	}

	_, err := StateDBPath()
	if err == nil {
		t.Fatal("expected stateDBPath to fail when directory creation fails")
	}
	if !strings.Contains(err.Error(), "create state DB dir") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenStateDBFailsWhenParentDirectoryMissing(t *testing.T) {
	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "missing-parent", "state.db")

	_, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err == nil {
		t.Fatal("expected openStateDB to fail when parent directory is missing")
	}
	if !strings.Contains(err.Error(), "create state db file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenStateDBFailsWhenSQLOpenFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	restore := withStateDBHooks()
	defer restore()

	OpenStateDBSQLFn = func(driverName, dataSourceName string) (*sql.DB, error) {
		return nil, errors.New("sql-open-failure")
	}

	_, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err == nil {
		t.Fatal("expected open state DB error")
	}
	if !strings.Contains(err.Error(), "open state db") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenStateDBFailsWhenChmodFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	restore := withStateDBHooks()
	defer restore()

	ChmodStateDBPathFn = func(path string, mode os.FileMode) error {
		if path == dbPath {
			return errors.New("chmod-failure")
		}
		return os.Chmod(path, mode)
	}

	_, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err == nil {
		t.Fatal("expected chmod failure")
	}
	if !strings.Contains(err.Error(), "harden state db file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenStateDBFailsWhenPragmaFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	restore := withStateDBHooks()
	defer restore()

	OpenStateDBSQLFn = func(driverName, dataSourceName string) (*sql.DB, error) {
		db, err := sql.Open(driverName, dataSourceName)
		if err != nil {
			return nil, err
		}
		db.Close()
		return db, nil
	}

	_, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err == nil {
		t.Fatal("expected PRAGMA failure")
	}
	if !strings.Contains(err.Error(), "set WAL mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenStateDBFailsWhenHardenSidecarsFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")

	restore := withStateDBHooks()
	defer restore()

	HardenStateDBSidecarsFn = func(path string) error {
		return errors.New("sidecar-harden-failure")
	}

	_, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err == nil {
		t.Fatal("expected sidecar harden failure")
	}
	if !strings.Contains(err.Error(), "sidecar-harden-failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStateDBCloseHandlesNil(t *testing.T) {
	var nilStateDB *StateDB
	if err := nilStateDB.Close(); err != nil {
		t.Fatalf("expected nil state DB close to succeed, got %v", err)
	}

	emptyStateDB := &StateDB{}
	if err := emptyStateDB.Close(); err != nil {
		t.Fatalf("expected empty state DB close to succeed, got %v", err)
	}
}

func TestHardenSidecarFilesIgnoresMissingFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	if err := hardenSidecarFiles(dbPath); err != nil {
		t.Fatalf("expected missing sidecar files to be ignored, got %v", err)
	}
}

func TestSaveProcessedStateLogsWarningWhenSaveFails(t *testing.T) {
	stateDB := openTestDB(t)
	if err := stateDB.Close(); err != nil {
		t.Fatalf("close state DB: %v", err)
	}

	snapshots := map[string]string{
		"asset-1": `{"description":"test"}`,
	}
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSuccess, NewID: "asset-1", Message: "uploaded"},
	}

	output := captureStateDBStderr(t, func() {
		SaveProcessedState(stateDB, snapshots, results)
	})

	if !strings.Contains(output, "Warning: failed to save state for asset asset-1") {
		t.Fatalf("expected warning about failed save, got %q", output)
	}
}

func captureStateDBStderr(t *testing.T, runFn func()) string {
	t.Helper()
	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writer

	runFn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	os.Stderr = originalStderr

	outputBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr output: %v", err)
	}
	return string(outputBytes)
}

func withStateDBHooks() func() {
	originalOpenFile := OpenStateDBFileFn
	originalSQLOpen := OpenStateDBSQLFn
	originalChmod := ChmodStateDBPathFn
	originalHardenSidecars := HardenStateDBSidecarsFn
	originalUserConfigDir := UserConfigDirFn
	originalMkdirStateDBDir := MkdirStateDBDirFn

	return func() {
		OpenStateDBFileFn = originalOpenFile
		OpenStateDBSQLFn = originalSQLOpen
		ChmodStateDBPathFn = originalChmod
		HardenStateDBSidecarsFn = originalHardenSidecars
		UserConfigDirFn = originalUserConfigDir
		MkdirStateDBDirFn = originalMkdirStateDBDir
	}
}
