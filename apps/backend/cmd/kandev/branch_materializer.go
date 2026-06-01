package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

// branchMaterializerRepo is the slim repo surface the mid-session worktree
// materializer needs. Defined as an interface so the wiring stays decoupled
// from the concrete sqlite repo.
type branchMaterializerRepo interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
	GetTaskRepository(ctx context.Context, id string) (*models.TaskRepository, error)
	ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error)
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	GetTaskEnvironmentByTaskID(ctx context.Context, taskID string) (*models.TaskEnvironment, error)
	UpdateTaskEnvironment(ctx context.Context, env *models.TaskEnvironment) error
}

// agentctlRescanner is the lifecycle-tier surface the materializer uses to
// reach a running agentctl by session_id. The lifecycle manager owns the
// agentctl URL in memory — the executors_running row never persists it —
// so the materializer can't HTTP-POST agentctl directly. Defined as an
// interface so the materializer stays decoupled from the lifecycle package.
type agentctlRescanner interface {
	RescanWorkspaceForSession(ctx context.Context, sessionID, workDir string) error
	NotifyWorktreeMaterialized(ctx context.Context, wt lifecycle.MaterializedWorktree)
}

// branchMaterializer creates a worktree on disk for a newly added
// task_repositories row so MCP-driven "add branch" surfaces the worktree in
// the UI without waiting for a session relaunch. Implements
// taskservice.BranchMaterializer.
//
// It is best-effort: when there's no active session (no tasksession row yet,
// or the task has never launched), it no-ops and lets the standard
// multi-repo prepare path materialize on the next launch.
type branchMaterializer struct {
	repo        branchMaterializerRepo
	worktreeMgr *worktree.Manager
	rescanner   agentctlRescanner
	logger      *logger.Logger
}

func newBranchMaterializer(repo branchMaterializerRepo, mgr *worktree.Manager, lc *lifecycle.Manager, log *logger.Logger) *branchMaterializer {
	var rescanner agentctlRescanner
	if lc != nil {
		rescanner = lc
	}
	return &branchMaterializer{
		repo:        repo,
		worktreeMgr: mgr,
		rescanner:   rescanner,
		logger:      log.WithFields(zap.String("component", "branch_materializer")),
	}
}

