package maintenance

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/persistence"
)

// defaultKeepMaintenanceBackups bounds how many pre-execution maintenance
// backups accumulate in the backups directory over repeated runs, matching
// this plan's "bound SQLite storage growth" goal rather than adding another
// unbounded-growth source of its own.
const defaultKeepMaintenanceBackups = 5

// createVerifiedBackup takes a fresh VACUUM INTO snapshot into
// backupDir/kandev-pre-maintenance-<timestamp>.db, verifies it with
// PRAGMA integrity_check, and prunes old maintenance backups beyond
// defaultKeepMaintenanceBackups. Returns the backup path on success. On any
// failure, no destructive retention/compaction step is permitted to run -
// callers must treat a non-nil error here as an unconditional abort.
func createVerifiedBackup(writer *sqlx.DB, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: create backup dir %s: %v", ErrBackupUnverified, backupDir, err)
	}
	path, err := uniqueBackupPath(backupDir)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackupUnverified, err)
	}

	if _, err := persistence.SnapshotSQLite(writer, path); err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackupUnverified, err)
	}
	if err := verifySQLiteIntegrity(path); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("%w: backup failed integrity check: %v", ErrBackupUnverified, err)
	}

	// Best-effort: a prune failure never invalidates the backup just
	// verified above, so it is logged by the caller rather than returned.
	_ = persistence.PruneBackups(backupDir, defaultKeepMaintenanceBackups)

	return path, nil
}

// uniqueBackupPath picks a backup file name that does not yet exist.
// VACUUM INTO refuses to write over an existing file, and a second-level
// timestamp alone can collide when two runs happen inside the same wall
// clock second (e.g. immediate back-to-back --execute invocations, or a
// test); this appends nanosecond precision and, defensively, a numeric
// suffix if that still collides.
func uniqueBackupPath(backupDir string) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405.000000000Z")
	base := fmt.Sprintf("kandev-pre-maintenance-%s", ts)
	for attempt := 0; attempt < 100; attempt++ {
		name := base + ".db"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.db", base, attempt)
		}
		path := filepath.Join(backupDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, nil
		}
	}
	return "", fmt.Errorf("could not find an available backup filename under %s", backupDir)
}
