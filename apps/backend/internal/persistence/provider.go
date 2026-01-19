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
		return dbConn, func() error { return dbConn.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}
