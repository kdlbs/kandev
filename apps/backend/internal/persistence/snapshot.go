package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// snapshotPath returns the absolute path for a new backup file.
// fromVersion defaults to "pre-meta" when empty.
func snapshotPath(backupDir, fromVersion string) string {
	v := fromVersion
	if v == "" {
		v = "pre-meta"
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	name := fmt.Sprintf("kandev-%s-%s.db", v, ts)
	return filepath.Join(backupDir, name)
}

// snapshotSQLite copies the live database to path using VACUUM INTO,
// which produces a clean, defragmented snapshot including all WAL frames.
// Returns the size of the created file in bytes.
func snapshotSQLite(writer *sqlx.DB, path string) (int64, error) {
	if _, err := writer.Exec(`VACUUM INTO ?`, path); err != nil {
		return 0, fmt.Errorf("vacuum into %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat snapshot %s: %w", path, err)
	}
	return info.Size(), nil
}

// pruneBackups retains only the keep newest backup files (sorted by mtime)
// inside dir, deleting everything older. Files are matched by the
// "kandev-*.db" glob pattern. Non-matching files in the directory are not
// touched. Errors on individual deletes are silently ignored so a stale
// permission issue on one file does not block the rest.
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read backup dir %s: %w", dir, err)
	}

	type fileInfo struct {
		path  string
		mtime time.Time
	}
	var files []fileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasPrefix(e.Name(), "kandev-") || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:  filepath.Join(dir, e.Name()),
			mtime: info.ModTime(),
		})
	}

	if len(files) <= keep {
		return nil
	}

	// Sort newest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.After(files[j].mtime)
	})

	for _, f := range files[keep:] {
		_ = os.Remove(f.path)
	}
	return nil
}
