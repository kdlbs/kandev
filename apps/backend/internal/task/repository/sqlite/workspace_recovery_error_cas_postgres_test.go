package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// PostgreSQL runs the recovery-error CAS against real row locks and JSONB
// metadata updates. It skips unless KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresCommitWorkspaceRecoveryErrorPreservesSuccessorState(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	const taskID, sessionID, environmentID = "task-workspace-recovery-cas-pg", "session-workspace-recovery-cas-pg", "environment-workspace-recovery-cas-pg"
	seedPostgresTaskSession(t, repo, taskID, sessionID)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusCreating,
	}); err != nil {
		t.Fatalf("seed selected environment: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE task_sessions SET state = ?, task_environment_id = ? WHERE id = ?`),
		models.TaskSessionStateCancelled, environmentID, sessionID); err != nil {
		t.Fatalf("seed cancelled session: %v", err)
	}

	observation := models.WorkspaceRecoveryErrorObservation{
		TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
		EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
		SessionState: models.TaskSessionStateCancelled,
	}
	observation.SelectionSnapshot = workspaceRecoverySelectionSnapshotForTest(t, repo, sessionID, environmentID)
	projected := models.LastAgentError{
		Message: "workspace relocation required", OccurredAt: time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC),
		Scope: models.ErrorScopeSession, Code: models.LaunchErrorCategoryManagedCloneRelocationRequired,
		RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: "relocation-pg-stamp",
	}
	stored, stamp, err := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, projected)
	if err != nil || !stored || stamp != projected.Stamp() {
		t.Fatalf("initial projection = (%v, %q, %v), want stored relocation stamp", stored, stamp, err)
	}

	// Repeated refusal must return the active relocation stamp even when the
	// caller observed the generic error before the first writer committed.
	observation.ExpectedErrorStamp = "old-generic-stamp"
	stored, stamp, err = repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, projected)
	if err != nil || stored || stamp != projected.Stamp() {
		t.Fatalf("duplicate projection = (%v, %q, %v), want reused relocation stamp", stored, stamp, err)
	}

	// A real successor executor is serialized through the session row and must
	// prevent a delayed inspection result from replacing the current session.
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: "execution-successor",
		Status: models.ExecutorRunningStatusStarting,
	}); err != nil {
		t.Fatalf("seed successor execution: %v", err)
	}
	observation.ExpectedErrorStamp = projected.Stamp()
	observation.AgentExecutionID = ""
	stored, stamp, err = repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, models.LastAgentError{
		Message: "late inspection result", OccurredAt: time.Now().UTC(), StampValue: "late-pg-stamp",
	})
	if err != nil || stored || stamp != "" {
		t.Fatalf("successor projection = (%v, %q, %v), want no write and no actionable stamp", stored, stamp, err)
	}

	// Owner generation changes defeat a delayed observation, even if the
	// selected environment ID is unchanged.
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE task_environments SET ownership_generation = ? WHERE id = ?`), 2, environmentID); err != nil {
		t.Fatalf("advance environment generation: %v", err)
	}
	observation.AgentExecutionID = "execution-successor"
	stored, stamp, err = repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, models.LastAgentError{
		Message: "late ownership result", OccurredAt: time.Now().UTC(), StampValue: "late-owner-pg-stamp",
	})
	if err != nil || stored || stamp != "" {
		t.Fatalf("stale-owner projection = (%v, %q, %v), want no write", stored, stamp, err)
	}

	final, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("read final session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(final.Metadata)
	if !ok || lastError.Stamp() != projected.Stamp() || final.State != models.TaskSessionStateCancelled {
		t.Fatalf("successor state changed: state=%s last_error=%+v", final.State, lastError)
	}
}

func TestPostgresCommitWorkspaceRecoveryErrorRejectsChangedSelectedInventory(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	for _, test := range []struct {
		name      string
		suffix    string
		seedError bool
		change    string
	}{
		{name: "delayed refusal after worktree change", suffix: "worktree-delayed", change: "worktree"},
		{name: "duplicate stamp after worktree change", suffix: "worktree-duplicate", seedError: true, change: "worktree"},
		{name: "delayed refusal after sibling slot addition", suffix: "sibling-delayed", change: "sibling_slot"},
		{name: "duplicate stamp after sibling slot addition", suffix: "sibling-duplicate", seedError: true, change: "sibling_slot"},
	} {
		t.Run(test.name, func(t *testing.T) {
			suffix := test.suffix
			workspaceID := "workspace-recovery-inventory-cas-pg-" + suffix
			taskID := "task-recovery-inventory-cas-pg-" + suffix
			sessionID := "session-recovery-inventory-cas-pg-" + suffix
			environmentID := "environment-recovery-inventory-cas-pg-" + suffix
			repositoryID := "repository-recovery-inventory-cas-pg-" + suffix
			slotID := "environment-repository-recovery-inventory-cas-pg-" + suffix
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Recovery inventory PG"}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Recovery inventory PG"}))
			require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
				ID: repositoryID, WorkspaceID: workspaceID, Name: "widget", LocalPath: "/managed/widget",
				Provider: "github", ProviderHost: "github.com", ProviderOwner: "acme", ProviderName: "widget",
			}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
				ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
				WorkspacePath: "/tasks/recovery-inventory-pg",
			}))
			slot := &models.TaskEnvironmentRepo{
				ID: slotID, TaskEnvironmentID: environmentID, RepositoryID: repositoryID,
				BranchSlug: "feature", WorktreeID: "worktree-before", WorktreePath: "/tasks/before",
				WorktreeBranch: "feature/before", WorktreeSourceClonePath: "/managed/old-widget",
				WorktreeSourceCommonDir: "/managed/old-widget/.git", Status: "active",
			}
			require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, slot))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
				State: models.TaskSessionStateCancelled,
			}))
			observation := models.WorkspaceRecoveryErrorObservation{
				TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
				EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
				SessionState: models.TaskSessionStateCancelled,
			}
			observation.SelectionSnapshot = workspaceRecoverySelectionSnapshotForTest(t, repo, sessionID, environmentID)
			projected := models.LastAgentError{
				Message: "workspace relocation required", OccurredAt: time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC),
				Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
				RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: "inventory-relocation-pg-stamp",
			}
			if test.seedError {
				stored, stamp, reportErr := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, projected)
				require.NoError(t, reportErr)
				require.True(t, stored)
				require.Equal(t, projected.Stamp(), stamp)
				observation.ExpectedErrorStamp = "older-generic-stamp"
			}
			switch test.change {
			case "worktree":
				slot.WorktreeID = "worktree-after"
				slot.WorktreePath = "/tasks/after"
				slot.WorktreeBranch = "feature/after"
				require.NoError(t, repo.UpdateTaskEnvironmentRepo(ctx, slot))
			case "sibling_slot":
				siblingID := "repository-recovery-inventory-cas-pg-sibling-" + suffix
				require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
					ID: siblingID, WorkspaceID: workspaceID, Name: "sibling", LocalPath: "/managed/sibling",
				}))
				require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
					ID:                "environment-repository-recovery-inventory-cas-pg-sibling-" + suffix,
					TaskEnvironmentID: environmentID, RepositoryID: siblingID, BranchSlug: "main",
					WorktreeID: "worktree-sibling-" + suffix, WorktreePath: "/tasks/sibling",
					WorktreeBranch: "feature/sibling", Status: "active", Position: 1,
				}))
			}
			stored, stamp, reportErr := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, projected)
			require.NoError(t, reportErr)
			require.False(t, stored)
			require.Empty(t, stamp)
			session, getErr := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, getErr)
			require.Equal(t, models.TaskSessionStateCancelled, session.State)
			lastError, hasError := models.LoadLastAgentError(session.Metadata)
			if test.seedError {
				require.True(t, hasError)
				require.Equal(t, projected.Stamp(), lastError.Stamp())
			} else {
				require.False(t, hasError)
			}
		})
	}
}
