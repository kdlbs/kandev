package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
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

// bootMsgAdapter adapts the task service to the lifecycle.BootMessageService interface.
type bootMsgAdapter struct {
	svc *taskservice.Service
}

func (a *bootMsgAdapter) CreateMessage(ctx context.Context, req *lifecycle.BootMessageRequest) (*models.Message, error) {
	return a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		Content:       req.Content,
		AuthorType:    req.AuthorType,
		Type:          req.Type,
		Metadata:      req.Metadata,
	})
}

func (a *bootMsgAdapter) UpdateMessage(ctx context.Context, message *models.Message) error {
	return a.svc.UpdateMessage(ctx, message)
}

func provideWorktreeManager(dbPool *db.Pool, cfg *config.Config, log *logger.Logger, lifecycleMgr *lifecycle.Manager, taskSvc *taskservice.Service) (*worktree.Manager, *worktree.Recreator, func() error, error) {
	manager, cleanup, err := worktree.Provide(dbPool.Writer(), dbPool.Reader(), cfg, log)
	if err != nil {
		return nil, nil, nil, err
	}
	if lifecycleMgr != nil {
		lifecycleMgr.SetWorktreeManager(manager)
		lifecycleMgr.SetBootMessageService(&bootMsgAdapter{svc: taskSvc})
	}
	taskSvc.SetWorktreeCleanup(manager)
	if lifecycleMgr != nil {
		taskSvc.SetEnvironmentDestroyer(&environmentDestroyerAdapter{
			lifecycle: lifecycleMgr,
			worktrees: manager,
		})
	}

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

	// Create recreator for orchestrator to use during session resume
	recreator := worktree.NewRecreator(manager)

	return manager, recreator, cleanup, nil
}

// environmentDestroyerAdapter implements taskservice.EnvironmentDestroyer by
// delegating to the lifecycle Manager (for containers/sandboxes) and the worktree
// Manager (for worktrees). Branch is preserved on worktree removal so unpushed
// work is never silently dropped.
type environmentDestroyerAdapter struct {
	lifecycle *lifecycle.Manager
	worktrees *worktree.Manager
}

func (a *environmentDestroyerAdapter) DestroyContainer(ctx context.Context, containerID string) error {
	return a.lifecycle.DestroyContainer(ctx, containerID)
}

func (a *environmentDestroyerAdapter) DestroySandbox(ctx context.Context, sandboxID, executionID string) error {
	return a.lifecycle.DestroySandbox(ctx, sandboxID, executionID)
}

func (a *environmentDestroyerAdapter) DestroyWorktree(ctx context.Context, worktreeID string) error {
	// removeBranch=false: preserve the branch so unpushed work isn't lost.
	return a.worktrees.RemoveByID(ctx, worktreeID, false)
}

func (a *environmentDestroyerAdapter) PushEnvironmentBranch(ctx context.Context, env *models.TaskEnvironment) error {
	// For host-side worktrees we can push directly. Container/sandbox workspaces
	// would require an active agentctl client — not wired yet, surface a clear
	// error so the user knows to push manually.
	if env.WorktreePath == "" {
		return fmt.Errorf("push-before-reset is not supported for this environment type; please push manually first")
	}
	branch := strings.TrimSpace(env.WorktreeBranch)
	var cmd *exec.Cmd
	if branch == "" {
		cmd = exec.CommandContext(ctx, "git", "push")
	} else {
		cmd = exec.CommandContext(ctx, "git", "push", "origin", branch)
	}
	cmd.Dir = env.WorktreePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
