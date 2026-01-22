package worktree

import (
	"database/sql"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// Provide creates the worktree manager using the shared database connection.
func Provide(db *sql.DB, cfg *config.Config, log *logger.Logger) (*Manager, func() error, error) {
	store, err := NewSQLiteStore(db)
	if err != nil {
		return nil, nil, err
	}
	manager, err := NewManager(Config{
		Enabled:      cfg.Worktree.Enabled,
		BasePath:     cfg.Worktree.BasePath,
		MaxPerRepo:   cfg.Worktree.MaxPerRepo,
		BranchPrefix: "kandev/",
	}, store, log)
	if err != nil {
		return nil, nil, err
	}
	return manager, func() error { return nil }, nil
}
