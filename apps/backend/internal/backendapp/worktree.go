package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
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
	isResuming, _ := req.Metadata["is_resuming"].(bool)
	return a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: req.TaskSessionID,
		TaskID:        req.TaskID,
		CompletedTurn: isResuming,
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
		destroyer := &environmentDestroyerAdapter{
			lifecycle: lifecycleMgr,
			worktrees: manager,
		}
		taskSvc.SetEnvironmentDestroyer(destroyer)
		taskSvc.SetTaskEnvironmentRuntimeSecretDeleter(destroyer)
	}
	taskSvc.SetSSHTaskDirReclaimer(newSSHTaskDirReclaimerAdapter(log))

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

func (a *environmentDestroyerAdapter) DeleteTaskEnvironmentRuntimeSecrets(
	ctx context.Context,
	taskEnvironmentID, authSecretID, bootstrapSecretID string,
) error {
	return a.lifecycle.DeleteTaskEnvironmentRuntimeSecrets(ctx, taskEnvironmentID, authSecretID, bootstrapSecretID)
}

func (a *environmentDestroyerAdapter) GetContainerLiveStatus(ctx context.Context, containerID string) (*taskservice.ContainerLiveStatus, error) {
	live, err := a.lifecycle.GetContainerLiveStatus(ctx, containerID)
	if err != nil || live == nil {
		return nil, err
	}
	out := &taskservice.ContainerLiveStatus{
		ContainerID: live.ContainerID,
		State:       live.State,
		Status:      live.Status,
		ExitCode:    live.ExitCode,
		Health:      live.Health,
		Missing:     live.Missing,
	}
	if live.StartedAt != nil {
		out.StartedAt = live.StartedAt.Format(time.RFC3339)
	}
	if live.FinishedAt != nil {
		out.FinishedAt = live.FinishedAt.Format(time.RFC3339)
	}
	return out, nil
}

// pushBranchTimeout caps how long we'll wait for `git push` before treating
// the call as hung. Reset is invoked from the request thread; without a bound
// a stalled remote (auth prompt, packet loss, slow proxy) would block the
// entire HTTP handler indefinitely.
const pushBranchTimeout = 30 * time.Second

func (a *environmentDestroyerAdapter) PushEnvironmentBranch(ctx context.Context, env *models.TaskEnvironment) error {
	// For host-side worktrees we can push directly. Container/sandbox workspaces
	// would require an active agentctl client — not wired yet, surface a clear
	// error so the user knows to push manually.
	worktreePath, branch := firstEnvironmentWorktree(env)
	if worktreePath == "" {
		return fmt.Errorf("push-before-reset is not supported for this environment type; please push manually first")
	}
	var args []string
	if branch == "" {
		args = []string{"push"}
	} else {
		// Prefer the branch's configured upstream remote so this works for
		// repos whose primary remote isn't called "origin" (e.g. fork
		// workflows with "upstream"/"github"). Fall back to "origin" only
		// when no upstream is set, matching the historical behaviour.
		remote := detectBranchRemote(ctx, worktreePath, branch)
		args = []string{"push", remote, branch}
	}
	out, runErr, execCtxErr := subproc.RunGitCombinedAfterAcquire(ctx, subproc.GitInteractive, pushBranchTimeout, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, args...)
		cmd.Dir = worktreePath
		// Disable interactive credential prompts — without this, a missing
		// credential helper can hang waiting on stdin even with the timeout
		// above (signal delivery is gated behind the prompt read).
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		return cmd
	})
	if runErr != nil || execCtxErr != nil {
		if errors.Is(execCtxErr, context.DeadlineExceeded) {
			return fmt.Errorf("git push timed out after %s in %s (branch %q); push manually and retry", pushBranchTimeout, worktreePath, branch)
		}
		if runErr == nil {
			runErr = execCtxErr
		}
		return fmt.Errorf("git push failed: %s: %w", strings.TrimSpace(string(out)), runErr)
	}
	return nil
}

// firstEnvironmentWorktree returns the path and branch of the environment's
// first repository row with a worktree path.
func firstEnvironmentWorktree(env *models.TaskEnvironment) (string, string) {
	for _, repo := range env.Repos {
		if repo != nil && repo.WorktreePath != "" {
			return repo.WorktreePath, repo.WorktreeBranch
		}
	}
	return "", ""
}

// defaultGitRemote is the conventional default remote name. Used as the
// fallback when a branch has no configured upstream and at the comment level
// when describing historical hard-coded behaviour.
const defaultGitRemote = "origin"

// detectBranchRemote reads the configured upstream remote for `branch` from
// the worktree's git config. Falls back to defaultGitRemote when no upstream
// is set (matching the historical hard-coded behaviour).
func detectBranchRemote(ctx context.Context, dir, branch string) string {
	out, runErr, execCtxErr := subproc.RunGitOutputAfterAcquire(
		ctx,
		subproc.GitInteractive,
		pushBranchTimeout,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, "config", "--get", "branch."+branch+".remote")
			cmd.Dir = dir
			return cmd
		},
	)
	if runErr != nil || execCtxErr != nil {
		return defaultGitRemote
	}
	remote := strings.TrimSpace(string(out))
	if remote == "" {
		return defaultGitRemote
	}
	return remote
}
