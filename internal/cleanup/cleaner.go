package cleanup

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sleepcast/internal/storage"
)

type Cleaner struct {
	DB       *storage.DB
	Media    *storage.Media
	TTLHours int
}

// PurgeOne deletes the file and DB row for videoID. Missing file is not an error.
func (c *Cleaner) PurgeOne(videoID string) error {
	d, err := c.DB.GetDownload(videoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := os.Remove(d.Filepath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Printf("cleanup: remove %s: %v", d.Filepath, err)
	}
	return c.DB.DeleteDownload(videoID)
}

func (c *Cleaner) SweepExpired() error {
	cutoff := time.Now().Add(-time.Duration(c.TTLHours) * time.Hour).Unix()
	rows, err := c.DB.DownloadsOlderThan(cutoff)
	if err != nil {
		return err
	}
	for _, d := range rows {
		if err := os.Remove(d.Filepath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("cleanup sweep: remove %s: %v", d.Filepath, err)
		}
		if err := c.DB.DeleteDownload(d.VideoID); err != nil {
			log.Printf("cleanup sweep: db delete %s: %v", d.VideoID, err)
		}
	}
	if len(rows) > 0 {
		log.Printf("cleanup sweep: removed %d expired downloads", len(rows))
	}
	return nil
}

func (c *Cleaner) Reconcile() error {
	rows, err := c.DB.AllDownloads()
	if err != nil {
		return err
	}
	tracked := make(map[string]struct{}, len(rows))
	for _, d := range rows {
		tracked[d.Filepath] = struct{}{}
		if _, err := os.Stat(d.Filepath); errors.Is(err, fs.ErrNotExist) {
			log.Printf("reconcile: file missing, removing row %s", d.VideoID)
			_ = c.DB.DeleteDownload(d.VideoID)
		}
	}
	return filepath.WalkDir(c.Media.Root, func(path string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".part") {
			log.Printf("reconcile: removing stale partial %s", path)
			_ = os.Remove(path)
			return nil
		}
		if !strings.HasSuffix(path, ".m4a") {
			return nil
		}
		if _, ok := tracked[path]; !ok {
			log.Printf("reconcile: orphan file, removing %s", path)
			_ = os.Remove(path)
		}
		return nil
	})
}

func (c *Cleaner) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	if err := c.SweepExpired(); err != nil {
		log.Printf("cleanup: initial sweep: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.SweepExpired(); err != nil {
				log.Printf("cleanup: sweep: %v", err)
			}
		}
	}
}
