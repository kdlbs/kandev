package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRelaunchDynamicTaskAfterFailurePreservesActingOfficeIdentity(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-identity"
		sessionID   = "session-dynamic-identity"
		executionID = "execution-dynamic-identity"
	)

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	workspace := &models.Workspace{
		ID:               "ws1",
		Name:             "Test",
		OfficeWorkflowID: "wf1",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	require.NoError(t, repo.CreateWorkspace(ctx, workspace))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID:          "wf1",
		WorkspaceID: "ws1",
		Name:        "Test Workflow",
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	task := &models.Task{
		ID:          taskID,
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Title:       "Test Task",
		Description: "desc",
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        sessionID,
		TaskID:    taskID,
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}))
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	session.AgentProfileID = "assignee"
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	var launchRequest *executor.LaunchAgentRequest
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = task.ToAPI()
	agentManager := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchRequest = req
			return &executor.LaunchAgentResponse{AgentExecutionID: "successor-execution"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentManager)
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})

	require.True(t, svc.relaunchDynamicTaskAfterFailure(ctx, watcher.AgentEventData{
		TaskID:           taskID,
		SessionID:        sessionID,
		AgentExecutionID: executionID,
		AgentProfileID:   "reviewer",
	}, "fallback-profile"))
	require.NotNil(t, launchRequest)
	require.Equal(t, "reviewer", launchRequest.OfficeAgentProfileID,
		"dynamic successor must keep the acting Office identity from the failure event")
	require.Equal(t, "fallback-profile", launchRequest.AgentProfileID)
}
