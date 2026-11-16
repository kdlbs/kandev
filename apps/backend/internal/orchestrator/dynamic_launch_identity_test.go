package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const (
	officeRelaunchTaskID      = "task1"
	officeRelaunchSessionID   = "session-dynamic-identity"
	officeRelaunchExecutionID = "execution-dynamic-identity"
)

// newOfficeRelaunchFixture seeds a running Office session whose execution
// failed and returns a service whose launches record their request.
func newOfficeRelaunchFixture(t *testing.T) (*Service, *sqliterepo.Repository, **executor.LaunchAgentRequest) {
	t.Helper()
	ctx := context.Background()
	const (
		taskID      = officeRelaunchTaskID
		sessionID   = officeRelaunchSessionID
		executionID = officeRelaunchExecutionID
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
	svc.turnService = &repoTurnService{repo: repo}
	svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})
	return svc, repo, &launchRequest
}

func relaunchOfficeAfterFailure(t *testing.T, svc *Service) {
	t.Helper()
	require.True(t, svc.relaunchDynamicTaskAfterFailure(context.Background(), watcher.AgentEventData{
		TaskID:           officeRelaunchTaskID,
		SessionID:        officeRelaunchSessionID,
		AgentExecutionID: officeRelaunchExecutionID,
		AgentProfileID:   "reviewer",
	}, "fallback-profile", launchOriginAutomatic))
}

func TestRelaunchDynamicTaskAfterFailurePreservesActingOfficeIdentity(t *testing.T) {
	svc, _, captured := newOfficeRelaunchFixture(t)
	relaunchOfficeAfterFailure(t, svc)
	launchRequest := *captured
	require.NotNil(t, launchRequest)
	require.Equal(t, "reviewer", launchRequest.OfficeAgentProfileID,
		"dynamic successor must keep the acting Office identity from the failure event")
	require.Equal(t, "fallback-profile", launchRequest.AgentProfileID)
}

// The Office successor prompt must own a durable turn, like the first Office
// launch. Without one its completion has no turn ID, so the workflow effect
// has no key and on_turn_complete is deferred with nothing to retry it.
func TestRelaunchDynamicOfficeTaskBindsSuccessorTurn(t *testing.T) {
	svc, repo, captured := newOfficeRelaunchFixture(t)
	relaunchOfficeAfterFailure(t, svc)
	launchRequest := *captured
	require.NotNil(t, launchRequest)
	require.NotEmpty(t, launchRequest.TurnID, "Office successor prompt must carry its turn")

	active, err := repo.GetActiveTurnBySessionID(context.Background(), officeRelaunchSessionID)
	require.NoError(t, err)
	require.Equal(t, active.ID, launchRequest.TurnID, "the successor turn must stay open for its completion")
}
