package executor

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

const metadataRepairKey = "repair"

func TestMetadataInventoryEnvironmentMatches(t *testing.T) {
	ready := func() *models.TaskEnvironment {
		return &models.TaskEnvironment{ID: "environment", TaskID: "task",
			ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady}
	}
	session := &models.TaskSession{ID: "session", TaskID: "task", TaskEnvironmentID: "environment"}
	tests := []struct {
		name   string
		taskID string
		env    func() *models.TaskEnvironment
		want   bool
	}{
		{name: "ready worktree environment of the task", taskID: "task", env: ready, want: true},
		{name: "missing environment", taskID: "task", env: func() *models.TaskEnvironment { return nil }},
		{name: "environment without id", taskID: "task", env: func() *models.TaskEnvironment { e := ready(); e.ID = ""; return e }},
		{name: "environment of another task", taskID: "task", env: func() *models.TaskEnvironment { e := ready(); e.TaskID = "task-other"; return e }},
		{name: "request for another task", taskID: "task-other", env: ready},
		{name: "session bound to another environment", taskID: "task", env: func() *models.TaskEnvironment { e := ready(); e.ID = "environment-other"; return e }},
		{name: "non-worktree executor", taskID: "task", env: func() *models.TaskEnvironment {
			e := ready()
			e.ExecutorType = string(models.ExecutorTypeLocal)
			return e
		}},
		{name: "environment not ready", taskID: "task", env: func() *models.TaskEnvironment {
			e := ready()
			e.Status = models.TaskEnvironmentStatusFailed
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, metadataInventoryEnvironmentMatches(tt.taskID, session, tt.env()))
		})
	}
}

