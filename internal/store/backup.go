package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Backup publishes a consistent SQLite snapshot without overwriting an existing path.
// It excludes endpoint credentials and other files; it is not a full product export.
func (s *Store) Backup(ctx context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backup(ctx, path)
}
func (s *Store) backup(ctx context.Context, path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(path); err == nil {
		return fmt.Errorf("backup destination already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".heimdall-backup-*.db")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err = file.Close(); err != nil {
		return err
	}
	if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", temp); err != nil {
		return err
	}
	// Exclusive link publication prevents overwriting a concurrent destination.
	if err = os.Link(temp, path); err != nil {
		return fmt.Errorf("backup publication (hard-link support required): %w", err)
	}
	return nil
}
