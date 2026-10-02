package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCommitWorkspaceRecoveryErrorPreservesSuccessorState(t *testing.T) {
	tests := []struct {
		name         string
		initialStamp string
		mutate       func(*testing.T, *Repository, string, string)
		wantStored   bool
		wantStamp    string
	}{
		{
			name:       "absent typed error with legacy message",
			wantStored: true,
			wantStamp:  "relocation-stamp",
		},
		{
			name:         "equal error stamp",
			initialStamp: "observed-stamp",
			wantStored:   true,
			wantStamp:    "relocation-stamp",
		},
		{
			name:         "newer error stamp",
			initialStamp: "observed-stamp",
			mutate: func(t *testing.T, repo *Repository, sessionID, _ string) {
				require.NoError(t, repo.SetSessionMetadataKey(context.Background(), sessionID, models.SessionMetaKeyLastAgentError,
					models.LastAgentError{Message: "successor", OccurredAt: time.Now().UTC(), StampValue: "successor-stamp"}))
			},
			wantStored: false,
		},
		{
			name:         "session state changed",
			initialStamp: "observed-stamp",
			mutate: func(t *testing.T, repo *Repository, sessionID, _ string) {
				_, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET state = ? WHERE id = ?`), models.TaskSessionStateRunning, sessionID)
				require.NoError(t, err)
			},
		},
		{
			name:         "execution changed",
			initialStamp: "observed-stamp",
			mutate: func(t *testing.T, repo *Repository, sessionID, taskID string) {
				require.NoError(t, repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
					ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: "successor-execution",
				}))
			},
		},
		{
			name:         "selected environment changed",
			initialStamp: "observed-stamp",
			mutate: func(t *testing.T, repo *Repository, sessionID, _ string) {
				_, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`), "other-environment", sessionID)
				require.NoError(t, err)
			},
		},
		{
			name:         "owner generation changed",
			initialStamp: "observed-stamp",
			mutate: func(t *testing.T, repo *Repository, _ string, _ string) {
				_, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_environments SET ownership_generation = ? WHERE id = ?`), 2, "environment-recovery-cas")
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			const (
				taskID        = "task-recovery-cas"
				sessionID     = "session-recovery-cas"
				environmentID = "environment-recovery-cas"
			)
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Recovery CAS"}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
				ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
			}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "other-task", Title: "Other task"}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: "other-environment", TaskID: "other-task", OwnershipGeneration: 1,
				ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
			}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
				State: models.TaskSessionStateCancelled, ErrorMessage: "legacy process error",
				Metadata: map[string]interface{}{"retained": "value", "provider": map[string]interface{}{"token": "keep"}},
			}))
			if tt.initialStamp != "" {
				require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError,
					models.LastAgentError{Message: "observed", OccurredAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), StampValue: tt.initialStamp}))
			}
			if tt.mutate != nil {
				tt.mutate(t, repo, sessionID, taskID)
			}

			observation := models.WorkspaceRecoveryErrorObservation{
				TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
				EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
				SessionState:       models.TaskSessionStateCancelled,
				ExpectedErrorStamp: tt.initialStamp,
			}
			observation.SelectionSnapshot = workspaceRecoverySelectionSnapshotForTest(t, repo, sessionID, environmentID)
			stored, stamp, err := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, models.LastAgentError{
				Message:    "The task workspace contains local changes and needs explicit relocation.",
				OccurredAt: time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC),
				Scope:      models.ErrorScopeSession, Phase: models.LaunchErrorPhaseBootstrap,
				Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
				RecoveryActions: []string{models.RecoveryActionRelocateAndResume},
				StampValue:      "relocation-stamp",
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantStored, stored)
			require.Equal(t, tt.wantStamp, stamp)

			session, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, "legacy process error", session.ErrorMessage)
			require.Equal(t, "value", session.Metadata["retained"])
			require.Equal(t, map[string]interface{}{"token": "keep"}, session.Metadata["provider"])
			switch {
			case tt.wantStored:
				require.Equal(t, models.TaskSessionStateCancelled, session.State)
				lastErr, ok := models.LoadLastAgentError(session.Metadata)
				require.True(t, ok)
				require.Equal(t, "relocation-stamp", lastErr.Stamp())
			case tt.name == "newer error stamp":
				lastErr, ok := models.LoadLastAgentError(session.Metadata)
				require.True(t, ok)
				require.Equal(t, "successor-stamp", lastErr.Stamp())
			case tt.name == "session state changed":
				require.Equal(t, models.TaskSessionStateRunning, session.State)
			}
		})
	}

	t.Run("duplicate refusal reuses active stamp without a write", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		ctx := context.Background()
		const taskID, sessionID, environmentID = "task-recovery-duplicate", "session-recovery-duplicate", "environment-recovery-duplicate"
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Recovery duplicate"}))
		require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: environmentID, TaskID: taskID, OwnershipGeneration: 1}))
		require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateCancelled,
		}))
		observation := models.WorkspaceRecoveryErrorObservation{
			TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
			EnvironmentOwnerTaskID: taskID, OwnershipGeneration: 1,
			SessionState: models.TaskSessionStateCancelled,
		}
		observation.SelectionSnapshot = workspaceRecoverySelectionSnapshotForTest(t, repo, sessionID, environmentID)
		newError := models.LastAgentError{
			Message: "relocation required", OccurredAt: time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC),
			Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
			RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: "same-relocation-stamp",
		}
		stored, stamp, err := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, newError)
		require.NoError(t, err)
		require.True(t, stored)
		require.Equal(t, "same-relocation-stamp", stamp)

		observation.ExpectedErrorStamp = "obsolete-generic-stamp"
		stored, stamp, err = repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, newError)
		require.NoError(t, err)
		require.False(t, stored)
		require.Equal(t, "same-relocation-stamp", stamp)
	})
}

func workspaceRecoverySelectionSnapshotForTest(
	t *testing.T,
	repo *Repository,
	sessionID, environmentID string,
) models.WorkspaceRecoverySelectionSnapshot {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	environment, err := repo.GetTaskEnvironment(context.Background(), environmentID)
	require.NoError(t, err)
	repositories := make(map[string]*models.Repository, len(environment.Repos))
	for _, slot := range environment.Repos {
		if slot == nil || slot.RepositoryID == "" || slot.DeletedAt != nil || (slot.Status != "" && slot.Status != "active") {
			continue
		}
		entity, getErr := repo.GetRepository(context.Background(), slot.RepositoryID)
		require.NoError(t, getErr)
		if entity != nil {
			repositories[slot.RepositoryID] = entity
		}
	}
	return models.NewWorkspaceRecoverySelectionSnapshot(session, environment, repositories)
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-002.4, AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
func TestCommitWorkspaceRecoveryErrorRejectsChangedSelectedInventory(t *testing.T) {
	tests := []struct {
		name       string
		seedError  bool
		change     string
		wantStored bool
		wantStamp  string
	}{
		{name: "delayed refusal after worktree change", change: "worktree"},
		{name: "duplicate stamp after worktree change", seedError: true, change: "worktree"},
		{name: "delayed refusal after repository path change", change: "repository_path"},
		{name: "duplicate stamp after repository path change", seedError: true, change: "repository_path"},
		{name: "delayed refusal after sibling slot addition", change: "sibling_slot"},
		{name: "duplicate stamp after sibling slot addition", seedError: true, change: "sibling_slot"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			const (
				workspaceID   = "workspace-recovery-inventory"
				taskID        = "task-recovery-inventory"
				sessionID     = "session-recovery-inventory"
				environmentID = "environment-recovery-inventory"
				repositoryID  = "repository-recovery-inventory"
				slotID        = "environment-repository-recovery-inventory"
			)
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Recovery inventory"}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Recovery inventory"}))
			require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
				ID: repositoryID, WorkspaceID: workspaceID, Name: "widget", LocalPath: "/managed/widget",
				Provider: "github", ProviderHost: "github.com", ProviderOwner: "acme", ProviderName: "widget",
			}))
			require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
				ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
				ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusReady,
				WorkspacePath: "/tasks/recovery-inventory",
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
			require.True(t, observation.SelectionSnapshot.Complete(), "%+v", observation.SelectionSnapshot)
			inventoryTx, beginErr := repo.db.BeginTxx(ctx, nil)
			require.NoError(t, beginErr)
			currentSlots, readErr := repo.readWorkspaceRecoveryInventoryForUpdate(ctx, inventoryTx, environmentID)
			require.NoError(t, readErr)
			require.NoError(t, inventoryTx.Rollback())
			require.Equal(t, observation.SelectionSnapshot.Canonical().Slots, currentSlots)
			projected := models.LastAgentError{
				Message: "relocation required", OccurredAt: time.Date(2026, 10, 2, 1, 0, 0, 0, time.UTC),
				Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
				RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: "inventory-relocation-stamp",
			}
			if tt.seedError {
				stored, stamp, err := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, projected)
				require.NoError(t, err)
				require.True(t, stored)
				require.Equal(t, projected.Stamp(), stamp)
				observation.ExpectedErrorStamp = "older-generic-stamp"
			}

			switch tt.change {
			case "worktree":
				slot.WorktreeID = "worktree-after"
				slot.WorktreePath = "/tasks/after"
				slot.WorktreeBranch = "feature/after"
				require.NoError(t, repo.UpdateTaskEnvironmentRepo(ctx, slot))
			case "repository_path":
				registered, getErr := repo.GetRepository(ctx, repositoryID)
				require.NoError(t, getErr)
				registered.LocalPath = "/managed/widget-moved"
				require.NoError(t, repo.UpdateRepository(ctx, registered))
			case "sibling_slot":
				require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
					ID: "repository-recovery-inventory-sibling", WorkspaceID: workspaceID,
					Name: "sibling", LocalPath: "/managed/sibling",
				}))
				require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
					ID: "environment-repository-recovery-inventory-sibling", TaskEnvironmentID: environmentID,
					RepositoryID: "repository-recovery-inventory-sibling", BranchSlug: "main",
					WorktreeID: "worktree-sibling", WorktreePath: "/tasks/sibling",
					WorktreeBranch: "feature/sibling", Status: "active", Position: 1,
				}))
			}

			stored, stamp, err := repo.CommitWorkspaceRecoveryErrorIfCurrent(ctx, observation, projected)
			require.NoError(t, err)
			require.Equal(t, tt.wantStored, stored)
			require.Equal(t, tt.wantStamp, stamp)
			session, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			lastError, hasError := models.LoadLastAgentError(session.Metadata)
			if tt.seedError {
				require.True(t, hasError)
				require.Equal(t, projected.Stamp(), lastError.Stamp(), "inventory drift must not make an old actionable stamp reusable")
			} else {
				require.False(t, hasError, "stale inventory must not install an actionable relocation error")
			}
			require.Equal(t, models.TaskSessionStateCancelled, session.State)
		})
	}
}
