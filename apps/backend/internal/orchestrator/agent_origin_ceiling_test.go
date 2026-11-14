package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const agentOriginCeilingHolder = "agent-origin-ceiling-holder"

// newFullCeilingService returns a service whose one-slot session ceiling is
// already held by an automatic start, plus a counter of agent launches.
func newFullCeilingService(t *testing.T, seeds map[string]models.TaskSessionState) (*Service, *sqliterepo.Repository, *atomic.Int32) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	taskRepo := newMockTaskRepo()
	seeds[agentOriginCeilingHolder] = models.TaskSessionStateCompleted
	for taskID, state := range seeds {
		seedTaskAndSession(t, repo, taskID, taskID+"-session", state)
		seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	}
	var launches atomic.Int32
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches.Add(1)
			return &executor.LaunchAgentResponse{AgentExecutionID: "agent-origin-execution"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.sessionCeiling = newSessionCeilingController(1, nil, nil)
	_, err := svc.StartTask(ctx, agentOriginCeilingHolder, "profile-1", "", "", "", "go", "", false, true, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, launches.Load())
	return svc, repo, &launches
}

// An agent that starts a task through Kandev MCP is not a direct user action,
// so a full ceiling defers its launch instead of admitting it as a manual
// override.
func TestLaunchSession_AgentOriginStartOverCeilingIsDeferred(t *testing.T) {
	ctx := context.Background()
	const taskID = "agent-origin-start"
	svc, _, launches := newFullCeilingService(t, map[string]models.TaskSessionState{
		taskID: models.TaskSessionStateCompleted,
	})

	resp, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: taskID, Intent: IntentStart, AgentProfileID: "profile-1", Prompt: "go", Automatic: true,
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, launches.Load(), "an agent-origin launch must not ride the manual override")
	require.NotNil(t, resp)
	require.Equal(t, activationDispositionQueued, resp.ActivationDisposition)
	record := deferredLaunchOf(t, svc, taskID)
	require.NotNil(t, record, "the refused launch must be kept for replay")
	require.Equal(t, true, record[models.CeilingDeferredKey])
}

// The same holds for an agent-requested start of a prepared session, and the
// recorded payload keeps the automatic class for its replay.
func TestLaunchSession_AgentOriginStartCreatedOverCeilingIsDeferred(t *testing.T) {
	ctx := context.Background()
	const taskID = "agent-origin-start-created"
	svc, _, launches := newFullCeilingService(t, map[string]models.TaskSessionState{
		taskID: models.TaskSessionStateCreated,
	})

	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: taskID, Intent: IntentStartCreated, SessionID: taskID + "-session",
		AgentProfileID: "profile-1", Prompt: "go", Automatic: true,
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, launches.Load(), "an agent-origin launch must not ride the manual override")
	record := deferredLaunchOf(t, svc, taskID)
	require.NotNil(t, record, "the refused launch must be kept for replay")
	payload, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	require.Equal(t, string(launchOriginAutomatic), payload["origin"])
}

// A user's own start without the agent marker keeps the manual override.
func TestLaunchSession_UserStartOverCeilingKeepsManualOverride(t *testing.T) {
	ctx := context.Background()
	const taskID = "agent-origin-user-start"
	svc, _, launches := newFullCeilingService(t, map[string]models.TaskSessionState{
		taskID: models.TaskSessionStateCompleted,
	})

	_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: taskID, Intent: IntentStart, AgentProfileID: "profile-1", Prompt: "go",
	})

	require.NoError(t, err)
	require.EqualValues(t, 2, launches.Load())
	require.Nil(t, deferredLaunchOf(t, svc, taskID))
}

// Resume and prepare read the activation origin; an agent-origin resume, such
// as an MCP follow-up to a completed session, is automatic.
func TestLaunchOriginForActivation_AgentOriginIsAutomatic(t *testing.T) {
	require.Equal(t, launchOriginAutomatic, launchOriginForActivation(&LaunchSessionRequest{Automatic: true}))
	require.Equal(t, launchOriginManual, launchOriginForActivation(&LaunchSessionRequest{}))
	require.Equal(t, launchOriginManual, launchOriginForActivation(nil))
}
