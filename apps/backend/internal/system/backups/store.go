package backups

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

// Snapshot is the public representation of a backup file on disk.
// Snapshot is the metadata one backup file exposes over the API. It
// deliberately carries no on-disk path: GET /backups is readable by any
// authenticated user (only the mutating and downloading routes are admin
// only), and the absolute path would disclose the install's data directory to
// all of them. Callers name a snapshot by Name; the service resolves it to a
// path itself via resolveSnapshotPath.
type Snapshot struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	ModTime   time.Time `json:"mtime"`
	Kind      string    `json:"kind"` // "auto" | "manual"
}

// RestoreConfirmToken is the literal string the client must send in the
// confirm field for Restore to proceed. Anything else is rejected with a
// 400 by the handler.
const RestoreConfirmToken = "RESTORE"

// errRestoreConfirm is exported so handlers can map it to HTTP 400.
var errRestoreConfirm = errors.New("restore requires confirm=RESTORE")

var errRestoreUnsupported = errors.New("restore is only supported for SQLite")

// ErrInvalidName is returned for filenames that contain path separators,
// "..", or absolute prefixes.
var ErrInvalidName = errors.New("invalid backup name")

// Service owns access to the backups directory beside the configured database
// file and exposes the list/create/restore/delete/download API.
//
// Restore intentionally does not attempt to re-exec the backend: the staged
// DB file is written in place and the user is told (via the frontend dialog)
// to quit and relaunch Kandev to load the restored data. Before replacing the
// file, restore quiesces scheduling, active executions, and database-backed
// workers, checkpoints and closes the SQLite pool, then replaces the main
// database and sidecars with rollback protection. The previous syscall.Exec
// approach was brittle under desktop launchers and `make dev` watchers, and
// left the web UI disconnected from a fresh backend.
type Service struct {
	databasePath string
	pool         *db.Pool
	jobs         *jobs.Tracker
	log          *logger.Logger

	// RestoreQuiesce stops scheduling, active executions, and database-backed
	// workers before restore closes the shared database pool. Wired by the
	// backend composition root; tests may leave it nil.
	RestoreQuiesce func() error

	// OrchestratorShutdown is the legacy reset/restore hook. Restore uses it
	// only when RestoreQuiesce is not wired.
	OrchestratorShutdown func()

	// failWritesForTest, when true, causes Restore's staged-write step to
	// fail before the configured database file is touched. Only set by tests.
	failWritesForTest bool
}

// NewService constructs a Service. The backups directory beside databasePath
// is created lazily by methods that need it.
func NewService(databasePath string, pool *db.Pool, tracker *jobs.Tracker, log *logger.Logger) *Service {
	return &Service{
		databasePath: databasePath,
		pool:         pool,
		jobs:         tracker,
		log:          log,
	}
}

// backupsDir returns the snapshots directory beside the configured database.
func (s *Service) backupsDir() string {
	return filepath.Join(filepath.Dir(s.databasePath), "backups")
}

// ensureBackupsDir mkdirs the backups directory.
func (s *Service) ensureBackupsDir() error {
	return os.MkdirAll(s.backupsDir(), 0o755)
}

// List enumerates the snapshots in the sibling backups directory, classifying each
// .db file as auto or manual. Non-.db files and unrecognised prefixes are
// skipped silently. Always returns a non-nil slice.
func (s *Service) List() ([]Snapshot, error) {
	out := make([]Snapshot, 0)
	dir := s.backupsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("read backups dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		kind := classify(e.Name())
		if kind == "" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Snapshot{
			Name:      e.Name(),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().UTC(),
			Kind:      kind,
		})
	}
	return out, nil
}

// Create starts a job that writes a manual snapshot via VACUUM INTO and
// returns the job ID immediately.
func (s *Service) Create(ctx context.Context) string {
	return s.jobs.Start(ctx, "backup-create", func(ctx context.Context) (map[string]interface{}, error) {
		return s.runCreate(ctx)
	})
}

func (s *Service) runCreate(_ context.Context) (map[string]interface{}, error) {
	if err := s.ensureBackupsDir(); err != nil {
		return nil, err
	}
	// A .tmp sidecar is normally renamed away on success and removed on the
	// error paths below, but a crash between SnapshotSQLite and os.Rename
	// leaves one behind. classify() hides it from List()/Delete(), so it can
	// never be cleaned up through the UI and would leak disk (up to the size
	// of the live DB) indefinitely. Sweep any leftovers before writing a new
	// one so crash debris is reclaimed on the next manual backup.
	s.sweepStaleTmpFiles()
	// Nanosecond precision so double-clicks or concurrent /backups POSTs do
	// not collide on the same filename and silently overwrite one job's
	// snapshot with another.
	name := fmt.Sprintf("%s%d%s", manualPrefix, time.Now().UTC().UnixNano(), dbSuffix)
	path := filepath.Join(s.backupsDir(), name)
	// VACUUM INTO writes the multi-hundred-MB snapshot incrementally. If we
	// wrote directly to the final "manual-*.db" name, a concurrent List()
	// (the UI refetches immediately after the 202) would os.Stat a
	// half-written file and report a truncated size. Write to a ".tmp"
	// sidecar first — classify() ignores non-.db suffixes so it is never
	// listed — then atomically rename it into place at its full size.
	tmpPath := path + tmpSuffix
	size, err := persistence.SnapshotSQLite(s.pool.Writer(), tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("rename snapshot into place: %w", err)
	}
	return map[string]interface{}{
		"name":       name,
		"path":       path,
		"size_bytes": size,
	}, nil
}

