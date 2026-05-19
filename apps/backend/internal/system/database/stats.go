// Package database serves the System -> Database page. It exposes read-only
// SQLite stats (path, size, WAL size, schema version, last-backup timestamp)
// and three maintenance operations: VACUUM, PRAGMA optimize, and Factory
// Reset. Long-running operations are tracked via the jobs.Tracker so the
// frontend can observe progress over the event bus.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
)

// Stats is the read-only SQLite-state payload returned to the frontend.
//
// LastBackupAt is a pointer so the JSON shape is `null` when no backup
// exists. Serialising a zero time.Time as "0001-01-01T00:00:00Z" would
// defeat the frontend's "Never" fallback in database-stats-card.tsx.
type Stats struct {
	Path          string     `json:"path"`
	SizeBytes     int64      `json:"size_bytes"`
	WALSizeBytes  int64      `json:"wal_size_bytes"`
	SchemaVersion string     `json:"schema_version"`
	LastBackupAt  *time.Time `json:"last_backup_at"`
}

// ResetDirs lists the on-disk directories factory-reset wipes. The Service
// only needs paths to call os.RemoveAll on — the construction site (cmd/kandev)
// fills these in from the resolved data/home dirs.
type ResetDirs struct {
	Worktrees string
	Repos     string
	Sessions  string
	Tasks     string
	QuickChat string
}

// Service is the maintenance + stats facade for the System -> Database page.
//
// OrchestratorShutdown and Restart are wired by cmd/kandev; tests pass
// no-op / observable callbacks. A nil callback is treated as a no-op so
// individual handlers remain testable in isolation.
type Service struct {
	pool    *db.Pool
	dataDir string
	dbPath  string
	dirs    ResetDirs
	jobs    *jobs.Tracker
	log     *logger.Logger

	// OrchestratorShutdown stops the orchestrator and active executions before
	// the factory-reset job runs. Wired by cmd/kandev. Tests pass a no-op.
	OrchestratorShutdown func()

	// Restart re-execs the backend process after a factory reset. Wired by
	// cmd/kandev (syscall.Exec); tests pass a callback that records the call.
	Restart func()
}

// NewService constructs a Service. dataDir is the resolved kandev data
// directory (the SQLite file lives at <dataDir>/kandev.db, backups under
// <dataDir>/backups). dirs lists the on-disk subtrees factory-reset wipes.
func NewService(pool *db.Pool, dataDir string, dirs ResetDirs, j *jobs.Tracker, log *logger.Logger) *Service {
	return &Service{
		pool:    pool,
		dataDir: dataDir,
		dbPath:  filepath.Join(dataDir, "kandev.db"),
		dirs:    dirs,
		jobs:    j,
		log:     log,
	}
}

// Path returns the absolute SQLite file path the service is bound to.
func (s *Service) Path() string { return s.dbPath }

// Stats returns the current SQLite stats. The size is computed from
// PRAGMA page_count * page_size (cheaper than os.Stat on hot writes and
// more accurate during a VACUUM that creates a sibling file).
func (s *Service) Stats() (Stats, error) {
	out := Stats{Path: s.dbPath}

	if s.pool != nil {
		size, err := readDBSize(s.pool.Reader())
		if err != nil {
			return Stats{}, err
		}
		out.SizeBytes = size

		version, err := readSchemaVersion(s.pool.Reader())
		if err != nil {
			return Stats{}, err
		}
		out.SchemaVersion = version
	}

	if wal, err := walSize(s.dbPath); err == nil {
		out.WALSizeBytes = wal
	}

	if last := lastBackupAt(filepath.Join(s.dataDir, "backups")); !last.IsZero() {
		out.LastBackupAt = &last
	}
	return out, nil
}

// readDBSize returns the database size in bytes via PRAGMA page_count *
// page_size. Both PRAGMAs return a single integer row.
func readDBSize(d interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}) (int64, error) {
	var pages, pageSize int64
	if err := d.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		return 0, fmt.Errorf("pragma page_count: %w", err)
	}
	if err := d.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("pragma page_size: %w", err)
	}
	return pages * pageSize, nil
}

// readSchemaVersion reads the binary version recorded by
// cmd/kandev/storage.go:recordSchemaVersion. Missing key returns "" with
// no error (fresh DB on first boot).
func readSchemaVersion(d interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}) (string, error) {
	var value string
	err := d.QueryRow(`SELECT value FROM kandev_meta WHERE key = ?`, "kandev_version").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read kandev_version: %w", err)
	}
	return value, nil
}

// walSize stats <dbPath>-wal and returns its size in bytes; a missing WAL
// file is treated as zero (the DB may have been just checkpointed).
func walSize(dbPath string) (int64, error) {
	info, err := os.Stat(dbPath + "-wal")
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

// lastBackupAt returns the mtime of the newest file in backupDir, or the
// zero time if the directory is missing/empty. A directory read error is
// treated as "no backups" — the Stats endpoint is best-effort.
func lastBackupAt(backupDir string) time.Time {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return time.Time{}
	}
	mtimes := make([]time.Time, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		mtimes = append(mtimes, fi.ModTime())
	}
	if len(mtimes) == 0 {
		return time.Time{}
	}
	sort.Slice(mtimes, func(i, j int) bool { return mtimes[i].After(mtimes[j]) })
	return mtimes[0]
}

// HandleStats returns a gin handler for GET /api/v1/system/database.
func HandleStats(s *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := s.Stats()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, stats)
	}
}

// startJobResponse is the canonical 202 payload for POST endpoints that
// spawn a background job.
type startJobResponse struct {
	JobID string `json:"job_id"`
}

func respondAccepted(c *gin.Context, jobID string) {
	c.JSON(http.StatusAccepted, startJobResponse{JobID: jobID})
}

// ctxOrBackground returns the context from the gin request when available,
// falling back to context.Background() for direct service callers.
func ctxOrBackground(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}
