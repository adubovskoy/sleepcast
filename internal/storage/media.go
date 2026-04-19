package storage

import (
	"os"
	"path/filepath"
)

type Media struct {
	Root string
}

func NewMedia(dataDir string) (*Media, error) {
	root := filepath.Join(dataDir, "media")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Media{Root: root}, nil
}

func (m *Media) FilePath(videoID string) string {
	return filepath.Join(m.Root, videoID+".m4a")
}
