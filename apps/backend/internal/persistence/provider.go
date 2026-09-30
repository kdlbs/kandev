package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/startup"
)

// Provide creates the database connection pool used by repositories.
// For SQLite it returns a Pool with a single-writer connection and a
// multi-reader connection pool (leveraging WAL for concurrent reads).
// For PostgreSQL both Writer and Reader point to the same *sqlx.DB.
//
// version is the current binary version string (e.g. "v0.43.0").  It is
// compared against the stored kandev_version to decide whether to take a
// pre-migration backup.  Pass "" in tests that do not care about snapshots.
func Provide(cfg *config.Config, log *logger.Logger, version string) (*db.Pool, func() error, error) {
	return ProvideContext(context.Background(), cfg, log, version)
}

// ProvideContext opens persistence for a cancellable backend initialization.
func ProvideContext(ctx context.Context, cfg *config.Config, log *logger.Logger, version string) (*db.Pool, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	startup.SetPhase(ctx, startup.OpeningDatabase)
	driver := cfg.Database.Driver
	if driver == "" {
		driver = "sqlite"
	}

	switch driver {
	case "sqlite":
		return provideSQLite(ctx, cfg, log, version)
	case "postgres":
		return providePostgres(cfg, log)
	default:
		return nil, nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func provideSQLite(ctx context.Context, cfg *config.Config, log *logger.Logger, version string) (*db.Pool, func() error, error) {
	selection, err := selectSQLiteDatabase(cfg, log)
	if err != nil {
		return nil, nil, fmt.Errorf("select sqlite database: %w", err)
	}
	dbPath := selection.path

	// Writer: single connection, owns WAL/journal_mode setup.
	writerConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open sqlite writer: %w", err)
	}
	writer := sqlx.NewDb(writerConn, "sqlite3")

	// Reader: multiple read-only connections for concurrent SELECTs.
	readerConn, err := db.OpenSQLiteReader(dbPath)
	if err != nil {
		_ = writer.Close()
		return nil, nil, fmt.Errorf("failed to open sqlite reader: %w", err)
	}
	reader := sqlx.NewDb(readerConn, "sqlite3")

	pool := db.NewPool(writer, reader)

	// --- meta + backup window ---
	// Runs before any repository touches the DB so the snapshot is a clean
	// pre-migration image.
	if err := ensureMetaTable(writer); err != nil {
		_ = pool.Close()
		return nil, nil, fmt.Errorf("ensure meta table: %w", err)
	}

	// Meta reads must succeed: if we cannot determine whether this is an
	// upgrade boot we must NOT charge ahead into migrations. The dominant
	// failure mode here is a partially-corrupt DB that opens but cannot
	// read sqlite_master / kandev_meta - exactly the case where a backup
	// would matter most.
	storedVersion, err := readKey(writer, "kandev_version")
	if err != nil {
		_ = pool.Close()
		return nil, nil, fmt.Errorf("read kandev_version: %w", err)
	}
	userTables, err := hasUserTables(writer)
	if err != nil {
		_ = pool.Close()
		return nil, nil, fmt.Errorf("inspect user tables: %w", err)
	}

	if shouldBackup(storedVersion, version, userTables) {
		startup.SetPhase(ctx, startup.BackingUpDatabase)
		started := time.Now()
		backupDir := filepath.Join(filepath.Dir(dbPath), "backups")
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			_ = pool.Close()
			return nil, nil, fmt.Errorf("create backup dir: %w", err)
		}
		startup.BeginStep(ctx, startup.StepDatabaseBackup)
		path, size, err := createPreMigrationBackup(ctx, writer, backupDir, storedVersion)
		startup.EndStep(ctx, startup.StepDatabaseBackup)
		if err != nil {
			_ = pool.Close()
			return nil, nil, err
		}
		if log != nil {
			log.Info("pre-migration backup taken",
				zap.String("from_version", fallback(storedVersion, "pre-meta")),
				zap.String("to_version", version),
				zap.String("path", path),
				zap.Int64("size_bytes", size),
				zap.Duration("duration", time.Since(started)),
			)
		}
		_ = pruneBackups(backupDir, 2)
	} else if storedVersion == "" && !userTables {
		// Fresh DB - record first-boot timestamp.
		_ = writeKey(writer, "schema_initialized_at", time.Now().UTC().Format(time.RFC3339))
	}

	if log != nil {
		log.Info("Database initialized (single-writer pool)",
			zap.String("db_path", dbPath),
			zap.String("db_driver", "sqlite"),
		)
	}

	cleanup := func() error {
		// Run PRAGMA optimize before closing to update query planner
		// statistics for tables that need it.
		_, _ = writer.Exec("PRAGMA optimize")
		return pool.Close()
	}
	return pool, cleanup, nil
}

