package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestStartTaskWithEnv_DuplicateOfficeLaunchKeepsRunningSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)
	task, err := repo.GetTask(ctx, "task1")
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	session.AgentProfileID = "office-runner"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, "session1", "task1", "live-execution")
	before, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, "live-execution", before.AgentExecutionID)
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task1"] = &v1.Task{ID: "task1", Title: "Office task", State: v1.TaskStateInProgress}
	manager := &mockAgentManager{
		isAgentRunning: true,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			t.Fatal("a duplicate launch must not dispatch another agent")
			return nil, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	_, err = svc.StartTaskWithEnv(ctx, "task1", "office-runner", "", "", "", "Do the work", "", false, false, nil, validOfficeRuntimeEnv())
	require.ErrorIs(t, err, executor.ErrExecutionAlreadyRunning)
	after, err := repo.GetTaskSession(ctx, "session1")
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, after.State)
	require.Equal(t, "live-execution", after.AgentExecutionID)
	require.Empty(t, after.ErrorMessage)
	sessions, err := repo.ListTaskSessions(ctx, "task1")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}
