package worktree

import (
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// Provide creates the worktree manager using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, cfg *config.Config, log *logger.Logger) (*Manager, func() error, error) {
	store, err := NewSQLiteStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	wtCfg := Config{
		Enabled:             cfg.Worktree.Enabled,
		BasePath:            cfg.Worktree.BasePath,
		BranchPrefix:        "kandev/",
		FetchTimeoutSeconds: cfg.Worktree.FetchTimeoutSeconds,
		PullTimeoutSeconds:  cfg.Worktree.PullTimeoutSeconds,
	}
	wtCfg.SetDataDirFallback(cfg.ResolvedHomeDir())
	manager, err := NewManager(wtCfg, store, log)
	if err != nil {
		return nil, nil, err
	}
	return manager, func() error { return nil }, nil
}
