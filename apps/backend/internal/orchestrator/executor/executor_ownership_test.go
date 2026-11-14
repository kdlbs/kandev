package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestExecutorOwnershipPreparedLaunchAndResumeRecheck(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{ID: "private-session", TaskID: "private-task", ExecutorProfileID: "private-profile"}
	repo.sessions[session.ID] = session
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	denied := errors.New("private executor denied")
	executor.SetExecutorAdmission(func(ctx context.Context, sessionID, _, profileID string) (context.Context, error) {
		require.Equal(t, session.ID, sessionID)
		require.Equal(t, session.ExecutorProfileID, profileID)
		return ctx, denied
	})
	_, err := executor.LaunchPreparedSession(t.Context(), &v1.Task{ID: session.TaskID}, session.ID, LaunchOptions{})
	require.ErrorIs(t, err, denied)
	_, err = executor.ResumeSessionWithOptions(t.Context(), session, true, ResumeOptions{})
	require.ErrorIs(t, err, denied)
}

func TestExecutorOwnershipPreparationChecksActualInheritedEnvironment(t *testing.T) {
	repo := newMockRepository()
	repo.taskEnvironments["inherited"] = &models.TaskEnvironment{ID: "inherited", TaskID: "parent", ExecutorID: "private-parent", ExecutorProfileID: "private-profile"}
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	denied := errors.New("inherited private executor denied")
	executor.SetExecutorAdmission(func(ctx context.Context, _, _, profileID string) (context.Context, error) {
		if profileID == "private-profile" {
			return ctx, denied
		}
		return ctx, nil
	})
	_, err := executor.prepareExecutorAdmission(t.Context(), &v1.Task{ID: "child"}, &models.TaskSession{TaskEnvironmentID: "inherited"}, executorConfig{ExecutorID: "shared", Metadata: map[string]any{}})
	require.ErrorIs(t, err, denied)
}

func TestPreflightSessionExecutorAdmissionUsesTransientSession(t *testing.T) {
	repo := newMockRepository()
	repo.executors["executor-1"] = &models.Executor{
		ID: "executor-1", Type: models.ExecutorTypeLocal,
	}
	repo.executorProfiles["profile-1"] = &models.ExecutorProfile{
		ID: "profile-1", ExecutorID: "executor-1",
	}
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	calls := 0
	executor.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		calls++
		require.Empty(t, sessionID)
		require.Equal(t, "executor-1", executorID)
		require.Equal(t, "profile-1", profileID)
		return ctx, nil
	})

	err := executor.PreflightSessionExecutorAdmission(
		t.Context(), &v1.Task{ID: "task-1", Metadata: map[string]interface{}{"existing": true}},
		"executor-1", "profile-1", "",
	)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Empty(t, repo.sessions, "admission preflight must not persist a session")
}

func TestPreflightSessionExecutorAdmissionWithEmptyTaskMetadata(t *testing.T) {
	repo := newMockRepository()
	repo.executors["executor-1"] = &models.Executor{ID: "executor-1", Type: models.ExecutorTypeLocal}
	repo.executorProfiles["profile-1"] = &models.ExecutorProfile{ID: "profile-1", ExecutorID: "executor-1"}
	executor := newTestExecutor(t, &mockAgentManager{}, repo)
	executor.SetExecutorAdmission(func(ctx context.Context, sessionID, executorID, profileID string) (context.Context, error) {
		require.Empty(t, sessionID)
		require.Equal(t, "executor-1", executorID)
		require.Equal(t, "profile-1", profileID)
		return ctx, nil
	})

	err := executor.PreflightSessionExecutorAdmission(
		t.Context(), &v1.Task{ID: "task-1"}, "executor-1", "profile-1", "",
	)
	require.NoError(t, err)
}