// staleTmpAge is how old a ".tmp" sidecar must be before the sweep reclaims
// it. Concurrent backup-create jobs are not serialized (jobs.Tracker runs each
// in its own goroutine), so a just-created sidecar may belong to another
// in-flight VACUUM INTO. Only files older than this are treated as crash debris,
// which keeps concurrent creates safe while still reclaiming leaked files. The
// threshold is far above any realistic VACUUM INTO duration.
const staleTmpAge = 10 * time.Minute

// sweepStaleTmpFiles removes leftover ".tmp" VACUUM INTO sidecars from a
// previously crashed runCreate, skipping any modified within staleTmpAge so a
// concurrent create's in-progress sidecar is never deleted out from under it.
// Best-effort: read/stat/remove failures are logged and ignored so a stale
// file never blocks a fresh backup.
func (s *Service) sweepStaleTmpFiles() {
	entries, err := os.ReadDir(s.backupsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), tmpSuffix) {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < staleTmpAge {
			continue
		}
		p := filepath.Join(s.backupsDir(), e.Name())
		if err := os.Remove(p); err != nil && s.log != nil {
			s.log.Warn("backups: failed to remove stale tmp snapshot", zap.String("path", p), zap.Error(err))
		}
	}
}

// Restore validates the confirm token, then runs the restore as a job.
// Returns the job ID on success, or an error if the token is wrong.
func (s *Service) Restore(ctx context.Context, name, confirm string) (string, error) {
	if confirm != RestoreConfirmToken {
		return "", errRestoreConfirm
	}
	if err := s.ensureSQLiteRestore(); err != nil {
		return "", err
	}
	abs, err := s.resolveSnapshotPath(name)
	if err != nil {
		return "", err
	}
	id := s.jobs.Start(ctx, "restore", func(ctx context.Context) (map[string]interface{}, error) {
		return s.runRestore(ctx, abs)
	})
	return id, nil
}

func (s *Service) runRestore(_ context.Context, snapshotPath string) (map[string]interface{}, error) {
	if _, err := os.Stat(snapshotPath); err != nil {
		return nil, fmt.Errorf("snapshot not found: %w", err)
	}
	stagedPath := s.databasePath + ".new"
	if err := s.writeStagedRestore(snapshotPath, stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		return nil, err
	}
	if s.RestoreQuiesce != nil {
		if err := s.RestoreQuiesce(); err != nil {
			_ = os.Remove(stagedPath)
			return nil, fmt.Errorf("quiesce database for restore: %w", err)
		}
	} else if s.OrchestratorShutdown != nil {
		s.OrchestratorShutdown()
	}
	if err := s.quiesceDatabase(); err != nil {
		_ = os.Remove(stagedPath)
		return nil, err
	}
	if err := s.replaceDatabase(stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		return nil, err
	}
	// Intentionally no auto-restart. The frontend dialog reads
	// restart_required from the job result and prompts the user to quit and
	// relaunch the app so the new DB file is loaded fresh.
	return map[string]interface{}{
		"restored_from":    filepath.Base(snapshotPath),
		"restart_required": true,
	}, nil
}

func (s *Service) ensureSQLiteRestore() error {
	if s.pool == nil || s.pool.Writer() == nil {
		return nil
	}
	driver := s.pool.Writer().DriverName()
	if driver != dialect.SQLite3 {
		return fmt.Errorf("%w: %s driver", errRestoreUnsupported, driver)
	}
	return nil
}

// quiesceDatabase flushes pending SQLite WAL frames and closes the shared
// pool before the configured database file is replaced. Closing the pool is
// intentional: the frontend receives restart_required and must relaunch the
// backend before it accepts database-backed work again.
func (s *Service) quiesceDatabase() error {
	if err := s.ensureSQLiteRestore(); err != nil {
		return err
	}
	if s.pool == nil || s.pool.Writer() == nil {
		return nil
	}
	writer := s.pool.Writer()
	var busy, logFrames, checkpointed int
	if err := writer.QueryRowx("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return fmt.Errorf("checkpoint sqlite WAL: %w", err)
	}
	if busy != 0 {
		return fmt.Errorf("checkpoint sqlite WAL: busy (%d frames remain)", logFrames-checkpointed)
	}
	if err := s.pool.Close(); err != nil {
		return fmt.Errorf("close database pool: %w", err)
	}
	return nil
}

type restoreOriginalFile struct {
	livePath       string
	quarantinePath string
}

