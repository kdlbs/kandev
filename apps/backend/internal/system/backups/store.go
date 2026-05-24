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

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

// Snapshot is the public representation of a backup file on disk.
type Snapshot struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
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

// ErrInvalidName is returned for filenames that contain path separators,
// "..", or absolute prefixes.
var ErrInvalidName = errors.New("invalid backup name")

// Service owns access to the <data-dir>/backups directory and exposes the
// list/create/restore/delete/download API.
//
// Restore intentionally does not attempt to re-exec the backend: the staged
// DB file is written in place and the user is told (via the frontend dialog)
// to quit and relaunch Kandev to load the restored data. The previous
// syscall.Exec approach was brittle under desktop launchers and `make dev`
// watchers, and left the web UI disconnected from a fresh backend.
type Service struct {
	dataDir string
	pool    *db.Pool
	jobs    *jobs.Tracker
	log     *logger.Logger

	// failWritesForTest, when true, causes Restore's staged-write step to
	// fail before kandev.db is touched. Only set by tests.
	failWritesForTest bool
}

// NewService constructs a Service. The backups directory under dataDir is
// created lazily by methods that need it.
func NewService(dataDir string, pool *db.Pool, tracker *jobs.Tracker, log *logger.Logger) *Service {
	return &Service{
		dataDir: dataDir,
		pool:    pool,
		jobs:    tracker,
		log:     log,
	}
}

// backupsDir returns the absolute path to the snapshots directory.
func (s *Service) backupsDir() string {
	return filepath.Join(s.dataDir, "backups")
}

// dbPath returns the absolute path to the live SQLite database file.
func (s *Service) dbPath() string {
	return filepath.Join(s.dataDir, "kandev.db")
}

// ensureBackupsDir mkdirs the backups directory.
func (s *Service) ensureBackupsDir() error {
	return os.MkdirAll(s.backupsDir(), 0o755)
}

// List enumerates the snapshots in <data-dir>/backups, classifying each
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
			Path:      filepath.Join(dir, e.Name()),
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
	name := fmt.Sprintf("%s%d%s", manualPrefix, time.Now().UTC().Unix(), dbSuffix)
	path := filepath.Join(s.backupsDir(), name)
	size, err := persistence.SnapshotSQLite(s.pool.Writer(), path)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"name":       name,
		"path":       path,
		"size_bytes": size,
	}, nil
}

// Restore validates the confirm token, then runs the restore as a job.
// Returns the job ID on success, or an error if the token is wrong.
func (s *Service) Restore(ctx context.Context, name, confirm string) (string, error) {
	if confirm != RestoreConfirmToken {
		return "", errRestoreConfirm
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
	stagedPath := s.dbPath() + ".new"
	if err := s.writeStagedRestore(snapshotPath, stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		return nil, err
	}
	if err := os.Rename(stagedPath, s.dbPath()); err != nil {
		_ = os.Remove(stagedPath)
		return nil, fmt.Errorf("atomic rename failed: %w", err)
	}
	// Intentionally no auto-restart. The frontend dialog reads
	// restart_required from the job result and prompts the user to quit and
	// relaunch the app so the new DB file is loaded fresh.
	return map[string]interface{}{
		"restored_from":    filepath.Base(snapshotPath),
		"restart_required": true,
	}, nil
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
// separators, no "..", no absolute prefix), confirms it lives inside the
// backups directory after path resolution, and returns the absolute path.
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
	abs := filepath.Join(s.backupsDir(), clean)
	// Confirm abs is still inside backupsDir.
	rel, err := filepath.Rel(s.backupsDir(), abs)
	if err != nil || strings.HasPrefix(rel, "..") || strings.ContainsAny(rel, "/\\") {
		return "", ErrInvalidName
	}
	return abs, nil
}
