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
	currentPath       string
	legacyPath        string
	currentTaskCount  int64
	legacyTaskCount   int64
}

type sqliteCandidate struct {
	path           string
	exists         bool
	tasksTable     bool
	hasTaskHistory bool
	taskCount      int64
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

// selectSQLiteDatabase runs after backend runtime-state ownership is acquired.
// The lock isolates selection and adoption from other Kandev instances.
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
	current, err := inspectSQLiteCandidateLightweight(currentPath)
	if err != nil {
		return sqliteSelection{}, err
	}
	if current.exists && current.hasTaskHistory {
		selection := sqliteSelection{
			path:              currentPath,
			existedBeforeOpen: true,
			outcome:           sqliteSelectionExisting,
		}
		logSQLiteSelection(log, cfg.SourceFor("database.path"), selection)
		return selection, nil
	}

	legacy := sqliteCandidate{}
	if filepath.Clean(currentPath) != filepath.Clean(legacyPath) {
		legacy, err = inspectSQLiteCandidate(legacyPath)
		if err != nil {
			return sqliteSelection{}, err
		}
	}

	if current.exists {
		if !current.hasTaskHistory && legacy.hasTaskHistory {
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
		path:             currentPath,
		outcome:          sqliteSelectionLegacyAdopted,
		currentPath:      currentPath,
		legacyPath:       legacyPath,
		currentTaskCount: current.taskCount,
		legacyTaskCount:  legacy.taskCount,
	}
	logSQLiteSelection(log, cfg.SourceFor("database.path"), selection)
	return selection, nil
}

func databasePathIsExplicit(cfg *config.Config) bool {
	if cfg == nil || strings.TrimSpace(cfg.Database.Path) == "" {
		return false
	}
	// Configuration-file paths are explicit too; they cross the managed
	// boundary through the internal config-file handoff.
	switch cfg.SourceFor("database.path") {
	case config.SourceConfiguration, config.SourceEnvironment:
		return true
	default:
		return false
	}
}

func inspectSQLiteCandidate(path string) (sqliteCandidate, error) {
	return inspectSQLiteCandidateWithMode(path, true)
}

func inspectSQLiteCandidateLightweight(path string) (sqliteCandidate, error) {
	return inspectSQLiteCandidateWithMode(path, false)
}

func inspectSQLiteCandidateWithMode(path string, checkIntegrity bool) (sqliteCandidate, error) {
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

	if checkIntegrity {
		var integrity string
		if err := reader.Get(&integrity, `PRAGMA integrity_check`); err != nil {
			return candidate, fmt.Errorf("check sqlite candidate %q integrity: %w", path, err)
		}
		if integrity != "ok" {
			return candidate, fmt.Errorf("check sqlite candidate %q integrity: %s", path, integrity)
		}
	}
	if err := reader.Get(&candidate.tasksTable, `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'tasks')`); err != nil {
		return candidate, fmt.Errorf("inspect sqlite candidate %q task table: %w", path, err)
	}
	if candidate.tasksTable {
		if checkIntegrity {
			if err := reader.Get(&candidate.taskCount, `SELECT COUNT(*) FROM tasks`); err != nil {
				return candidate, fmt.Errorf("count sqlite candidate %q tasks: %w", path, err)
			}
			candidate.hasTaskHistory = candidate.taskCount > 0
		} else if err := reader.Get(&candidate.hasTaskHistory, `SELECT EXISTS (SELECT 1 FROM tasks LIMIT 1)`); err != nil {
			return candidate, fmt.Errorf("inspect sqlite candidate %q task history: %w", path, err)
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
	// The caller holds runtime-state ownership for the legacy and current paths.
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		return fmt.Errorf("create data directory %q: %w", filepath.Dir(currentPath), err)
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(currentPath), ".kandev-legacy-*")
	if err != nil {
		return fmt.Errorf("create private sqlite staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()
	stagedPath := filepath.Join(stagingDir, "kandev.db")

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
	if err := installStagedSQLite(stagedPath, currentPath); err != nil {
		return err
	}
	return nil
}

// installStagedSQLite creates the target without replacing a file that may
// have appeared after candidate inspection. Both paths are on the same
// filesystem, so the hard-link creation is atomic and the staged name can be
// removed after installation.
func installStagedSQLite(stagedPath, currentPath string) error {
	if err := os.Link(stagedPath, currentPath); err != nil {
		return fmt.Errorf("install staged sqlite database without replacing existing target: %w", err)
	}
	if err := os.Remove(stagedPath); err != nil {
		return fmt.Errorf("remove staged sqlite path after installation: %w", err)
	}
	return nil
}

func logSQLiteSelection(log *logger.Logger, source config.SettingSource, selection sqliteSelection) {
	if log == nil {
		return
	}
	fields := []zap.Field{
		zap.String("db_path", selection.path),
		zap.String("source", string(source)),
		zap.Bool("existed_before_open", selection.existedBeforeOpen),
		zap.String("outcome", string(selection.outcome)),
	}
	if selection.outcome == sqliteSelectionLegacyAdopted {
		fields = append(fields,
			zap.String("current_path", selection.currentPath),
			zap.String("legacy_path", selection.legacyPath),
			zap.Int64("current_task_count", selection.currentTaskCount),
			zap.Int64("legacy_task_count", selection.legacyTaskCount),
		)
	}
	log.Info("SQLite database selected", fields...)
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
