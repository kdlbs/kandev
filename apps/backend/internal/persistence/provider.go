package persistence

import (
	"database/sql"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

// Provide creates the database connection used by repositories.
func Provide(cfg *config.Config, log *logger.Logger) (*sql.DB, func() error, error) {
	_ = cfg

	driver := os.Getenv("KANDEV_DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}

	dbPath := os.Getenv("KANDEV_DB_PATH")
	if dbPath == "" {
		dbPath = "./kandev.db"
	}

	switch driver {
	case "sqlite":
		dbConn, err := db.OpenSQLite(dbPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open sqlite database: %w", err)
		}
		if log != nil {
			log.Info("Database initialized", zap.String("db_path", dbPath), zap.String("db_driver", driver))
		}
		cleanup := func() error {
			// Run PRAGMA optimize before closing to update query planner
			// statistics for tables that need it. This is the SQLite-recommended
			// way to maintain stats — lightweight and safe to call on every close.
			_, _ = dbConn.Exec("PRAGMA optimize")
			return dbConn.Close()
		}
		return dbConn, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}