// Every request outside the single-local-repository worktree boundary is
// refused before any inventory write and never launches an agent.
func TestRepairWorkspaceInventoryOnlyRejectsOutOfScopeRequests(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		emptyKey  bool
		mutate    func(*testing.T, *localMetadataRepairState)
		want      error
	}{
		{name: "empty idempotency key", emptyKey: true, want: models.ErrWorkspaceInventoryRecoveryInvalid},
		{name: "running session", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `UPDATE task_sessions SET state = ? WHERE id = ?`, string(models.TaskSessionStateRunning), "session")
		}},
		{name: "session of another task", sessionID: "session-other", mutate: func(t *testing.T, s *localMetadataRepairState) {
			require.NoError(t, s.repo.CreateTask(t.Context(), &models.Task{ID: "task-other", WorkspaceID: "workspace", Title: "Other"}))
			require.NoError(t, s.repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: "session-other", TaskID: "task-other", State: models.TaskSessionStateFailed,
			}))
		}},
		{name: "missing environment", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `DELETE FROM task_environment_repos WHERE task_environment_id = ?`, "environment")
			s.exec(t, `DELETE FROM task_environments WHERE id = ?`, "environment")
		}},
		{name: "session bound to another environment", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`, "environment-other", "session")
		}},
		{name: "non-worktree executor", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `UPDATE task_environments SET executor_type = ? WHERE id = ?`, string(models.ExecutorTypeLocal), "environment")
		}},
		{name: "environment not ready", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `UPDATE task_environments SET status = ? WHERE id = ?`, string(models.TaskEnvironmentStatusFailed), "environment")
		}},
		{name: "multiple task repositories", mutate: func(t *testing.T, s *localMetadataRepairState) {
			require.NoError(t, s.repo.CreateRepository(t.Context(), &models.Repository{
				ID: "repository-second", WorkspaceID: "workspace", Name: "second", SourceType: sourceTypeLocal, LocalPath: s.source,
			}))
			require.NoError(t, s.repo.CreateTaskRepository(t.Context(), &models.TaskRepository{
				ID: "task-repository-second", TaskID: "task", RepositoryID: "repository-second", BaseBranch: "main", Position: 1,
			}))
		}},
		{name: "repository of another workspace", mutate: func(t *testing.T, s *localMetadataRepairState) {
			require.NoError(t, s.repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace-other", Name: "Other"}))
			s.exec(t, `UPDATE repositories SET workspace_id = ? WHERE id = ?`, "workspace-other", "repository")
		}},
		{name: "non-local repository", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `UPDATE repositories SET source_type = ? WHERE id = ?`, "provider", "repository")
		}},
		{name: "local repository without path", mutate: func(t *testing.T, s *localMetadataRepairState) {
			s.exec(t, `UPDATE repositories SET local_path = ? WHERE id = ?`, "", "repository")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newLocalMetadataRepairState(t)
			if tt.mutate != nil {
				tt.mutate(t, state)
			}
			manager := &mockAgentManager{}
			executor := newTestExecutor(t, manager, state.repo)
			sessionID, key, want := "session", metadataRepairKey, models.ErrWorkspaceInventoryRecoveryConflict
			if tt.sessionID != "" {
				sessionID = tt.sessionID
			}
			if tt.emptyKey {
				key = ""
			}
			if tt.want != nil {
				want = tt.want
			}

			receipt, err := executor.RepairWorkspaceInventoryOnly(t.Context(), "task", sessionID, key)
			require.ErrorIs(t, err, want)
			require.Nil(t, receipt)
			require.Zero(t, manager.launchAgentCallCount)
			state.requireUnrepaired(t)
		})
	}
}

// fenceDriftRepository models a concurrent lifecycle writer that commits
// right after the executor captured the repair fence.
type fenceDriftRepository struct {
	*tasksqlite.Repository
	once  sync.Once
	drift func()
}

func (r *fenceDriftRepository) ReadWorkspaceInventoryRepairFence(ctx context.Context, taskID, sessionID string) (*models.WorkspaceInventoryRepairFence, error) {
	fence, err := r.Repository.ReadWorkspaceInventoryRepairFence(ctx, taskID, sessionID)
	r.once.Do(r.drift)
	return fence, err
}

func TestRepairWorkspaceInventoryOnlyRejectsFenceDriftBeforeRepairTransaction(t *testing.T) {
	later := time.Now().UTC().Add(time.Hour)
	tests := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "task updated", query: `UPDATE tasks SET updated_at = ? WHERE id = ?`, args: []any{later, "task"}},
		{name: "workflow step", query: `UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, args: []any{"step-moved", "task"}},
		{name: "session updated", query: `UPDATE task_sessions SET updated_at = ? WHERE id = ?`, args: []any{later, "session"}},
		{name: "session state", query: `UPDATE task_sessions SET state = ? WHERE id = ?`, args: []any{string(models.TaskSessionStateCancelled), "session"}},
		{name: "route appears", query: `INSERT INTO dynamic_route_states (session_id, logical_profile_id, route_generation, state, updated_at)
			VALUES (?, ?, ?, ?, ?)`, args: []any{"session", "logical", 1, "active", later}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newLocalMetadataRepairState(t)
			repo := &fenceDriftRepository{Repository: state.repo, drift: func() { state.exec(t, tt.query, tt.args...) }}
			manager := &mockAgentManager{}
			executor := newTestExecutor(t, manager, repo)

			receipt, err := executor.RepairWorkspaceInventoryOnly(t.Context(), "task", "session", metadataRepairKey)
			require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryConflict)
			require.Nil(t, receipt)
			require.Zero(t, manager.launchAgentCallCount)
			state.requireUnrepaired(t)
		})
	}

	t.Run("no drift commits", func(t *testing.T) {
		state := newLocalMetadataRepairState(t)
		repo := &fenceDriftRepository{Repository: state.repo, drift: func() {}}
		receipt, err := newTestExecutor(t, &mockAgentManager{}, repo).
			RepairWorkspaceInventoryOnly(t.Context(), "task", "session", metadataRepairKey)
		require.NoError(t, err)
		require.NotNil(t, receipt.Preservation.Fence)
	})
}

// Pins the CURRENT behavior of a same-key retry: an unchanged fence replays
// the committed receipt, while a retry after the fence changed is refused as
// an idempotency conflict and leaves the committed receipt in place. Whether
// that refusal is the intended contract is an open owner decision; update
// this test together with that decision.
func TestRepairWorkspaceInventoryOnlySameKeyRetryCurrentBehavior(t *testing.T) {
	state := newLocalMetadataRepairState(t)
	manager := &mockAgentManager{}
	executor := newTestExecutor(t, manager, state.repo)
	first, err := executor.RepairWorkspaceInventoryOnly(t.Context(), "task", "session", metadataRepairKey)
	require.NoError(t, err)

	replay, err := executor.RepairWorkspaceInventoryOnly(t.Context(), "task", "session", metadataRepairKey)
	require.NoError(t, err, "a same-key retry with an unchanged fence replays the committed receipt")
	require.Equal(t, first.ID, replay.ID)

	state.exec(t, `UPDATE tasks SET updated_at = ? WHERE id = ?`, time.Now().UTC().Add(time.Hour), "task")
	_, err = executor.RepairWorkspaceInventoryOnly(t.Context(), "task", "session", metadataRepairKey)
	require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryIdempotencyConflict)
	stored, err := state.repo.GetWorkspaceInventoryRepairReceipt(t.Context(), "task", metadataRepairKey)
	require.NoError(t, err)
	require.Equal(t, first.ID, stored.ID)
	require.Zero(t, manager.launchAgentCallCount)
}

