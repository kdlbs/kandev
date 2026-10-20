package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A prepared session whose runtime is bound to another execution profile is
// replaced in place: the lifecycle stop synchronously removes the runtime row
// and delivers agent.stopped, and the launch must still register its new
// execution on the same session.
func TestLaunchPreparedSession_ProfileReplacementKeepsSessionLaunchable(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-replace", "session-replace", "step-replace")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "session-replace", models.TaskSessionStateCreated, ""))
	session, err := repo.GetTaskSession(ctx, "session-replace")
	require.NoError(t, err)
	session.AgentProfileID = "agent-prepared"
	session.ExecutionProfileID = "agent-prepared"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "runtime-replace", TaskID: session.TaskID, SessionID: session.ID,
		Status:             models.ExecutorRunningStatusPrepared,
		AgentExecutionID:   "execution-prepared",
		ExecutionProfileID: "agent-prepared",
	}))

	manager := &mockAgentManager{repoForExecutionLookup: repo}
	taskRepo := newMockTaskRepo()
	task := &v1.Task{ID: session.TaskID, WorkspaceID: "ws1", Title: "Replace", State: v1.TaskStateInProgress}
	taskRepo.tasks[task.ID] = task
	svc := NewService(DefaultServiceConfig(), nil, manager, taskRepo, repo, nil, nil, nil, testLogger())
	manager.stopAgentWithReasonFunc = func(stopCtx context.Context, executionID, _ string, _ bool) error {
		// Mirrors the lifecycle manager: the runtime row is removed and the
		// stopped event is handled before StopAgentWithReason returns.
		if err := repo.DeleteExecutorRunningBySessionID(stopCtx, session.ID); err != nil {
			return err
		}
		svc.handleAgentStopped(stopCtx, watcher.AgentEventData{
			TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: executionID,
		})
		return nil
	}
	var launchedProfile string
	manager.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchedProfile = req.AgentProfileID
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-replacement"}, nil
	}

	_, err = svc.executor.LaunchPreparedSession(ctx, task, session.ID, executor.LaunchOptions{
		AgentProfileID: "profile-routed", Prompt: "run", StartAgent: true,
	})

	require.NoError(t, err)
	assert.Equal(t, "profile-routed", launchedProfile)
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.NotEqual(t, models.TaskSessionStateCancelled, current.State,
		"the profile-replacement stop must not end the session being relaunched")
}
