package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
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
	driver := cfg.Database.Driver
	if driver == "" {
		driver = "sqlite"
	}

	switch driver {
	case "sqlite":
		return provideSQLite(cfg, log, version)
	case "postgres":
		return providePostgres(cfg, log)
	default:
		return nil, nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func provideSQLite(cfg *config.Config, log *logger.Logger, version string) (*db.Pool, func() error, error) {
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = filepath.Join(cfg.ResolvedDataDir(), "kandev.db")
		if err := migrateLegacyDBPath(cfg, dbPath, log); err != nil {
			return nil, nil, fmt.Errorf("migrate legacy DB: %w", err)
		}
	}

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
		backupDir := filepath.Join(filepath.Dir(dbPath), "backups")
		if err := os.MkdirAll(backupDir, 0o755); err != nil {
			_ = pool.Close()
			return nil, nil, fmt.Errorf("create backup dir: %w", err)
		}
		path := snapshotPath(backupDir, storedVersion)
		size, err := snapshotSQLite(writer, path)
		if err != nil {
			_ = pool.Close()
			return nil, nil, fmt.Errorf("pre-migration backup failed: %w", err)
		}
		if log != nil {
			log.Info("pre-migration backup taken",
				zap.String("from_version", fallback(storedVersion, "pre-meta")),
				zap.String("to_version", version),
				zap.String("path", path),
				zap.Int64("size_bytes", size),
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

// migrateLegacyDBPath moves a pre-KANDEV_HOME_DIR SQLite DB from
// <HomeDir>/kandev.db into the new derived location at <HomeDir>/data/kandev.db
// on first boot after an upgrade, so `docker pull && docker restart` doesn't
// silently start against an empty DB.
//
// Runs only when:
//   - KANDEV_DATABASE_PATH is not explicitly set (caller checks this).
//   - The new derived path does not exist yet.
//   - A legacy file exists at <HomeDir>/kandev.db.
//   - The legacy path differs from the new path (skip when HomeDir == DataDir).
//
// Only the main .db file is moved - SQLite recreates -wal/-shm on open, and a
// cleanly-shut-down DB (the expected state on container restart) has empty WAL.
func migrateLegacyDBPath(cfg *config.Config, newPath string, log *logger.Logger) error {
	if _, err := os.Stat(newPath); !errors.Is(err, fs.ErrNotExist) {
		return nil // new path exists (or stat failed) - nothing to migrate
	}
	legacyPath := filepath.Join(cfg.ResolvedHomeDir(), "kandev.db")
	if legacyPath == newPath {
		return nil
	}
	if _, err := os.Stat(legacyPath); err != nil {
		return nil // no legacy file to migrate
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return fmt.Errorf("create data dir %s: %w", filepath.Dir(newPath), err)
	}
	if err := os.Rename(legacyPath, newPath); err != nil {
		return fmt.Errorf("move %s -> %s: %w", legacyPath, newPath, err)
	}
	// Also move -wal / -shm if present. These are transient (SQLite recreates
	// them on open) but moving them avoids orphaned files on the volume.
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Rename(legacyPath+suffix, newPath+suffix)
	}
	if log != nil {
		log.Info("Migrated SQLite database from pre-KANDEV_HOME_DIR location",
			zap.String("legacy_path", legacyPath),
			zap.String("new_path", newPath),
		)
	}
	return nil
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

	if log != nil {
		log.Info("pre-migration backup skipped: postgres driver (use pg_dump)")
		log.Info("Database initialized", zap.String("db_driver", "postgres"))
	}
	cleanup := func() error {
		return pool.Close()
	}
	return pool, cleanup, nil
}
