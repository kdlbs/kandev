package main

import (
	"context"
	"database/sql"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// taskServiceAdapter adapts the task service to the worktree.TaskService interface.
type taskServiceAdapter struct {
	svc *taskservice.Service
}

func (a *taskServiceAdapter) CreateMessage(ctx context.Context, req *worktree.CreateMessageRequest) (*models.Message, error) {
	// Convert worktree.CreateMessageRequest to taskservice.CreateMessageRequest
	return a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		TurnID:        req.TurnID,
		Content:       req.Content,
		AuthorType:    req.AuthorType,
		AuthorID:      req.AuthorID,
		RequestsInput: req.RequestsInput,
		Type:          req.Type,
		Metadata:      req.Metadata,
	})
}

func (a *taskServiceAdapter) UpdateMessage(ctx context.Context, message *models.Message) error {
	return a.svc.UpdateMessage(ctx, message)
}

func provideWorktreeManager(dbConn *sql.DB, cfg *config.Config, log *logger.Logger, lifecycleMgr *lifecycle.Manager, taskSvc *taskservice.Service) (*worktree.Manager, func() error, error) {
	manager, cleanup, err := worktree.Provide(dbConn, cfg, log)
	if err != nil {
		return nil, nil, err
	}
	if lifecycleMgr != nil {
		lifecycleMgr.SetWorktreeManager(manager)
	}
	taskSvc.SetWorktreeCleanup(manager)

	// Wire script message handler with adapters
	taskSvcAdapter := &taskServiceAdapter{svc: taskSvc}

	scriptHandler := worktree.NewDefaultScriptMessageHandler(
		log,
		taskSvcAdapter,
		constants.SetupScriptTimeout,
	)
	repoAdapter := worktree.NewRepositoryAdapter(taskSvc)

	manager.SetScriptMessageHandler(scriptHandler)
	manager.SetRepositoryProvider(repoAdapter)

	return manager, cleanup, nil
}
