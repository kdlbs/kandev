package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

type sqliteSelectionOutcome string

const (
	sqliteSelectionFresh         sqliteSelectionOutcome = "fresh"
	sqliteSelectionExisting      sqliteSelectionOutcome = "existing"
	sqliteSelectionLegacyAdopted sqliteSelectionOutcome = "legacy_adopted"
	sqliteSelectionConflict      sqliteSelectionOutcome = "conflict"
)

type sqliteSelection struct {
	path              string
	existedBeforeOpen bool
	outcome           sqliteSelectionOutcome
}

type sqliteCandidate struct {
	path       string
	exists     bool
	tasksTable bool
	taskCount  int64
}

// SQLiteDatabaseSelectionConflictError identifies the two default candidates
// that cannot be selected safely without operator direction.
type SQLiteDatabaseSelectionConflictError struct {
	CurrentPath      string
	LegacyPath       string
	CurrentTaskCount int64
	LegacyTaskCount  int64
}

func (e *SQLiteDatabaseSelectionConflictError) Error() string {
	return fmt.Sprintf(
		"sqlite database selection conflict: current default %q has %d tasks, but legacy default %q has %d tasks; startup stopped without modifying either database, preserve both and select one explicitly with database.path or KANDEV_DATABASE_PATH",
		e.CurrentPath,
		e.CurrentTaskCount,
		e.LegacyPath,
		e.LegacyTaskCount,
	)
}

func selectSQLiteDatabase(cfg *config.Config, log *logger.Logger) (sqliteSelection, error) {
	if databasePathIsExplicit(cfg) {
		path := strings.TrimSpace(cfg.Database.Path)
		selection := sqliteSelection{
			path:    path,
			outcome: sqliteSelectionExisting,
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			selection.outcome = sqliteSelectionFresh
		} else if err != nil {
			return sqliteSelection{}, fmt.Errorf("inspect explicit sqlite path %q: %w", path, err)
		} else {
			selection.existedBeforeOpen = true
		}
		logSQLiteSelection(log, cfg.SourceFor("database.path"), selection)
		return selection, nil
	}

	currentPath := filepath.Join(cfg.ResolvedDataDir(), "kandev.db")
	legacyPath := filepath.Join(cfg.ResolvedHomeDir(), "kandev.db")
	current, err := inspectSQLiteCandidate(currentPath)
	if err != nil {
		return sqliteSelection{}, err
	}
	legacy := sqliteCandidate{}
	if filepath.Clean(currentPath) != filepath.Clean(legacyPath) {
		legacy, err = inspectSQLiteCandidate(legacyPath)
		if err != nil {
			return sqliteSelection{}, err
		}
	}

	if current.exists {
		if current.taskCount == 0 && legacy.taskCount > 0 {
			conflict := &SQLiteDatabaseSelectionConflictError{
				CurrentPath:      currentPath,
				LegacyPath:       legacyPath,
				CurrentTaskCount: current.taskCount,
				LegacyTaskCount:  legacy.taskCount,
			}
			logSQLiteConflict(log, cfg.SourceFor("database.path"), conflict)
			return sqliteSelection{}, conflict
		}
		selection := sqliteSelection{
			path:              currentPath,
			existedBeforeOpen: true,
			outcome:           sqliteSelectionExisting,
		}
		logSQLiteSelection(log, cfg.SourceFor("database.path"), selection)
		return selection, nil
	}

	if !legacy.exists {
		selection := sqliteSelection{path: currentPath, outcome: sqliteSelectionFresh}
		logSQLiteSelection(log, cfg.SourceFor("database.path"), selection)
		return selection, nil
	}

	if err := adoptLegacySQLite(legacyPath, currentPath, legacy); err != nil {
		return sqliteSelection{}, fmt.Errorf("adopt legacy sqlite database %q into %q: %w", legacyPath, currentPath, err)
	}
	selection := sqliteSelection{
		path:    currentPath,
		outcome: sqliteSelectionLegacyAdopted,
	}
	logSQLiteSelection(log, cfg.SourceFor("database.path"), selection)
	return selection, nil
}

func databasePathIsExplicit(cfg *config.Config) bool {
	if cfg == nil || strings.TrimSpace(cfg.Database.Path) == "" {
		return false
	}
	switch cfg.SourceFor("database.path") {
	case config.SourceConfiguration, config.SourceEnvironment:
		return true
	default:
		return false
	}
}