// The metadata-only request claims the environment without the ordinary
// launch path's allowance for the requesting session's own live runtime.
func TestMetadataOnlyInventoryClaimRefusesCurrentSessionRuntime(t *testing.T) {
	state := newLocalMetadataRepairState(t)
	now := time.Now().UTC()
	require.NoError(t, state.repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{
		ID: "running", SessionID: "session", TaskID: "task", Status: models.ExecutorRunningStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}))
	manager := &mockAgentManager{}
	executor := newTestExecutor(t, manager, state.repo)
	env, err := state.repo.GetTaskEnvironment(t.Context(), "environment")
	require.NoError(t, err)

	_, release, err := executor.claimInventoryEnvironment(t.Context(), env, "session")
	require.NoError(t, err, "the ordinary path tolerates the requesting session's own runtime")
	require.NoError(t, release())

	fenced := context.WithValue(t.Context(), inventoryMetadataKey{}, &models.WorkspaceInventoryRepairFence{})
	_, _, err = executor.claimInventoryEnvironment(fenced, env, "session")
	require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryConflict)

	receipt, err := executor.RepairWorkspaceInventoryOnly(t.Context(), "task", "session", metadataRepairKey)
	require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryConflict)
	require.ErrorContains(t, err, "acquire environment preservation claim")
	require.Nil(t, receipt)
	require.Zero(t, manager.launchAgentCallCount)
	state.requireUnrepaired(t)
}

func TestValidateScopedInventoryCandidate(t *testing.T) {
	source, checkout := createExecutorPreservationFixture(t)
	otherSource, _ := createExecutorPreservationFixture(t)
	fenced := context.WithValue(context.Background(), inventoryMetadataKey{}, &models.WorkspaceInventoryRepairFence{})
	tests := []struct {
		name       string
		ctx        context.Context
		sourceType string
		configured string
		repoPath   string
		workspace  string
		candidate  string
		wantErr    bool
	}{
		{name: "fenced local checkout", ctx: fenced, sourceType: sourceTypeLocal, configured: source, repoPath: source, workspace: checkout, candidate: checkout},
		{name: "unfenced local checkout keeps automatic repair disabled", ctx: context.Background(), sourceType: sourceTypeLocal,
			configured: source, repoPath: source, workspace: checkout, candidate: checkout, wantErr: true},
		{name: "fenced non-local checkout uses the ordinary rule", ctx: fenced, sourceType: "provider",
			configured: source, repoPath: source, workspace: checkout, candidate: checkout},
		{name: "checkout is the repository itself", ctx: fenced, sourceType: sourceTypeLocal,
			configured: source, repoPath: source, workspace: source, candidate: source, wantErr: true},
		{name: "checkout outside the task workspace", ctx: fenced, sourceType: sourceTypeLocal,
			configured: source, repoPath: source, workspace: filepath.Join(t.TempDir(), "workspace"), candidate: checkout, wantErr: true},
		{name: "resolved path differs from the configured repository", ctx: fenced, sourceType: sourceTypeLocal,
			configured: source, repoPath: otherSource, workspace: checkout, candidate: checkout, wantErr: true},
		{name: "repository path is not a Git repository", ctx: fenced, sourceType: sourceTypeLocal,
			configured: source, repoPath: t.TempDir(), workspace: checkout, candidate: checkout, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := &models.TaskEnvironment{ID: "environment", TaskID: "task", WorkspacePath: tt.workspace}
			info := &repoInfo{RepositoryPath: tt.repoPath,
				Repository: &models.Repository{ID: "repository", SourceType: tt.sourceType, LocalPath: tt.configured}}
			err := validateScopedInventoryCandidate(tt.ctx, env, info, &models.TaskEnvironmentRepo{WorktreePath: tt.candidate})
			if tt.wantErr {
				require.ErrorIs(t, err, models.ErrWorkspaceInventoryRecoveryConflict)
				return
			}
			require.NoError(t, err)
		})
	}
}

func (s *localMetadataRepairState) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	_, err := s.db.ExecContext(context.Background(), s.db.Rebind(query), args...)
	require.NoError(t, err)
}

func (s *localMetadataRepairState) requireUnrepaired(t *testing.T) {
	t.Helper()
	receipt, err := s.repo.GetWorkspaceInventoryRepairReceipt(context.Background(), "task", metadataRepairKey)
	require.NoError(t, err)
	require.Nil(t, receipt, "a refused request must not commit a repair receipt")
	rows, err := s.repo.ListTaskEnvironmentRepos(context.Background(), "environment")
	require.NoError(t, err)
	for _, row := range rows {
		require.Equal(t, "stale", row.BranchSlug, "a refused request must leave the inventory row untouched")
	}
}