// replaceDatabase quarantines the live database and its sidecars before
// installing the staged file. If installation fails, every quarantined file
// is moved back before the error is returned. The quarantine is in the same
// directory so each rename is atomic on the same filesystem.
func (s *Service) replaceDatabase(stagedPath string) error {
	return s.replaceDatabaseWith(stagedPath, os.Rename)
}

func (s *Service) replaceDatabaseWith(stagedPath string, rename func(string, string) error) error {
	quarantineBase, err := createRestoreQuarantine(filepath.Dir(s.databasePath), filepath.Base(s.databasePath))
	if err != nil {
		return fmt.Errorf("create restore quarantine: %w", err)
	}

	originals := []restoreOriginalFile{
		{livePath: s.databasePath, quarantinePath: quarantineBase},
		{livePath: s.databasePath + "-wal", quarantinePath: quarantineBase + "-wal"},
		{livePath: s.databasePath + "-shm", quarantinePath: quarantineBase + "-shm"},
	}
	moved := make([]restoreOriginalFile, 0, len(originals))
	rollback := func(cause error) error {
		var rollbackErrs []error
		for i := len(moved) - 1; i >= 0; i-- {
			original := moved[i]
			if err := rename(original.quarantinePath, original.livePath); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("restore %s: %w", original.livePath, err))
			}
		}
		return errors.Join(cause, errors.Join(rollbackErrs...))
	}

	for _, original := range originals {
		if _, err := os.Lstat(original.livePath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return rollback(fmt.Errorf("inspect %s: %w", original.livePath, err))
		}
		if err := rename(original.livePath, original.quarantinePath); err != nil {
			return rollback(fmt.Errorf("quarantine %s: %w", original.livePath, err))
		}
		moved = append(moved, original)
	}

	if err := rename(stagedPath, s.databasePath); err != nil {
		return rollback(fmt.Errorf("atomic rename failed: %w", err))
	}

	for _, original := range moved {
		if err := os.Remove(original.quarantinePath); err != nil && !errors.Is(err, os.ErrNotExist) && s.log != nil {
			s.log.Warn("backups: failed to remove quarantined database file",
				zap.String("path", original.quarantinePath), zap.Error(err))
		}
	}
	return nil
}

func createRestoreQuarantine(dir, base string) (string, error) {
	f, err := os.CreateTemp(dir, "."+base+".restore-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

// writeStagedRestore copies snapshotPath to stagedPath. Honors
// failWritesForTest so tests can simulate a mid-restore failure that
// leaves the original DB untouched.
func (s *Service) writeStagedRestore(snapshotPath, stagedPath string) error {
	if s.failWritesForTest {
		return errors.New("simulated write failure")
	}
	src, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(stagedPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create staged db: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("copy snapshot: %w", err)
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return fmt.Errorf("sync staged db: %w", err)
	}
	return dst.Close()
}

// Delete removes a snapshot file. Refuses to delete pre-reset recovery
// snapshots.
func (s *Service) Delete(name string) error {
	abs, err := s.resolveSnapshotPath(name)
	if err != nil {
		return err
	}
	if isPreResetSnapshot(name) {
		return fmt.Errorf("cannot delete pre-reset recovery snapshot %q", name)
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

// OpenForDownload validates the name and returns an open *os.File plus its
// size for the handler to stream. The caller owns closing the file.
func (s *Service) OpenForDownload(name string) (*os.File, int64, error) {
	abs, err := s.resolveSnapshotPath(name)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, 0, fmt.Errorf("open snapshot: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("stat snapshot: %w", err)
	}
	return f, info.Size(), nil
}

// resolveSnapshotPath validates that name is a bare filename (no
// separators, no "..", no absolute prefix), confirms it matches a
// recognised snapshot prefix (so unrelated files dropped into the backups
// directory cannot be restored/downloaded/deleted), and that it resolves
// inside the backups directory. Returns the joined path.
func (s *Service) resolveSnapshotPath(name string) (string, error) {
	if name == "" || name == "." || name == ".." {
		return "", ErrInvalidName
	}
	if strings.ContainsAny(name, "/\\") {
		return "", ErrInvalidName
	}
	if strings.Contains(name, "..") {
		return "", ErrInvalidName
	}
	if filepath.IsAbs(name) {
		return "", ErrInvalidName
	}
	// Defensive: filepath.Clean strips any tricks before we join.
	clean := filepath.Clean(name)
	if clean != name {
		return "", ErrInvalidName
	}
	// Allow-list by prefix + suffix. classify() returns "" for anything
	// that isn't a manual/auto snapshot; pre-reset recovery snapshots use
	// the auto prefix and are accepted here too (Delete blocks them later).
	if classify(name) == "" && !isPreResetSnapshot(name) {
		return "", ErrInvalidName
	}
	abs := filepath.Join(s.backupsDir(), clean)
	// Confirm abs is still inside backupsDir.
	rel, err := filepath.Rel(s.backupsDir(), abs)
	if err != nil || strings.HasPrefix(rel, "..") || strings.ContainsAny(rel, "/\\") {
		return "", ErrInvalidName
	}
	return abs, nil
}
