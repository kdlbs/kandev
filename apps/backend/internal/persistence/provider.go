package persistence

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

// Provide creates the database connection used by repositories.
func Provide(cfg *config.Config, log *logger.Logger) (*sqlx.DB, func() error, error) {
	driver := cfg.Database.Driver
	if driver == "" {
		driver = "sqlite"
	}

	switch driver {
	case "sqlite":
		dbPath := cfg.Database.Path
		if dbPath == "" {
			dbPath = "./kandev.db"
		}

		dbConn, err := db.OpenSQLite(dbPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}

		sqlxDB := sqlx.NewDb(dbConn, "sqlite3")

		if log != nil {
			log.Info("Database initialized", zap.String("db_path", dbPath), zap.String("db_driver", driver))
		}
		cleanup := func() error {
			// Run PRAGMA optimize before closing to update query planner
			// statistics for tables that need it. This is the SQLite-recommended
			// way to maintain stats — lightweight and safe to call on every close.
			_, _ = sqlxDB.Exec("PRAGMA optimize")
			return sqlxDB.Close()
		}
		return sqlxDB, cleanup, nil

	case "postgres":
		dsn := cfg.Database.DSN()
		dbConn, err := db.OpenPostgres(dsn, cfg.Database.MaxConns, cfg.Database.MinConns)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open postgres database: %w", err)
		}

		sqlxDB := sqlx.NewDb(dbConn, "pgx")

		if log != nil {
			log.Info("Database initialized", zap.String("db_driver", driver))
		}
		cleanup := func() error {
			return sqlxDB.Close()
		}
		return sqlxDB, cleanup, nil

	default:
		return nil, nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}
