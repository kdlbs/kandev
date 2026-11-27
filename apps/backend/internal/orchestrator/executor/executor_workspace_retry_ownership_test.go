package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
func TestPrepareSessionRetryUsesCommittedPrimaryOwnership(t *testing.T) {
	repo := &workspacePreparingRepo{mockRepository: newMockRepository(), boundEnvironmentID: "env-retry"}
	repo.taskEnvironments["env-retry"] = &models.TaskEnvironment{
		ID: "env-retry", TaskID: "task-retry", Status: models.TaskEnvironmentStatusReady,
	}
	winner := &models.TaskSession{ID: "winner", TaskID: "task-retry", IsPrimary: true}
	repo.onBinding = func(call int) error {
		if call == 1 {
			return preparingRefusal("env-retry")
		}
		// Another retry commits after the caller's refresh and before insertion.
		require.NoError(t, repo.CreateTaskSession(context.Background(), winner))
		return nil
	}
	repo.onSession = func(session *models.TaskSession) {
		session.IsPrimary = false
		delete(session.Metadata, models.SessionMetaKeyOrigin)
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	primaryEvents := 0
	exec.onPrimarySessionSet = func(context.Context, string, string) { primaryEvents++ }
	task := &v1.Task{ID: "task-retry", WorkspaceID: "ws-retry", Title: "retry"}
	sessionID, err := exec.PrepareSession(context.Background(), task, "profile-1", "executor-1", "", "")
	require.NoError(t, err)
	require.False(t, repo.sessions[sessionID].IsPrimary)
	require.True(t, repo.sessions[winner.ID].IsPrimary)
	require.Empty(t, repo.setSessionPrimaryCalls)
	require.Zero(t, primaryEvents)
	require.False(t, models.IsOriginalTaskSession(repo.sessions[sessionID].Metadata))
}

func TestRetryPreparedSessionDoesNotCreateAfterCancellation(t *testing.T) {
	repo := newMockRepository()
	repo.taskEnvironments["ready"] = &models.TaskEnvironment{ID: "ready", Status: models.TaskEnvironmentStatusReady}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	created := false
	err := exec.retryPreparedSessionAfterWorkspaceReady(ctx, "session", preparingRefusal("ready"), func() error {
		created = true
		return nil
	})
	require.ErrorIs(t, err, models.ErrWorkspacePreparing)
	require.False(t, created)
}

func TestRetryPreparedSessionMissingEnvironmentKeepsRefusal(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	created := false
	err := exec.retryPreparedSessionAfterWorkspaceReady(context.Background(), "session", preparingRefusal("missing"), func() error {
		created = true
		return nil
	})
	require.ErrorIs(t, err, models.ErrWorkspacePreparing)
	require.False(t, created)
}

func TestRetryPreparedSessionRetriesOnlyOnce(t *testing.T) {
	repo := newMockRepository()
	repo.taskEnvironments["ready"] = &models.TaskEnvironment{ID: "ready", Status: models.TaskEnvironmentStatusReady}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	calls := 0
	refusal := preparingRefusal("ready")
	err := exec.retryPreparedSessionAfterWorkspaceReady(context.Background(), "session", refusal, func() error {
		calls++
		return refusal
	})
	require.Same(t, refusal, err)
	require.Equal(t, 1, calls)
}

func TestRefreshRetriedSessionOrderKeepsExplicitRuntimeOverrides(t *testing.T) {
	for _, appliedInitial := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit", true: "initial seed"}[appliedInitial], func(t *testing.T) {
			repo := newMockRepository()
			require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
				ID: "winner", TaskID: "task", IsPrimary: true,
			}))
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			config := models.SessionRuntimeConfig{Model: "selected-model"}
			session := &models.TaskSession{ID: "loser", TaskID: "task", IsPrimary: true,
				Metadata: map[string]interface{}{
					models.SessionMetaKeyOrigin:                 models.SessionOriginTaskInitial,
					models.SessionMetaKeyRuntimeConfigOverrides: config,
				},
			}
			primary, err := exec.refreshRetriedSessionOrder(context.Background(), session, true, appliedInitial)
			require.NoError(t, err)
			require.False(t, primary)
			require.False(t, models.IsOriginalTaskSession(session.Metadata))
			actual, exists := session.Metadata[models.SessionMetaKeyRuntimeConfigOverrides]
			require.Equal(t, !appliedInitial, exists)
			if exists {
				require.Equal(t, config, actual)
			}
		})
	}
}
