package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// resetConfirmToken is the literal value the client must POST as
// {"confirm": "..."} to authorise a factory reset. Anything else is
// rejected with a 400 / typed error.
const resetConfirmToken = "RESET"

// ErrResetNotConfirmed is returned when the confirm value supplied to
// FactoryReset does not equal the resetConfirmToken sentinel.
var ErrResetNotConfirmed = errors.New("factory reset requires confirm=\"RESET\"")

// FactoryReset orchestrates a destructive wipe of the kandev install. The
// caller must pass confirm == "RESET" — anything else returns
// ErrResetNotConfirmed without starting a job. On success the job ID is
// returned immediately; the heavy work runs asynchronously via the jobs
// tracker.
//
// The job:
//  1. Calls s.OrchestratorShutdown (if set) to stop running executions.
//  2. Snapshots the live DB to <dataDir>/backups/kandev-pre-reset-<unix>.db.
//  3. Drops every user table from the SQLite schema (kandev_meta is kept).
//  4. os.RemoveAll on worktrees/repos/sessions/tasks/quick-chat subdirs.
//  5. Calls s.Restart (if set) to re-exec the process.
//
// The result map exposes {"snapshot_path", "tables_dropped"} on success.
func (s *Service) FactoryReset(ctx context.Context, confirm string) (string, error) {
	if confirm != resetConfirmToken {
		return "", ErrResetNotConfirmed
	}
	return s.jobs.Start(ctx, "factory-reset", func(jobCtx context.Context) (map[string]interface{}, error) {
		return s.runFactoryReset(jobCtx)
	}), nil
}

func (s *Service) runFactoryReset(_ context.Context) (map[string]interface{}, error) {
	if s.OrchestratorShutdown != nil {
		s.OrchestratorShutdown()
	}

	snapshotPath, err := s.createPreResetSnapshot()
	if err != nil {
		return nil, fmt.Errorf("pre-reset snapshot: %w", err)
	}

	dropped, err := dropUserTables(s.pool.Writer())
	if err != nil {
		return nil, fmt.Errorf("drop tables: %w", err)
	}

	if err := s.wipeSubdirs(); err != nil {
		return nil, fmt.Errorf("wipe subdirs: %w", err)
	}

	if s.Restart != nil {
		// Restart hands control to the new process; in tests the callback
		// just records the invocation.
		s.Restart()
	}

	return map[string]interface{}{
		"snapshot_path":  snapshotPath,
		"tables_dropped": dropped,
	}, nil
}

// createPreResetSnapshot performs VACUUM INTO into a stable, time-stamped
// path inside the backups directory. The "kandev-" prefix is kept so the
// existing backup retention regex continues to match the file.
func (s *Service) createPreResetSnapshot() (string, error) {
	if s.pool == nil {
		return "", fmt.Errorf("no database pool")
	}
	backupDir := filepath.Join(s.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir backups: %w", err)
	}
	name := fmt.Sprintf("kandev-pre-reset-%s.db", strconv.FormatInt(time.Now().UTC().Unix(), 10))
	path := filepath.Join(backupDir, name)
	if _, err := s.pool.Writer().Exec(`VACUUM INTO ?`, path); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", path, err)
	}
	if s.log != nil {
		s.log.Info("pre-reset snapshot created", zap.String("path", path))
	}
	return path, nil
}

// dropUserTables drops every row in sqlite_master that is a non-internal
// table other than kandev_meta. Returns the count of tables dropped.
func dropUserTables(writer *sqlx.DB) (int, error) {
	rows, err := writer.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name != 'kandev_meta'
	`)
	if err != nil {
		return 0, fmt.Errorf("enumerate user tables: %w", err)
	}

	var names []string
	for rows.Next() {
		var n string
		if scanErr := rows.Scan(&n); scanErr != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("scan table name: %w", scanErr)
		}
		names = append(names, n)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return 0, fmt.Errorf("close rows: %w", closeErr)
	}

	// FK enforcement is on for the writer pool — defer constraint checks so
	// drop order doesn't matter. SQLite ignores PRAGMA foreign_keys inside a
	// transaction, so toggle it at the connection level.
	if _, err := writer.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return 0, fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() { _, _ = writer.Exec("PRAGMA foreign_keys = ON") }()

	for _, n := range names {
		if _, err := writer.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q", n)); err != nil {
			return 0, fmt.Errorf("drop table %s: %w", n, err)
		}
	}
	return len(names), nil
}

// wipeSubdirs removes the worktrees/repos/sessions/tasks/quick-chat
// subdirs configured at construction. Empty paths are skipped so partial
// wiring is harmless in tests.
func (s *Service) wipeSubdirs() error {
	targets := []string{
		s.dirs.Worktrees,
		s.dirs.Repos,
		s.dirs.Sessions,
		s.dirs.Tasks,
		s.dirs.QuickChat,
	}
	for _, t := range targets {
		if t == "" {
			continue
		}
		if err := os.RemoveAll(t); err != nil {
			return fmt.Errorf("remove %s: %w", t, err)
		}
	}
	return nil
}
