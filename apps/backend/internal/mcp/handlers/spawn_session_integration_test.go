package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type spawnReuseFixture struct {
	handler      *Handlers
	orchestrator *orchestrator.Service
	repo         *sqliterepo.Repository
	runtime      *peerLaunchAgentManager
	sender, task *models.Task
	source       *models.TaskSession
}

// Use the existing SQLite service and runtime-boundary fixtures. Session
// preparation, workflow routing, persistence and MCP response are all real.
func newSpawnReuseFixture(t *testing.T) spawnReuseFixture {
	t.Helper()
	ctx := context.Background()
	svc, repo, _, workflowRepo, _, eventBus := newTestTaskServiceWithWorkflowDBAndEventBus(t)
	sender, task, source := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	work := seedWorkflowStep(t, ctx, workflowRepo, &wfmodels.WorkflowStep{
		ID: "spawn-work", WorkflowID: task.WorkflowID, Name: "Work", Position: 0,
		AgentProfileID: source.AgentProfileID,
	})
	blocked := seedWorkflowStep(t, ctx, workflowRepo, &wfmodels.WorkflowStep{
		ID: "spawn-blocked", WorkflowID: task.WorkflowID, Name: "Blocked", Position: 1,
		AgentProfileID:            "blocked-profile",
		SessionTarget:             &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: work.ID},
		ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse,
	})
	task.WorkflowStepID = blocked.ID
	require.NoError(t, repo.UpdateTask(ctx, task))
	require.NoError(t, repo.RenameTaskSession(ctx, source.ID, "original"))
	updated, err := repo.UpsertWorkflowSessionBinding(ctx, &models.WorkflowSessionBinding{
		TaskID: task.ID, WorkflowID: task.WorkflowID, TargetKey: "step:" + work.ID,
		AgentProfileID: source.AgentProfileID, SessionID: source.ID, OperationID: "spawn-source-entry",
	})
	require.NoError(t, err)
	require.True(t, updated)
	entryID, err := repo.GetLatestTaskStepTransitionID(ctx, task.ID)
	require.NoError(t, err)
	entryIdentity := fmt.Sprintf("entry:%020d", entryID)
	require.NoError(t, repo.SetTaskMetadataKey(ctx, task.ID, models.MetaKeyWorkflowSessionRoute, models.WorkflowSessionRoute{
		OperationID:       fmt.Sprintf("workflow-session:%s:%s:%s:step:%s:reuse", task.ID, blocked.ID, entryIdentity, work.ID),
		EntryIdentity:     entryIdentity,
		DestinationStepID: blocked.ID, TargetKind: "step", TargetStepID: work.ID,
		AgentProfileID: source.AgentProfileID, DestinationID: source.ID, Phase: "committed",
	}))
	runtime := &peerLaunchAgentManager{
		runtimeStore: repo, launchEntered: make(chan *executor.LaunchAgentRequest, 1),
		releaseLaunch: make(chan struct{}), initialAccepted: make(chan string, 1),
		processStarted: make(chan struct{}, 1), followUpAccepted: make(chan string, 1),
	}
	close(runtime.releaseLaunch)
	orch := orchestrator.NewService(orchestrator.DefaultServiceConfig(), eventBus, runtime,
		&mcpReadinessSchedulerTaskRepo{repo: repo}, repo, nil, nil, nil, testLogger(t))
	orch.SetWorkflowStepGetter(&peerMessageWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{work.ID: work, blocked.ID: blocked}})
	handler := &Handlers{taskSvc: svc, taskRepo: repo, sessionRepo: repo, sessionLauncher: orch, logger: testLogger(t)}
	return spawnReuseFixture{handler, orch, repo, runtime, sender, task, source}
}

func TestHandleSpawnSession_DistinctUnderWorkflowReuse(t *testing.T) {
	for _, tc := range []struct {
		name, requested, expected string
		crossTask                 bool
	}{
		{name: "explicit different profile", requested: "requested-profile", expected: "requested-profile"},
		{name: "explicit same profile", requested: "agent-profile-1", expected: "agent-profile-1"},
		{name: "inherit same-task caller", expected: "agent-profile-1"},
		{name: "inherit target primary", crossTask: true, expected: "agent-profile-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSpawnReuseFixture(t)
			ctx := context.Background()
			senderID, senderSessionID := f.task.ID, f.source.ID
			if tc.crossTask {
				senderID, senderSessionID = f.sender.ID, "sender-sess-1"
			}
			payload := spawnPayload(f.task.ID, "inspect independently", senderID, senderSessionID)
			payload["agent_profile_id"], payload["name"] = tc.requested, "reviewer"
			before, err := f.repo.ListTaskSessions(ctx, f.task.ID)
			require.NoError(t, err)
			response, err := f.handler.handleSpawnSession(ctx, makeWSMessage(t, ws.ActionMCPSpawnSession, payload))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
			var out orchestrator.LaunchSessionResponse
			require.NoError(t, json.Unmarshal(response.Payload, &out))
			require.NotEqual(t, f.source.ID, out.SessionID, "explicit spawn must not reuse the workflow source session")
			verifyDistinctSpawn(t, f, out, tc.expected, len(before))
		})
	}
}