func inspectSQLiteCandidate(path string) (sqliteCandidate, error) {
	candidate := sqliteCandidate{path: path}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return candidate, nil
	}
	if err != nil {
		return candidate, fmt.Errorf("inspect sqlite candidate %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return candidate, fmt.Errorf("inspect sqlite candidate %q: not a regular file", path)
	}
	candidate.exists = true

	reader, err := openSQLiteReadOnly(path)
	if err != nil {
		return candidate, fmt.Errorf("open sqlite candidate %q: %w", path, err)
	}
	defer func() { _ = reader.Close() }()

	var integrity string
	if err := reader.Get(&integrity, `PRAGMA integrity_check`); err != nil {
		return candidate, fmt.Errorf("check sqlite candidate %q integrity: %w", path, err)
	}
	if integrity != "ok" {
		return candidate, fmt.Errorf("check sqlite candidate %q integrity: %s", path, integrity)
	}
	if err := reader.Get(&candidate.tasksTable, `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'tasks')`); err != nil {
		return candidate, fmt.Errorf("inspect sqlite candidate %q task table: %w", path, err)
	}
	if candidate.tasksTable {
		if err := reader.Get(&candidate.taskCount, `SELECT COUNT(*) FROM tasks`); err != nil {
			return candidate, fmt.Errorf("count sqlite candidate %q tasks: %w", path, err)
		}
	}
	return candidate, nil
}

func openSQLiteReadOnly(path string) (*sqlx.DB, error) {
	connection, err := db.OpenSQLiteReader(path)
	if err != nil {
		return nil, err
	}
	reader := sqlx.NewDb(connection, "sqlite3")
	if err := reader.Ping(); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return reader, nil
}

func adoptLegacySQLite(legacyPath, currentPath string, legacy sqliteCandidate) error {
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		return fmt.Errorf("create data directory %q: %w", filepath.Dir(currentPath), err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(currentPath), ".kandev-legacy-*.db")
	if err != nil {
		return fmt.Errorf("create staged sqlite path: %w", err)
	}
	stagedPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("close staged sqlite path: %w", err)
	}
	if err := os.Remove(stagedPath); err != nil {
		return fmt.Errorf("prepare staged sqlite path: %w", err)
	}
	defer func() { _ = os.Remove(stagedPath) }()

	source, err := openSQLiteReadOnly(legacyPath)
	if err != nil {
		return fmt.Errorf("open legacy sqlite database: %w", err)
	}
	defer func() { _ = source.Close() }()
	if _, err := snapshotSQLite(source, stagedPath); err != nil {
		return fmt.Errorf("snapshot legacy sqlite database: %w", err)
	}
	if err := os.Chmod(stagedPath, 0o600); err != nil {
		return fmt.Errorf("protect staged sqlite database: %w", err)
	}
	staged, err := inspectSQLiteCandidate(stagedPath)
	if err != nil {
		return fmt.Errorf("validate staged sqlite database: %w", err)
	}
	if staged.taskCount != legacy.taskCount {
		return fmt.Errorf("staged sqlite database task count = %d, want %d", staged.taskCount, legacy.taskCount)
	}
	if err := os.Rename(stagedPath, currentPath); err != nil {
		return fmt.Errorf("install staged sqlite database: %w", err)
	}
	return nil
}

func logSQLiteSelection(log *logger.Logger, source config.SettingSource, selection sqliteSelection) {
	if log == nil {
		return
	}
	log.Info("SQLite database selected",
		zap.String("db_path", selection.path),
		zap.String("source", string(source)),
		zap.Bool("existed_before_open", selection.existedBeforeOpen),
		zap.String("outcome", string(selection.outcome)),
	)
}

func logSQLiteConflict(log *logger.Logger, source config.SettingSource, conflict *SQLiteDatabaseSelectionConflictError) {
	if log == nil {
		return
	}
	log.Warn("SQLite database selection conflict",
		zap.String("source", string(source)),
		zap.String("outcome", string(sqliteSelectionConflict)),
		zap.String("current_path", conflict.CurrentPath),
		zap.String("legacy_path", conflict.LegacyPath),
		zap.Int64("current_task_count", conflict.CurrentTaskCount),
		zap.Int64("legacy_task_count", conflict.LegacyTaskCount),
	)
}
