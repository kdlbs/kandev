package main

import (
	"database/sql"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func provideWorktreeManager(dbConn *sql.DB, cfg *config.Config, log *logger.Logger, lifecycleMgr *lifecycle.Manager, taskSvc *taskservice.Service) (*worktree.Manager, func() error, error) {
	manager, cleanup, err := worktree.Provide(dbConn, cfg, log)
	if err != nil {
		return nil, nil, err
	}
	if lifecycleMgr != nil {
		lifecycleMgr.SetWorktreeManager(manager)
	}
	taskSvc.SetWorktreeCleanup(manager)
	return manager, cleanup, nil
}