// MaterializeBranch creates the worktree dir + persists the
// task_session_worktrees row for the just-inserted task_repositories row.
// Designed to be idempotent: the worktree manager's reuse path catches a
// rerun for the same (session, repo, branch_slug) triple and returns the
// existing worktree.
func (b *branchMaterializer) MaterializeBranch(ctx context.Context, taskID, taskRepositoryID string) error {
	if b == nil || b.worktreeMgr == nil {
		return nil
	}
	task, tr, repo, err := b.loadContext(ctx, taskID, taskRepositoryID)
	if err != nil {
		return err
	}
	if repo.LocalPath == "" {
		// Provider-backed repos that haven't been cloned yet need the
		// orchestrator's repoCloner path. Defer to next launch where the
		// existing clone-on-demand logic runs.
		b.logger.Info("skipping materialize: repository has no local path",
			zap.String("repository_id", repo.ID),
			zap.String("task_id", taskID))
		return nil
	}

	session, err := b.pickActiveSession(ctx, taskID)
	if err != nil {
		return err
	}
	if session == nil {
		b.logger.Info("skipping materialize: no active session for task",
			zap.String("task_id", taskID))
		return nil
	}

	env, err := b.repo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("lookup task environment: %w", err)
	}
	if env == nil || env.TaskDirName == "" {
		b.logger.Info("skipping materialize: task environment not provisioned yet",
			zap.String("task_id", taskID))
		return nil
	}

	slug := deriveBranchSlugForRow(tr)
	req := worktree.CreateRequest{
		TaskID:               taskID,
		SessionID:            session.ID,
		TaskTitle:            task.Title,
		RepositoryID:         repo.ID,
		RepositoryPath:       repo.LocalPath,
		BaseBranch:           tr.BaseBranch,
		FallbackBaseBranch:   repo.DefaultBranch,
		CheckoutBranch:       tr.CheckoutBranch,
		WorktreeBranchPrefix: repo.WorktreeBranchPrefix,
		PullBeforeWorktree:   repo.PullBeforeWorktree,
		TaskDirName:          env.TaskDirName,
		RepoName:             repo.Name,
		BranchSlug:           slug,
	}
	wt, err := b.worktreeMgr.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	b.logger.Info("materialized branch worktree",
		zap.String("task_id", taskID),
		zap.String("task_repository_id", taskRepositoryID),
		zap.String("worktree_id", wt.ID),
		zap.String("path", wt.Path),
		zap.String("branch", wt.Branch))

	// Promote the task environment's workspace_path to the task root when
	// the new worktree creates a sibling layout. Without this, agentctl's
	// scan stays scoped to the primary worktree and misses the new sibling.
	taskRoot := b.promoteWorkspacePathIfNeeded(ctx, env, wt.Path)

	// Live-update the running agentctl instance so the new worktree's
	// file/git events flow without a session restart. Best-effort: a
	// failed rescan leaves the worktree on disk; the next launch will
	// rebuild trackers via prepareMultiRepo and pick everything up.
	b.notifyAgentctlRescan(ctx, session, taskRoot)

	// Push a frontend WS event so the session's worktree tabs render the
	// new entry immediately. Without it the new tsw row would only
	// surface after a full page reload because the gateway's
	// session-update path doesn't watch the worktree table.
	if b.rescanner != nil {
		b.rescanner.NotifyWorktreeMaterialized(ctx, lifecycle.MaterializedWorktree{
			TaskID:         taskID,
			SessionID:      session.ID,
			WorktreeID:     wt.ID,
			WorktreePath:   wt.Path,
			WorktreeBranch: wt.Branch,
			RepositoryID:   repo.ID,
			BranchSlug:     slug,
		})
	}
	return nil
}

// promoteWorkspacePathIfNeeded switches task_environments.workspace_path
// from the primary worktree to the task root when the new worktree lives
// as a sibling (e.g. <task>/<repo>-<slug>/). The agent process's CWD does
// not change — that's set at launch time — but later session relaunches
// pick up multi-repo mode via prepareMultiRepo because workspace_path is
// now the parent of all repo dirs. Returns the resolved task-root path so
// the caller can pass it to the agentctl rescan.
//
// Sibling layout: <task-root>/<repo>/ and <task-root>/<repo>-<slug>/ both
// have filepath.Dir == <task-root>. The check compares parents to confirm
// the new worktree is actually a sibling of the env's workspace_path
// before mutating the row — otherwise an unrelated path would silently
// repoint the env at the wrong dir.
func (b *branchMaterializer) promoteWorkspacePathIfNeeded(ctx context.Context, env *models.TaskEnvironment, newWorktreePath string) string {
	taskRoot := filepath.Dir(newWorktreePath)
	if env.WorkspacePath == taskRoot {
		return taskRoot
	}
	if filepath.Dir(env.WorkspacePath) != taskRoot {
		b.logger.Warn("skip workspace_path promotion: new worktree is not a sibling of env workspace_path",
			zap.String("env_workspace_path", env.WorkspacePath),
			zap.String("new_worktree_path", newWorktreePath))
		return env.WorkspacePath
	}
	prev := env.WorkspacePath
	env.WorkspacePath = taskRoot
	env.UpdatedAt = time.Now().UTC()
	if err := b.repo.UpdateTaskEnvironment(ctx, env); err != nil {
		b.logger.Warn("failed to promote workspace_path to task root",
			zap.String("task_environment_id", env.ID),
			zap.String("from", prev),
			zap.String("to", taskRoot),
			zap.Error(err))
		return prev
	}
	b.logger.Info("promoted workspace_path to task root",
		zap.String("task_environment_id", env.ID),
		zap.String("from", prev),
		zap.String("to", taskRoot))
	return taskRoot
}

