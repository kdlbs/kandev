package maintenance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/metrics"
)

// compactionSafetyMarginBytes is added on top of the live database's
// current size when checking free disk space for a staged compaction copy,
// so a near-exact-size compacted output still leaves headroom for the
// filesystem's own bookkeeping.
const compactionSafetyMarginBytes = 10 * 1024 * 1024 // 10 MiB

// CompactionResult reports the outcome of a successful staged compaction:
// where the pre-compaction database was preserved, and the size delta.
type CompactionResult struct {
	// RollbackPath is the pre-compaction database file, renamed aside
	// rather than deleted. Restoring it is: stop kandev (if running),
	// then move RollbackPath back over the live database path.
	RollbackPath    string
	SizeBeforeBytes int64
	SizeAfterBytes  int64
}

// diskUsageFunc matches internal/system/metrics.DiskUsage's signature so
// tests can substitute a deterministic stand-in without touching the real
// filesystem's free space.
type diskUsageFunc func(ctx context.Context, path string) (metrics.DiskCapacity, error)

// snapshotFunc matches internal/persistence.SnapshotSQLite's signature so
// tests can inject a VACUUM INTO failure, or a "succeeded but produced
// corrupt output" scenario, without depending on actually corrupting a
// SQLite file on disk.
type snapshotFunc func(writer *sqlx.DB, path string) (int64, error)

// compact stages a fresh VACUUM INTO copy of the live database, verifies it
// with PRAGMA integrity_check, and only then atomically replaces the live
// file - retaining the pre-compaction file as a rollback artifact rather
// than deleting it. closeConnections must close every open handle to
// dbPath (writer and reader pools) before compact attempts the swap: open
// handles can silently block or corrupt a rename on some platforms.
//
// On any failure before the swap (insufficient disk space, VACUUM INTO
// failure, integrity-check failure), the live database at dbPath is left
// completely untouched and closeConnections is not called - the caller's
// existing connections remain valid.
func compact(ctx context.Context, writer *sqlx.DB, closeConnections func() error, dbPath string, diskUsage diskUsageFunc, snapshot snapshotFunc) (CompactionResult, error) {
	info, err := os.Stat(dbPath)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("stat live database: %w", err)
	}
	sizeBefore := info.Size()

	capacity, err := diskUsage(ctx, filepath.Dir(dbPath))
	if err != nil {
		return CompactionResult{}, fmt.Errorf("check staging disk space: %w", err)
	}
	required := uint64(sizeBefore) + compactionSafetyMarginBytes
	if capacity.AvailableBytes < required {
		return CompactionResult{}, fmt.Errorf("%w: have %d bytes free, need at least %d",
			ErrInsufficientDiskSpace, capacity.AvailableBytes, required)
	}

	ts := time.Now().UTC().Format("20060102T150405.000000000Z")
	stagedPath := dbPath + ".maintenance-staging-" + ts + ".db"
	sizeAfter, err := snapshot(writer, stagedPath)
	if err != nil {
		_ = os.Remove(stagedPath)
		return CompactionResult{}, fmt.Errorf("stage compacted copy: %w", err)
	}

	if err := verifySQLiteIntegrity(stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		return CompactionResult{}, err
	}

	// Every handle to dbPath must close before the swap: an open connection
	// can block (Windows) or silently keep writing to the old inode
	// (POSIX, if anything re-opened it) through the rename.
	if err := closeConnections(); err != nil {
		_ = os.Remove(stagedPath)
		return CompactionResult{}, fmt.Errorf("close database connections before compaction swap: %w", err)
	}

	rollbackPath := dbPath + ".pre-compact-" + ts + ".bak"
	if err := os.Rename(dbPath, rollbackPath); err != nil {
		_ = os.Remove(stagedPath)
		return CompactionResult{}, fmt.Errorf("move live database aside for rollback: %w", err)
	}
	// A VACUUM INTO target has no WAL/SHM sidecars of its own, but the live
	// database being replaced may - move them alongside the rollback file
	// (rather than leave them stranded at the old name) so the rollback
	// artifact is self-consistent if ever restored.
	moveSidecarIfExists(dbPath+"-wal", rollbackPath+"-wal")
	moveSidecarIfExists(dbPath+"-shm", rollbackPath+"-shm")

	if err := os.Rename(stagedPath, dbPath); err != nil {
		// Best-effort restore of the original so a failed swap doesn't
		// leave the install without any database at dbPath.
		_ = os.Rename(rollbackPath, dbPath)
		return CompactionResult{}, fmt.Errorf("swap staged database into place: %w", err)
	}
	fsyncDir(filepath.Dir(dbPath))

	return CompactionResult{RollbackPath: rollbackPath, SizeBeforeBytes: sizeBefore, SizeAfterBytes: sizeAfter}, nil
}

// verifySQLiteIntegrity opens path read-only and runs PRAGMA
// integrity_check, treating anything other than a single "ok" row as a
// failure. Used both for the staged compaction copy and (via
// verifyBackup) the pre-execution backup.
func verifySQLiteIntegrity(path string) error {
	conn, err := db.OpenSQLiteReader(path)
	if err != nil {
		return fmt.Errorf("open %s for integrity check: %w", path, err)
	}
	defer func() { _ = conn.Close() }()

	rows, err := conn.Query(`PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("%w: run integrity_check on %s: %v", ErrIntegrityCheckFailed, path, err)
	}
	defer func() { _ = rows.Close() }()

	var messages []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return fmt.Errorf("scan integrity_check row: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate integrity_check rows: %w", err)
	}
	if len(messages) == 1 && messages[0] == "ok" {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrIntegrityCheckFailed, strings.Join(messages, "; "))
}

// moveSidecarIfExists renames a WAL/SHM sidecar file if present, silently
// doing nothing if it doesn't exist (the common case for a WAL-checkpointed
// database at rest under exclusive access).
func moveSidecarIfExists(from, to string) {
	if _, err := os.Stat(from); err != nil {
		return
	}
	_ = os.Rename(from, to)
}

// fsyncDir best-effort fsyncs a directory so the preceding renames are
// durable across a crash immediately after compaction. Failures are not
// fatal - the renames themselves already succeeded.
func fsyncDir(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_ = f.Sync()
}
