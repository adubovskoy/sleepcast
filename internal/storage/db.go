package storage

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(dataDir string) (*DB, error) {
	path := filepath.Join(dataDir, "sleepcast.db")
	sqldb, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db := &DB{sqldb}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS downloads (
			video_id       TEXT PRIMARY KEY,
			title          TEXT,
			filepath       TEXT NOT NULL,
			downloaded_at  INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_downloads_downloaded_at ON downloads(downloaded_at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

type Download struct {
	VideoID      string
	Title        string
	Filepath     string
	DownloadedAt int64
}

func (db *DB) UpsertDownload(d Download) error {
	_, err := db.Exec(`
		INSERT INTO downloads (video_id, title, filepath, downloaded_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(video_id) DO UPDATE SET
			title = excluded.title,
			filepath = excluded.filepath,
			downloaded_at = excluded.downloaded_at
	`, d.VideoID, d.Title, d.Filepath, d.DownloadedAt)
	return err
}

func (db *DB) GetDownload(videoID string) (*Download, error) {
	d := &Download{}
	err := db.QueryRow(`
		SELECT video_id, COALESCE(title, ''), filepath, downloaded_at
		FROM downloads WHERE video_id = ?`, videoID).
		Scan(&d.VideoID, &d.Title, &d.Filepath, &d.DownloadedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (db *DB) DeleteDownload(videoID string) error {
	_, err := db.Exec(`DELETE FROM downloads WHERE video_id = ?`, videoID)
	return err
}

func (db *DB) DownloadsOlderThan(cutoff int64) ([]Download, error) {
	rows, err := db.Query(`
		SELECT video_id, COALESCE(title, ''), filepath, downloaded_at
		FROM downloads WHERE downloaded_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Download
	for rows.Next() {
		var d Download
		if err := rows.Scan(&d.VideoID, &d.Title, &d.Filepath, &d.DownloadedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (db *DB) AllDownloads() ([]Download, error) {
	rows, err := db.Query(`
		SELECT video_id, COALESCE(title, ''), filepath, downloaded_at FROM downloads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Download
	for rows.Next() {
		var d Download
		if err := rows.Scan(&d.VideoID, &d.Title, &d.Filepath, &d.DownloadedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
