package persistence

import (
	"fmt"

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
func Provide(cfg *config.Config, log *logger.Logger) (*db.Pool, func() error, error) {
	driver := cfg.Database.Driver
	if driver == "" {
		driver = "sqlite"
	}

	switch driver {
	case "sqlite":
		return provideSQLite(cfg, log)
	case "postgres":
		return providePostgres(cfg, log)
	default:
		return nil, nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func provideSQLite(cfg *config.Config, log *logger.Logger) (*db.Pool, func() error, error) {
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "./kandev.db"
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
		log.Info("Database initialized", zap.String("db_driver", "postgres"))
	}
	cleanup := func() error {
		return pool.Close()
	}
	return pool, cleanup, nil
}