func verifyDistinctSpawn(t *testing.T, f spawnReuseFixture, out orchestrator.LaunchSessionResponse, expected string, beforeCount int) {
	t.Helper()
	ctx := context.Background()
	session, err := f.repo.GetTaskSession(ctx, out.SessionID)
	require.NoError(t, err)
	require.Equal(t, f.task.ID, session.TaskID)
	require.Equal(t, expected, session.AgentProfileID)
	require.Equal(t, session.AgentProfileID, out.AgentProfileID)
	require.Equal(t, "reviewer", session.Name)
	sessions, err := f.repo.ListTaskSessions(ctx, f.task.ID)
	require.NoError(t, err)
	require.Len(t, sessions, beforeCount+1)
	launched := <-f.runtime.launchEntered
	require.Equal(t, session.ID, launched.SessionID)
	require.Equal(t, session.AgentProfileID, launched.AgentProfileID)
	original, err := f.repo.GetTaskSession(ctx, f.source.ID)
	require.NoError(t, err)
	require.Equal(t, "original", original.Name)
	require.Equal(t, f.source.AgentProfileID, original.AgentProfileID)
	binding, err := f.repo.GetWorkflowSessionBinding(ctx, f.task.ID, "step:spawn-work")
	require.NoError(t, err)
	require.Equal(t, f.source.ID, binding.SessionID)
	task, err := f.repo.GetTask(ctx, f.task.ID)
	require.NoError(t, err)
	route, found := models.LoadWorkflowSessionRoute(task.Metadata)
	require.True(t, found)
	require.Equal(t, f.task.WorkflowStepID, task.WorkflowStepID)
	require.Equal(t, f.source.ID, route.DestinationID, "spawn must leave the recorded workflow route intact")
}

func TestHandleSpawnSession_OrdinaryWorkflowReuseControl(t *testing.T) {
	f := newSpawnReuseFixture(t)
	ctx := context.Background()
	response, err := f.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID: f.task.ID, Intent: orchestrator.IntentStart, AgentProfileID: "requested-profile", Prompt: "workflow launch",
	})
	require.NoError(t, err)
	require.Equal(t, f.source.ID, response.SessionID)
	require.Equal(t, f.source.AgentProfileID, response.AgentProfileID)
	sessions, err := f.repo.ListTaskSessions(ctx, f.task.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
}

func TestHandleSpawnSession_EmptyProfileDoesNotReuseWorkflowSession(t *testing.T) {
	f := newSpawnReuseFixture(t)
	ctx := context.Background()
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "empty-primary", TaskID: f.task.ID, IsPrimary: true, State: models.TaskSessionStateCreated,
	}))
	require.NoError(t, f.repo.SetSessionPrimary(ctx, "empty-primary"))
	payload := spawnPayload(f.task.ID, "independent launch", f.sender.ID, "sender-sess-1")
	response, err := f.handler.handleSpawnSession(ctx, makeWSMessage(t, ws.ActionMCPSpawnSession, payload))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
	require.Contains(t, string(response.Payload), executor.ErrNoAgentProfileID.Error())
	sessions, err := f.repo.ListTaskSessions(ctx, f.task.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 2, "missing launch defaults must not allocate or reuse a session")
	require.Empty(t, f.runtime.launchEntered)
}

func TestHandleSpawnSession_EmptyProfileKeepsNormalLaunchDefaults(t *testing.T) {
	f := newSpawnReuseFixture(t)
	ctx := context.Background()
	require.NoError(t, f.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "empty-primary", TaskID: f.task.ID, IsPrimary: true, State: models.TaskSessionStateCreated,
	}))
	require.NoError(t, f.repo.SetSessionPrimary(ctx, "empty-primary"))
	// A normal step without SessionTarget still provides its launch default.
	f.orchestrator.SetWorkflowStepGetter(&peerMessageWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		f.task.WorkflowStepID: {ID: f.task.WorkflowStepID, WorkflowID: f.task.WorkflowID, AgentProfileID: "default-profile"},
	}})
	payload := spawnPayload(f.task.ID, "independent launch", f.sender.ID, "sender-sess-1")
	payload["name"] = "reviewer"
	response, err := f.handler.handleSpawnSession(ctx, makeWSMessage(t, ws.ActionMCPSpawnSession, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
	var out orchestrator.LaunchSessionResponse
	require.NoError(t, json.Unmarshal(response.Payload, &out))
	require.NotEqual(t, f.source.ID, out.SessionID)
	verifyDistinctSpawn(t, f, out, "default-profile", 2)
}

func TestHandleSpawnSession_PreservesOfficeSchedulerGuard(t *testing.T) {
	f := newSpawnReuseFixture(t)
	ctx := context.Background()
	task, err := f.repo.GetTask(ctx, f.task.ID)
	require.NoError(t, err)
	task.ProjectID = "office-project"
	require.NoError(t, f.repo.UpdateTask(ctx, task))
	payload := spawnPayload(task.ID, "independent launch", task.ID, f.source.ID)
	payload["agent_profile_id"] = "requested-profile"
	response, err := f.handler.handleSpawnSession(ctx, makeWSMessage(t, ws.ActionMCPSpawnSession, payload))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeInternalError)
	require.Contains(t, string(response.Payload), "start or wake the task through Office")
	sessions, err := f.repo.ListTaskSessions(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Empty(t, f.runtime.launchEntered)
}