func createPreMigrationBackup(ctx context.Context, writer *sqlx.DB, backupDir, storedVersion string) (string, int64, error) {
	path := snapshotPath(backupDir, storedVersion)
	stagingDir, err := os.MkdirTemp(backupDir, ".kandev-backup-*")
	if err != nil {
		return "", 0, fmt.Errorf("create private backup staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	stagedPath := filepath.Join(stagingDir, filepath.Base(path))
	stopTracking := trackBackupProgress(ctx, stagedPath)
	size, err := snapshotSQLiteContext(ctx, writer, stagedPath)
	stopTracking()
	if err != nil {
		return "", 0, fmt.Errorf("pre-migration backup failed: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", 0, fmt.Errorf("startup canceled after pre-migration backup: %w", err)
	}
	if err := os.Chmod(stagedPath, 0o600); err != nil {
		return "", 0, fmt.Errorf("protect staged pre-migration backup: %w", err)
	}
	if _, err := inspectSQLiteCandidate(stagedPath); err != nil {
		return "", 0, fmt.Errorf("validate staged pre-migration backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", 0, fmt.Errorf("startup canceled before installing pre-migration backup: %w", err)
	}
	if err := installStagedSQLite(stagedPath, path); err != nil {
		return "", 0, fmt.Errorf("install pre-migration backup: %w", err)
	}
	return path, size, nil
}

// backupProgressTickInterval is how often trackBackupProgress stats the
// staging file. VACUUM INTO gives no row-level progress, so this is the only
// signal the counting step has.
const backupProgressTickInterval = 250 * time.Millisecond

// trackBackupProgress polls stagedPath's size while a VACUUM INTO writes it
// and reports the increase to the database.backup step. VACUUM INTO creates
// the file only once it starts writing, so ENOENT before that point is
// expected and not a measurement failure; any other stat error degrades the
// step once, not on every tick. The returned stop function cancels the
// goroutine and waits for it to exit before returning.
func trackBackupProgress(ctx context.Context, stagedPath string) func() {
	tickerCtx, cancel := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(backupProgressTickInterval)
		defer ticker.Stop()
		var lastSize int64
		var degraded bool
		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-ticker.C:
				lastSize, degraded = reportBackupProgress(ctx, stagedPath, lastSize, degraded)
			}
		}
	}()
	return func() {
		cancel()
		<-stopped
	}
}

// reportBackupProgress stats stagedPath once and advances the step by the
// size increase since lastSize, returning the updated size and degraded
// state for the next tick.
func reportBackupProgress(ctx context.Context, stagedPath string, lastSize int64, degraded bool) (int64, bool) {
	info, err := os.Stat(stagedPath)
	if err != nil {
		if !os.IsNotExist(err) && !degraded {
			startup.Degrade(ctx, startup.StepDatabaseBackup)
			degraded = true
		}
		return lastSize, degraded
	}
	if delta := info.Size() - lastSize; delta > 0 {
		startup.Advance(ctx, startup.StepDatabaseBackup, delta)
		lastSize = info.Size()
	}
	return lastSize, degraded
}

func providePostgres(cfg *config.Config, log *logger.Logger) (*db.Pool, func() error, error) {
	dsn := cfg.Database.DSN()
	dbConn, err := db.OpenPostgres(dsn, cfg.Database.MaxConns, cfg.Database.MinConns)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	pgDB := sqlx.NewDb(dbConn, "pgx")
	// For Postgres, writer and reader share the same pool.
	pool := db.NewPool(pgDB, pgDB)
	if err := ensureMetaTable(pgDB); err != nil {
		_ = pool.Close()
		return nil, nil, fmt.Errorf("ensure meta table: %w", err)
	}

	if log != nil {
		log.Info("pre-migration backup skipped: postgres driver (use pg_dump)")
		log.Info("Database initialized", zap.String("db_driver", "postgres"))
	}
	cleanup := func() error {
		return pool.Close()
	}
	return pool, cleanup, nil
}
