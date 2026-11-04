package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

type branchContinuityFixture struct {
	repo     *tasksqlite.Repository
	executor *Executor
	manager  *mockAgentManager
	session  *models.TaskSession
	env      *models.TaskEnvironment
	row      *models.TaskEnvironmentRepo
	info     *repoInfo
	receipt  *models.WorkspaceInventoryRecoveryReceipt
}

func newBranchContinuityFixture(t *testing.T) *branchContinuityFixture {
	t.Helper()
	db, _ := remoteSelectionSQLiteTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(db, db, nil)
	ctx := t.Context()
	source, checkout := createExecutorPreservationFixture(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repository", WorkspaceID: "workspace", Name: "repository", SourceType: "github", LocalPath: source,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Task"}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repository", TaskID: "task", RepositoryID: "repository", BaseBranch: "main",
	}))
	env := &models.TaskEnvironment{
		ID: "environment", TaskID: "task", ExecutorType: "worktree", ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: checkout,
	}
	row := &models.TaskEnvironmentRepo{
		ID: "environment-repo", TaskEnvironmentID: env.ID, RepositoryID: "repository", BranchSlug: "stale",
		WorktreeID: "worktree-recovery", WorktreePath: checkout, WorktreeBranch: "feature/recovery", Status: "active",
	}
	env.Repos = []*models.TaskEnvironmentRepo{row}
	require.NoError(t, repo.CreateTaskEnvironment(ctx, env))
	session := &models.TaskSession{
		ID: "session", TaskID: "task", TaskEnvironmentID: env.ID, RepositoryID: "repository",
		State: models.TaskSessionStateFailed, ExecutorID: models.ExecutorIDWorktree, AgentProfileID: "profile-123",
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	manager := &mockAgentManager{}
	executor := newTestExecutor(t, manager, repo)
	task, err := repo.GetTask(ctx, session.TaskID)
	require.NoError(t, err)
	req, _, _, _, _, err := executor.buildResumeRequestAtCredentialBoundaryWithOptions(ctx, task.ToAPI(), session, false, nil,
		ResumeOptions{RepairWorkspaceInventory: true, WorkspaceInventoryIdempotencyKey: "repair"})
	require.NoError(t, err)
	require.NotNil(t, req.WorkspaceInventoryRecoveryReceipt)
	receipt := req.WorkspaceInventoryRecoveryReceipt
	require.True(t, receipt.PostRepairMatched)
	require.NotNil(t, receipt.PostRepairVerifiedAt)
	receipt, err = repo.GetWorkspaceInventoryRepairReceipt(ctx, "task", "repair")
	require.NoError(t, err)
	rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	env.Repos = rows
	return &branchContinuityFixture{
		repo: repo, executor: executor, manager: manager, session: session, env: env, row: rows[0], receipt: receipt,
		info: &repoInfo{TaskRepositoryID: "task-repository", RepositoryID: "repository", RepositoryPath: source,
			Repository: &models.Repository{ID: "repository", SourceType: "github", LocalPath: source}},
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11
func TestLaunchPreparedSessionAllowsAttestedBranchEvolution(t *testing.T) {
	f := newBranchContinuityFixture(t)
	f.changeBranch(t, true)
	session := *f.session
	session.ID = "additional-session"
	session.State = models.TaskSessionStateCreated
	require.NoError(t, f.repo.CreateTaskSession(t.Context(), &session))
	task, err := f.repo.GetTask(t.Context(), session.TaskID)
	require.NoError(t, err)
	before := f.evidence(t)
	execution, err := f.executor.LaunchPreparedSession(t.Context(), task.ToAPI(), session.ID,
		LaunchOptions{AgentProfileID: "profile-123", ExecutorID: models.ExecutorIDWorktree, StartAgent: true})
	require.NoError(t, err)
	require.Equal(t, 1, f.manager.launchAgentCallCount)
	require.Equal(t, f.receipt.ID, execution.WorkspaceInventoryRecoveryReceipt.ID)
	require.Equal(t, before, f.evidence(t))
	stored, err := f.repo.GetWorkspaceInventoryRepairReceipt(t.Context(), "task", "repair")
	require.NoError(t, err)
	require.Equal(t, f.receipt, stored)
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11
func TestResumeSessionWithOptionsAllowsBorrowedAttestedBranchEvolution(t *testing.T) {
	for _, mode := range []string{"inherit_parent", "shared_group"} {
		t.Run(mode, func(t *testing.T) {
			f := newBranchContinuityFixture(t)
			f.changeBranch(t, false)
			child := &models.Task{ID: "child", WorkspaceID: "workspace", Title: "Child",
				Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": mode}}}
			require.NoError(t, f.repo.CreateTask(t.Context(), child))
			require.NoError(t, f.repo.CreateTaskRepository(t.Context(), &models.TaskRepository{
				ID: "child-repository", TaskID: child.ID, RepositoryID: "repository", BaseBranch: "main",
			}))
			session := *f.session
			session.ID, session.TaskID = "child-session", child.ID
			require.NoError(t, f.repo.CreateTaskSession(t.Context(), &session))
			execution, err := f.executor.ResumeSessionWithOptions(t.Context(), &session, true, ResumeOptions{})
			require.NoError(t, err)
			require.Equal(t, 1, f.manager.launchAgentCallCount)
			require.Equal(t, f.receipt.ID, execution.WorkspaceInventoryRecoveryReceipt.ID)
		})
	}
}

func TestResumeSessionWithOptionsAllowsAttestedBranchEvolutionWithLiveSibling(t *testing.T) {
	f := newBranchContinuityFixture(t)
	f.changeBranch(t, false)
	sibling := *f.session
	sibling.ID, sibling.State = "live-sibling", models.TaskSessionStateRunning
	require.NoError(t, f.repo.CreateTaskSession(t.Context(), &sibling))
	_, err := f.executor.ResumeSessionWithOptions(t.Context(), f.session, true, ResumeOptions{})
	require.NoError(t, err, "completed repair checks do not need an exclusive environment claim")
	require.Equal(t, 1, f.manager.launchAgentCallCount)
}

func (f *branchContinuityFixture) changeBranch(t *testing.T, rename bool) {
	t.Helper()
	const branch = "feature/continued"
	if rename {
		runExecutorGit(t, f.row.WorktreePath, "branch", "-m", branch)
	} else {
		runExecutorGit(t, f.row.WorktreePath, "checkout", "-b", branch)
	}
	require.NoError(t, f.repo.UpdateTaskSessionWorktreeBranchByWorktree(t.Context(), f.session.ID, f.row.WorktreeID, branch))
	rows, err := f.repo.ListTaskEnvironmentRepos(t.Context(), f.env.ID)
	require.NoError(t, err)
	f.env.Repos, f.row = rows, rows[0]
}

func (f *branchContinuityFixture) evidence(t *testing.T) *worktree.PreservationEvidence {
	t.Helper()
	evidence, err := inspectWorkspaceInventoryCandidate(t.Context(), f.info, f.row)
	require.NoError(t, err)
	return evidence
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11
func TestResumeSessionWithOptionsAllowsAttestedBranchEvolution(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(map[bool]string{false: "switch", true: "rename"}[rename], func(t *testing.T) {
			f := newBranchContinuityFixture(t)
			f.changeBranch(t, rename)
			require.NoError(t, os.WriteFile(filepath.Join(f.row.WorktreePath, "README.md"), []byte("later commit\n"), 0o644))
			runExecutorGit(t, f.row.WorktreePath, "commit", "-am", "later work")
			require.NoError(t, os.WriteFile(filepath.Join(f.row.WorktreePath, "README.md"), []byte("staged\n"), 0o644))
			runExecutorGit(t, f.row.WorktreePath, "add", "README.md")
			require.NoError(t, os.WriteFile(filepath.Join(f.row.WorktreePath, "README.md"), []byte("unstaged\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(f.row.WorktreePath, "design.md"), []byte("untracked design\n"), 0o644))
			before := f.evidence(t)
			receiptBefore, err := json.Marshal(f.receipt)
			require.NoError(t, err)

			_, err = f.executor.ResumeSessionWithOptions(t.Context(), f.session, true, ResumeOptions{})
			require.NoError(t, err, "ordinary Resume must allow a proven branch change after completed repair")
			require.Equal(t, 1, f.manager.launchAgentCallCount)
			require.Equal(t, before, f.evidence(t), "admission must preserve current work")
			stored, err := f.repo.GetWorkspaceInventoryRepairReceipt(t.Context(), "task", "repair")
			require.NoError(t, err)
			receiptAfter, err := json.Marshal(stored)
			require.NoError(t, err)
			require.JSONEq(t, string(receiptBefore), string(receiptAfter), "historical receipt must remain unchanged")
		})
	}
}
