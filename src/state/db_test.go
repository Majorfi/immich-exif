package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/majorfi/immich-exif/model"
)

func openTestDB(t *testing.T) *StateDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-state.db")
	db, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err != nil {
		t.Fatalf("failed to open test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenStateDBCreatesTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-state.db")

	db, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("expected state.db to be created")
	}

	var tableName string
	err = db.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='assets'").Scan(&tableName)
	if err != nil {
		t.Fatalf("assets table not found: %v", err)
	}
	if tableName != "assets" {
		t.Fatalf("expected table 'assets', got %q", tableName)
	}
}

func TestOpenStateDBFilePermissions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-state.db")

	db, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()

	if err := db.Save("asset-1", "success", `{"description":"perm-check"}`); err != nil {
		t.Fatalf("save error: %v", err)
	}

	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
		perm := info.Mode().Perm()
		if perm&0077 != 0 {
			t.Errorf("%s has permissions %o, expected no group/other access", filepath.Base(path), perm)
		}
	}
}

func TestOpenStateDBHardensExistingDBFilePermissions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-state.db")

	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if err := os.Chmod(dbPath, 0644); err != nil {
		t.Fatalf("chmod file: %v", err)
	}

	db, err := OpenStateDB(dbPath, "https://immich.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer db.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db file: %v", err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatalf("state.db has permissions %o, expected no group/other access", info.Mode().Perm())
	}
}

func TestIsUpToDateReturnsFalseForUnknownAsset(t *testing.T) {
	db := openTestDB(t)

	if db.IsUpToDate("unknown-asset", `{"description":"test"}`) {
		t.Fatal("expected false for unknown asset")
	}
}

func TestSaveAndIsUpToDate(t *testing.T) {
	db := openTestDB(t)
	snapshot := `{"description":"test"}`

	if err := db.Save("asset-1", "success", snapshot); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if !db.IsUpToDate("asset-1", snapshot) {
		t.Fatal("expected IsUpToDate to return true after save")
	}
}

func TestStateDBIsolatesByServerURL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-state.db")
	snapshot := `{"description":"test"}`

	dbA, err := OpenStateDB(dbPath, "https://a.example.com")
	if err != nil {
		t.Fatalf("open A: %v", err)
	}
	defer dbA.Close()

	if err := dbA.Save("asset-1", "success", snapshot); err != nil {
		t.Fatalf("save under A: %v", err)
	}

	dbB, err := OpenStateDB(dbPath, "https://b.example.com")
	if err != nil {
		t.Fatalf("open B: %v", err)
	}
	defer dbB.Close()

	if !dbA.IsUpToDate("asset-1", snapshot) {
		t.Fatal("server A must see its own saved asset as up to date")
	}
	if dbB.IsUpToDate("asset-1", snapshot) {
		t.Fatal("server B must NOT see an asset saved under server A; the (serverURL, assetID) key isolates servers")
	}
}

func TestIsUpToDateReturnsFalseWhenExifChanged(t *testing.T) {
	db := openTestDB(t)

	oldSnapshot := `{"description":"old"}`
	newSnapshot := `{"description":"new"}`

	if err := db.Save("asset-1", "success", oldSnapshot); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if db.IsUpToDate("asset-1", newSnapshot) {
		t.Fatal("expected false when exif changed")
	}
}

func TestSnapshotAssetDeterministic(t *testing.T) {
	desc := "A photo"
	lat := 48.8566
	lng := 2.3522
	modTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	asset := model.AssetResponse{
		ID:             "asset-1",
		FileModifiedAt: modTime,
		ExifInfo: &model.ExifInfo{
			Description: &desc,
			Latitude:    &lat,
			Longitude:   &lng,
		},
	}

	snap1, err := SnapshotAsset(asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap2, err := SnapshotAsset(asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap1 != snap2 {
		t.Fatalf("snapshots differ:\n  %s\n  %s", snap1, snap2)
	}
}

func TestSnapshotAssetChangesWhenFileModified(t *testing.T) {
	desc := "A photo"
	asset1 := model.AssetResponse{
		ID:             "asset-1",
		FileModifiedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		ExifInfo:       &model.ExifInfo{Description: &desc},
	}
	asset2 := model.AssetResponse{
		ID:             "asset-1",
		FileModifiedAt: time.Date(2025, 6, 20, 14, 0, 0, 0, time.UTC),
		ExifInfo:       &model.ExifInfo{Description: &desc},
	}

	snap1, err := SnapshotAsset(asset1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap2, err := SnapshotAsset(asset2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap1 == snap2 {
		t.Fatal("snapshots should differ when fileModifiedAt changes")
	}
}

func TestSaveProcessedStatePersistsUnderBothIDs(t *testing.T) {
	db := openTestDB(t)
	snapshot := `{"description":"test"}`

	snapshots := map[string]string{
		"old-id": snapshot,
	}
	results := []model.ProcessResult{
		{AssetID: "old-id", Status: model.StatusSuccess, NewID: "new-id", Message: "uploaded (new ID: new-id)"},
	}

	SaveProcessedState(db, snapshots, results)

	if !db.IsUpToDate("new-id", snapshot) {
		t.Fatal("state should be saved under new asset ID")
	}
	if !db.IsUpToDate("old-id", snapshot) {
		t.Fatal("state should also be saved under old asset ID (may still exist for duplicate/replaced)")
	}
}

func TestSaveProcessedStateInPlaceOnlySavesOnce(t *testing.T) {
	db := openTestDB(t)
	snapshot := `{"description":"test"}`

	snapshots := map[string]string{
		"same-id": snapshot,
	}
	results := []model.ProcessResult{
		{AssetID: "same-id", Status: model.StatusSuccess, NewID: "same-id", Message: "replaced in-place"},
	}

	SaveProcessedState(db, snapshots, results)

	if !db.IsUpToDate("same-id", snapshot) {
		t.Fatal("state should be saved for in-place replacement")
	}
}

func TestSaveProcessedStateSkipsDryRun(t *testing.T) {
	db := openTestDB(t)
	snapshot := `{"description":"test"}`

	snapshots := map[string]string{
		"asset-1": snapshot,
	}
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSuccess, NewID: "", Message: "dry-run: /tmp/file.jpg"},
	}

	SaveProcessedState(db, snapshots, results)

	if db.IsUpToDate("asset-1", snapshot) {
		t.Fatal("dry-run results should not be cached")
	}
}

func TestSaveProcessedStatePersistsMetadataAlreadyMatches(t *testing.T) {
	db := openTestDB(t)
	snapshot := `{"description":"test"}`

	snapshots := map[string]string{
		"asset-1": snapshot,
	}
	results := []model.ProcessResult{
		{AssetID: "asset-1", Status: model.StatusSkipped, Message: "metadata already matches", ExifMatched: true},
	}

	SaveProcessedState(db, snapshots, results)

	if !db.IsUpToDate("asset-1", snapshot) {
		t.Fatal("metadata-already-matches results should be cached under original ID")
	}
}

func TestSnapshotAssetNilExif(t *testing.T) {
	asset := model.AssetResponse{
		ID:             "asset-1",
		FileModifiedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		ExifInfo:       nil,
	}

	snap, err := SnapshotAsset(asset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap == "" {
		t.Fatal("expected non-empty snapshot even with nil ExifInfo (fileModifiedAt still included)")
	}
}
