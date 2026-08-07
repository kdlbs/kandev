package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Prefix for dev-prod-db automatic snapshots. Distinct from the backend's
// "kandev-*" auto-snapshots and "manual-*" user snapshots so the families
// don't interfere with each other's retention policies.
const (
	backupPrefix = "dev-prod-db-"
	backupSuffix = ".db"
	maxBackups   = 5
)

// isProductionDb reports whether dbPath points to a non-dev database that
// should be backed up before running dev mode. Dev-isolated databases live
// under <repo>/.kandev-dev/ and are considered disposable.
func isProductionDb(dbPath string) bool {
	normalized := filepath.Clean(dbPath)
	segments := strings.Split(normalized, string(os.PathSeparator))
	for _, segment := range segments {
		if segment == devKandevDotdir {
			return false
		}
	}
	return true
}

// backupProductionDb copies the database at dbPath to
// <homeDir>/.kandev/data/backups/ before dev mode runs against it, keeping
// only the maxBackups newest snapshots. It returns the created backup path,
// or "" with no error when the source DB does not exist.
//
// Backups are always placed under <homeDir>/.kandev/data/backups/ even when
// dbPath points elsewhere. This matches the dev-prod-db default flow; custom
// KANDEV_DATABASE_PATH values are advanced usage where the user is responsible
// for backup location.
//
// homeDir and now are injectable for tests: an explicit timestamp makes
// back-to-back calls produce distinct filenames and mtimes without sleeping.
func backupProductionDb(dbPath, homeDir string, now time.Time) (string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return "", nil
	}

	backupDir := filepath.Join(homeDir, ".kandev", "data", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	name := backupPrefix + backupTimestamp(now) + backupSuffix
	dest := filepath.Join(backupDir, name)
	if err := copyFile(dbPath, dest); err != nil {
		return "", fmt.Errorf("copy database to backup: %w", err)
	}
	// Stamp both atime and mtime so pruning (which sorts by mtime) is
	// deterministic.
	if err := os.Chtimes(dest, now, now); err != nil {
		return "", fmt.Errorf("stamp backup mtime: %w", err)
	}

	pruneBackups(backupDir)

	return dest, nil
}

// backupTimestamp renders RFC3339 with separators stripped, matching the
// TypeScript launcher's toISOString().replace(/[:.]/g, "").
func backupTimestamp(now time.Time) string {
	raw := now.UTC().Format("2006-01-02T15:04:05.000Z")
	raw = strings.ReplaceAll(raw, ":", "")
	return strings.ReplaceAll(raw, ".", "")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// pruneBackups keeps only the maxBackups newest dev-prod-db-*.db files in dir
// by mtime, deleting older ones. Non-matching files are left untouched.
func pruneBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type backupFile struct {
		path  string
		mtime time.Time
	}
	var files []backupFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, backupPrefix) || !strings.HasSuffix(name, backupSuffix) {
			continue
		}
		fullPath := filepath.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, backupFile{path: fullPath, mtime: info.ModTime()})
	}
	if len(files) <= maxBackups {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	for _, f := range files[maxBackups:] {
		// Non-critical: don't fail the launch if one old backup can't be removed.
		_ = os.Remove(f.path)
	}
}