// notifyAgentctlRescan tells the running agentctl instance attached to
// this session to re-discover repo subdirs and add per-repo trackers for
// new sibling worktrees. Routes through the lifecycle manager because the
// agentctl URL is in-memory runtime state — the executors_running row
// never persists it — and the lifecycle manager is the single owner of
// per-execution agentctl clients.
//
// No-op when no rescanner is wired (test stubs, agentless test envs) or
// when the session has no active execution (agent stopped). The next
// session launch picks up the new layout via prepareMultiRepo regardless.
func (b *branchMaterializer) notifyAgentctlRescan(ctx context.Context, session *models.TaskSession, taskRoot string) {
	if b.rescanner == nil {
		return
	}
	rescanCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := b.rescanner.RescanWorkspaceForSession(rescanCtx, session.ID, taskRoot); err != nil {
		b.logger.Warn("agentctl rescan failed; new worktree will appear after next session restart",
			zap.String("session_id", session.ID),
			zap.String("task_root", taskRoot),
			zap.Error(err))
	}
}

// loadContext fetches the task + task_repository row + repository entity in
// one place so MaterializeBranch can fail-fast on missing data without
// scattering nil checks across the body.
func (b *branchMaterializer) loadContext(ctx context.Context, taskID, taskRepositoryID string) (*models.Task, *models.TaskRepository, *models.Repository, error) {
	task, err := b.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get task: %w", err)
	}
	if task == nil {
		return nil, nil, nil, fmt.Errorf("task not found: %s", taskID)
	}
	tr, err := b.repo.GetTaskRepository(ctx, taskRepositoryID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get task repository: %w", err)
	}
	if tr == nil {
		return nil, nil, nil, fmt.Errorf("task repository not found: %s", taskRepositoryID)
	}
	repo, err := b.repo.GetRepository(ctx, tr.RepositoryID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get repository: %w", err)
	}
	if repo == nil {
		return nil, nil, nil, fmt.Errorf("repository not found: %s", tr.RepositoryID)
	}
	return task, tr, repo, nil
}

// pickActiveSession returns the most recently active session for the task,
// or nil if none qualifies. Worktree persistence requires a session_id, so
// when there's no candidate we no-op rather than orphaning a worktree
// without an owning session.
func (b *branchMaterializer) pickActiveSession(ctx context.Context, taskID string) (*models.TaskSession, error) {
	sessions, err := b.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	var best *models.TaskSession
	for _, s := range sessions {
		if !sessionEligibleForMaterialize(s) {
			continue
		}
		if best == nil || s.UpdatedAt.After(best.UpdatedAt) {
			best = s
		}
	}
	return best, nil
}

func sessionEligibleForMaterialize(s *models.TaskSession) bool {
	switch s.State {
	case models.TaskSessionStateRunning,
		models.TaskSessionStateStarting,
		models.TaskSessionStateWaitingForInput,
		models.TaskSessionStateCreated:
		return true
	}
	return false
}

// deriveBranchSlugForRow chooses the slug input that uniquely identifies the
// row's worktree on disk. CheckoutBranch wins when set (local-executor and
// PR-style flows put the branch there); BaseBranch is the fallback for the
// worktree-executor flow where the branch lives in BaseBranch and
// CheckoutBranch is empty.
func deriveBranchSlugForRow(tr *models.TaskRepository) string {
	slug := worktree.SanitizeBranchSlug(tr.CheckoutBranch)
	if slug == "" {
		slug = worktree.SanitizeBranchSlug(tr.BaseBranch)
	}
	return slug
}

// ensureBranchMaterializerSatisfiesInterface is a compile-time guard so the
// interface contract drift surfaces at build, not at runtime.
var _ taskservice.BranchMaterializer = (*branchMaterializer)(nil)
