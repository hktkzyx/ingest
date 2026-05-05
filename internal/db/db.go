package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Record struct {
	SourcePath string
	TargetPath string
	DeviceID   string
	VolumeID   string
	Size       int64
	Hash       string
	CreatedAt  time.Time
}

type DB struct {
	sql *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS ingest_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_path TEXT NOT NULL,
    target_path TEXT NOT NULL,
    device_id TEXT NOT NULL,
    volume_id TEXT,
    size_bytes INTEGER NOT NULL,
    xxhash64 TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(target_path)
);
CREATE INDEX IF NOT EXISTS idx_target ON ingest_history(target_path);
CREATE INDEX IF NOT EXISTS idx_hash ON ingest_history(xxhash64);
`

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	sd, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := sd.Exec(schema); err != nil {
		sd.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &DB{sql: sd}, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) Get(targetPath string) (*Record, error) {
	var r Record
	err := d.sql.QueryRow(
		`SELECT source_path, target_path, device_id, COALESCE(volume_id,''), size_bytes, xxhash64, created_at
		 FROM ingest_history WHERE target_path = ?`, targetPath).
		Scan(&r.SourcePath, &r.TargetPath, &r.DeviceID, &r.VolumeID, &r.Size, &r.Hash, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) Put(r Record) error {
	_, err := d.sql.Exec(
		`INSERT INTO ingest_history(source_path, target_path, device_id, volume_id, size_bytes, xxhash64)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(target_path) DO UPDATE SET
		    source_path=excluded.source_path,
		    device_id=excluded.device_id,
		    volume_id=excluded.volume_id,
		    size_bytes=excluded.size_bytes,
		    xxhash64=excluded.xxhash64,
		    created_at=CURRENT_TIMESTAMP`,
		r.SourcePath, r.TargetPath, r.DeviceID, r.VolumeID, r.Size, r.Hash)
	return err
}
