package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

const (
	// bootStagingDirPrefix names the private directory createPreMigrationBackup
	// reserves before the snapshot copy writes the staged backup into it.
	bootStagingDirPrefix = ".kandev-backup-"

	// staleBootStagingAge is how long a boot staging directory must show no
	// modification before it counts as crash debris. The backups directory is
	// shared by every database file in the same directory, so this window is
	// what protects another process's backup that is still being written.
	staleBootStagingAge = 6 * time.Hour
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

// SnapshotSQLite copies the live database to path using VACUUM INTO,
// which produces a clean, defragmented snapshot including all WAL frames.
// Returns the size of the created file in bytes. Exported for use by
// internal/system/backups; the lowercase alias preserves call sites in
// this package.
func SnapshotSQLite(writer *sqlx.DB, path string) (int64, error) {
	return snapshotSQLite(writer, path)
}

// SnapshotSQLiteContext creates a cancellable SQLite snapshot.
func SnapshotSQLiteContext(ctx context.Context, writer *sqlx.DB, path string) (int64, error) {
	return snapshotSQLiteContext(ctx, writer, path)
}

// snapshotSQLite copies the live database to path using VACUUM INTO,
// which produces a clean, defragmented snapshot including all WAL frames.
// Returns the size of the created file in bytes.
func snapshotSQLite(writer *sqlx.DB, path string) (int64, error) {
	return snapshotSQLiteContext(context.Background(), writer, path)
}

func snapshotSQLiteContext(ctx context.Context, writer *sqlx.DB, path string) (int64, error) {
	_, statErr := os.Lstat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return 0, fmt.Errorf("inspect snapshot destination %s: %w", path, statErr)
	}
	pathExisted := statErr == nil
	installed := false
	defer func() {
		if !installed && !pathExisted {
			// VACUUM INTO writes the destination incrementally. A canceled or
			// failed statement can leave a partial SQLite file behind, which
			// must not be mistaken for a usable backup on retry.
			_ = os.Remove(path)
		}
	}()
	if _, err := writer.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return 0, fmt.Errorf("vacuum into %s: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("snapshot canceled after vacuum into %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat snapshot %s: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("snapshot canceled before installing %s: %w", path, err)
	}
	installed = true
	return info.Size(), nil
}

// SweepStaleBootStaging removes boot backup staging directories that a process
// left behind when it exited between MkdirTemp and the deferred cleanup. Such a
// directory otherwise keeps a whole database copy on disk.
//
// Liveness is the newest modification time of the directory and its direct
// entries, because a directory's own time does not change while the snapshot
// copy grows a file inside it. Only names with the boot staging prefix are matched,
// so published backups and other staging directories are never removed. A
// failure is logged and never returned: reclaiming debris must not fail boot
// or backup creation.
func SweepStaleBootStaging(backupDir string, log *logger.Logger) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && log != nil {
			log.Warn("persistence: failed to list boot backup staging directories",
				zap.String("path", backupDir), zap.Error(err))
		}
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), bootStagingDirPrefix) {
			continue
		}
		path := filepath.Join(backupDir, e.Name())
		newest, err := newestModTime(path)
		if err != nil {
			// A directory, or an entry in it, that vanished while being
			// inspected was removed by its owner or another sweep and needs
			// nothing from this one.
			if !errors.Is(err, os.ErrNotExist) && log != nil {
				log.Warn("persistence: failed to inspect boot backup staging directory",
					zap.String("path", path), zap.Error(err))
			}
			continue
		}
		if time.Since(newest) < staleBootStagingAge {
			continue
		}
		if err := os.RemoveAll(path); err != nil && log != nil {
			log.Warn("persistence: failed to reclaim stale boot backup staging directory",
				zap.String("path", path), zap.Error(err))
		}
	}
}

// newestModTime returns the latest modification time among dir and its direct
// entries. It stats each path rather than trusting directory-listing metadata.
func newestModTime(dir string) (time.Time, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return time.Time{}, err
	}
	newest := info.ModTime()
	children, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, err
	}
	for _, child := range children {
		childInfo, err := os.Lstat(filepath.Join(dir, child.Name()))
		if err != nil {
			return time.Time{}, err
		}
		if childInfo.ModTime().After(newest) {
			newest = childInfo.ModTime()
		}
	}
	return newest, nil
}

// CopySQLiteSnapshot creates a consistent page copy without physical compaction.
func CopySQLiteSnapshot(source *sqlx.DB, path string) (int64, error) {
	return CopySQLiteSnapshotContext(context.Background(), source, path)
}

// CopySQLiteSnapshotContext copies in bounded increments. The caller verifies
// integrity before publishing the completed snapshot.
func CopySQLiteSnapshotContext(ctx context.Context, source *sqlx.DB, path string) (int64, error) {
	if err := db.BackupSQLite(ctx, source.DB, path); err != nil {
		return 0, fmt.Errorf("copy SQLite snapshot: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat snapshot: %w", err)
	}
	return info.Size(), nil
}

// PruneBackups is the exported alias of pruneBackups, intended for callers
// outside this package (e.g. internal/system/backups retention tests) that
// need to verify the auto-snapshot retention policy without copy-pasting
// the implementation.
func PruneBackups(dir string, keep int) error {
	return pruneBackups(dir, keep)
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
