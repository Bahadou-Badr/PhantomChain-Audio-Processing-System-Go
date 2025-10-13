package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Storage defines operations we need
type Storage interface {
	Save(r io.Reader, destPath string) (int64, error)
	EnsureBasePath(base string) error
}

// LocalFS implements Storage on disk (dev)
type LocalFS struct {
	BasePath string
}

func NewLocalFS(base string) *LocalFS {
	return &LocalFS{BasePath: base}
}

func (l *LocalFS) EnsureBasePath(base string) error {
	return os.MkdirAll(base, 0o755)
}

func (l *LocalFS) Save(r io.Reader, destPath string) (int64, error) {
	full := filepath.Join(l.BasePath, destPath)
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(full)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, r)
	if err != nil {
		return n, err
	}
	return n, nil
}

// Helper to build path with timestamp filename suffix
func BuildPath(filename string) string {
	t := time.Now().UTC().Format("20060102-150405")
	return filepath.Join(t[:8], fmt.Sprintf("%s-%s", t, filename))
}
