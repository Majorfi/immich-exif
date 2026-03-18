package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/majorfi/immich-exif/model"

	_ "modernc.org/sqlite"
)

type StateDB struct {
	db        *sql.DB
	dbPath    string
	serverURL string
}

var OpenStateDBFileFn = os.OpenFile
var OpenStateDBSQLFn = sql.Open
var ChmodStateDBPathFn = os.Chmod
var HardenStateDBSidecarsFn = hardenSidecarFiles
var UserConfigDirFn = os.UserConfigDir
var MkdirStateDBDirFn = os.MkdirAll

func StateDBPath() (string, error) {
	configDir, err := UserConfigDirFn()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	dir := filepath.Join(configDir, "immich-exif")
	if err := MkdirStateDBDirFn(dir, 0700); err != nil {
		return "", fmt.Errorf("create state DB dir: %w", err)
	}
	return filepath.Join(dir, "state.db"), nil
}

func OpenStateDB(dbPath, serverURL string) (*StateDB, error) {
	f, err := OpenStateDBFileFn(dbPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("create state db file: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close state db file: %w", err)
	}

	db, err := OpenStateDBSQLFn("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}

	if err := ChmodStateDBPathFn(dbPath, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("harden state db file: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	createSQL := `CREATE TABLE IF NOT EXISTS assets (
		serverURL TEXT NOT NULL,
		assetID TEXT NOT NULL,
		status TEXT NOT NULL,
		processedAt TEXT NOT NULL,
		exifSnapshot TEXT,
		PRIMARY KEY (serverURL, assetID)
	)`
	if _, err := db.Exec(createSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	if err := HardenStateDBSidecarsFn(dbPath); err != nil {
		db.Close()
		return nil, err
	}

	return &StateDB{db: db, dbPath: dbPath, serverURL: serverURL}, nil
}

func hardenSidecarFiles(dbPath string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecarPath := dbPath + suffix
		if err := os.Chmod(sidecarPath, 0600); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("harden sqlite sidecar file: %w", err)
		}
	}
	return nil
}

func (s *StateDB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *StateDB) IsUpToDate(assetID, snapshot string) bool {
	var stored string
	err := s.db.QueryRow(
		"SELECT exifSnapshot FROM assets WHERE serverURL = ? AND assetID = ?",
		s.serverURL, assetID,
	).Scan(&stored)
	if err != nil {
		return false
	}
	return stored == snapshot
}

func (s *StateDB) Save(assetID, status, exifSnapshot string) error {
	_, err := s.db.Exec(`
		INSERT INTO assets (serverURL, assetID, status, processedAt, exifSnapshot)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (serverURL, assetID)
		DO UPDATE SET status = excluded.status,
		              processedAt = excluded.processedAt,
		              exifSnapshot = excluded.exifSnapshot`,
		s.serverURL, assetID, status, time.Now().UTC().Format(time.RFC3339), exifSnapshot,
	)
	if err != nil {
		return err
	}
	return HardenStateDBSidecarsFn(s.dbPath)
}

type assetSnapshot struct {
	ExifInfo       *model.ExifInfo `json:"exifInfo"`
	FileModifiedAt time.Time       `json:"fileModifiedAt"`
}

func SnapshotAsset(asset model.AssetResponse) (string, error) {
	snap := assetSnapshot{
		ExifInfo:       asset.ExifInfo,
		FileModifiedAt: asset.FileModifiedAt,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func SaveProcessedState(stateDB *StateDB, snapshots map[string]string, results []model.ProcessResult) {
	for _, r := range results {
		snap, hasPending := snapshots[r.AssetID]
		if !hasPending {
			continue
		}
		if r.Status == model.StatusSuccess && r.NewID != "" {
			if err := stateDB.Save(r.NewID, r.Status.String(), snap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save state for asset %s: %v\n", r.NewID, err)
			}
			if r.NewID != r.AssetID {
				if err := stateDB.Save(r.AssetID, r.Status.String(), snap); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save state for asset %s: %v\n", r.AssetID, err)
				}
			}
		} else if r.ExifMatched {
			if err := stateDB.Save(r.AssetID, r.Status.String(), snap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save state for asset %s: %v\n", r.AssetID, err)
			}
		}
	}
}
