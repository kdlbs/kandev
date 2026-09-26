package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimeagentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.2
func TestPeerMessageInitialLaunch_Delivery(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	originalTurn, err := svc.StartTurn(ctx, session.ID)
	require.NoError(t, err)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.startCreatedErr = executor.ErrExecutionAlreadyRunning
	require.NoError(t, orch.queue.SetAutoRun(ctx, session.ID, false))

	metadata := map[string]interface{}{"sender_task_id": sender.ID, "sender_session_id": "sender-sess-1"}
	result, err := h.dispatchPreparedTaskMessage(
		ctx,
		target.ID,
		session,
		"follow up during initial launch",
		metadata,
	)

	require.NoError(t, err)
	assert.Equal(t, taskMessageStatusQueued, result.status)
	assert.Equal(t, 1, orch.queue.GetStatus(ctx, session.ID).Count)
	entry := orch.queue.GetStatus(ctx, session.ID).Entries[0]
	assert.Equal(t, "follow up during initial launch", entry.Content)
	assert.Equal(t, sender.ID, entry.Metadata["sender_task_id"])
	assert.Equal(t, "sender-sess-1", entry.Metadata["sender_session_id"])

	messages, err := svc.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, messages, "the queued follow-up must not remain as an undispatched user turn")
	activeTurn, err := svc.GetActiveTurn(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	assert.Equal(t, originalTurn.ID, activeTurn.ID, "message rollback must preserve the initial launch's active turn")
	require.Len(t, orch.startCreatedCalls, 1)
	require.Len(t, orch.readinessCalls, 1)
	assert.Equal(t, session.QueueIncarnationID, orch.readinessCalls[0].SessionIncarnationID)
	reserved, found, autoRun := orch.queue.ReserveQueuedWithAutoRun(ctx, session.ID)
	assert.False(t, autoRun)
	assert.False(t, found)
	assert.Nil(t, reserved, "Auto-run OFF must leave the accepted follow-up pending")
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.2
func TestPeerMessageInitialLaunch_HandlerToRuntime(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, taskSvc, repo, models.TaskSessionStateCreated)
	log := testLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	identity := messagequeue.QueueSessionIdentity{
		TaskID: target.ID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
	}
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(
		func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
			current, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return messagequeue.QueueSessionIdentity{}, err
			}
			if current == nil || current.TaskID != taskID || current.QueueIncarnationID == "" {
				return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
			}
			return messagequeue.QueueSessionIdentity{
				TaskID: taskID, SessionID: sessionID, SessionIncarnationID: current.QueueIncarnationID,
			}, nil
		},
	)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	agentManager := &peerLaunchAgentManager{
		runtimeStore:     repo,
		launchEntered:    make(chan *executor.LaunchAgentRequest, 1),
		releaseLaunch:    make(chan struct{}),
		initialAccepted:  make(chan string, 1),
		processStarted:   make(chan struct{}, 1),
		followUpAccepted: make(chan string, 1),
	}
	service := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, agentManager,
		&mcpReadinessSchedulerTaskRepo{repo: repo}, repo, nil, nil, queue, log,
	)
	h := &Handlers{
		taskSvc: taskSvc, sessionRepo: repo, taskRepo: repo,
		sessionLauncher: service, logger: log.WithFields(),
	}

	firstMessage := makeWSMessage(
		t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "original implementation brief", sender.ID),
	)
	firstResult := make(chan struct {
		response *ws.Message
		err      error
	}, 1)
	go func() {
		response, err := h.handleMessageTask(ctx, firstMessage)
		firstResult <- struct {
			response *ws.Message
			err      error
		}{response: response, err: err}
	}()

	var launchRequest *executor.LaunchAgentRequest
	select {
	case launchRequest = <-agentManager.launchEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial launch did not reach the controlled runtime")
	}
	require.Contains(t, launchRequest.TaskDescription, "original implementation brief")

	followUpResponse, err := h.handleMessageTask(ctx, makeWSMessage(
		t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "follow-up during bootstrap", sender.ID),
	))
	require.NoError(t, err)
	var followUpPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(followUpResponse.Payload, &followUpPayload))
	assert.Equal(t, taskMessageStatusQueued, followUpPayload[stopTaskStatusKey])
	queued := queue.GetStatus(ctx, session.ID)
	require.Len(t, queued.Entries, 1)
	assert.Contains(t, queued.Entries[0].Content, "follow-up during bootstrap")
	assert.Equal(t, sender.ID, queued.Entries[0].Metadata["sender_task_id"])
	assert.Equal(t, "sender-sess-1", queued.Entries[0].Metadata["sender_session_id"])

	close(agentManager.releaseLaunch)
	select {
	case accepted := <-agentManager.initialAccepted:
		assert.Contains(t, accepted, "original implementation brief")
	case <-time.After(3 * time.Second):
		t.Fatal("runtime did not accept the original brief")
	}
	select {
	case result := <-firstResult:
		require.NoError(t, result.err)
		var firstPayload map[string]interface{}
		require.NoError(t, json.Unmarshal(result.response.Payload, &firstPayload))
		assert.Equal(t, "started", firstPayload[stopTaskStatusKey])
	case <-time.After(3 * time.Second):
		t.Fatal("initial launch handler did not complete")
	}
	select {
	case <-agentManager.processStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("initial runtime process did not start")
	}

	// Model the first turn settling after startup. The same identity is used by
	// the readiness recheck that drains the accepted follow-up.
	require.NoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""))
	require.NoError(t, repo.UpdateExecutorRunningStatus(ctx, session.ID, models.ExecutorRunningStatusRunning))
	require.NoError(t, repo.UpdateTaskState(ctx, target.ID, v1.TaskStateInProgress))
	service.CheckQueueAdmissionReadiness(ctx, identity)
	select {
	case accepted := <-agentManager.followUpAccepted:
		assert.Contains(t, accepted, "follow-up during bootstrap")
	case <-time.After(3 * time.Second):
		t.Fatal("queued follow-up was not accepted after the initial turn settled")
	}
	assert.Zero(t, queue.GetStatus(ctx, session.ID).Count)

	messages, err := taskSvc.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	assert.Equal(t, sender.ID, messages[0].Metadata["sender_task_id"])
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.2
func TestPeerMessageInitialLaunch_QueuesBeforeWorkflowTurnPreparation(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, taskSvc, repo, models.TaskSessionStateCreated)
	task, err := taskSvc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.WorkflowStepID = "step-a"
	require.NoError(t, repo.UpdateTask(ctx, task))
	turn, err := taskSvc.StartTurn(ctx, session.ID)
	require.NoError(t, err)

	log := testLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(func() { eventBus.Close() })
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(
		func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
			current, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				return messagequeue.QueueSessionIdentity{}, err
			}
			if current == nil || current.TaskID != taskID || current.QueueIncarnationID == "" {
				return messagequeue.QueueSessionIdentity{}, messagequeue.ErrSessionIdentityMismatch
			}
			return messagequeue.QueueSessionIdentity{
				TaskID: taskID, SessionID: sessionID, SessionIncarnationID: current.QueueIncarnationID,
			}, nil
		},
	)
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	agentManager := &peerLaunchAgentManager{
		runtimeStore:     repo,
		launchEntered:    make(chan *executor.LaunchAgentRequest, 1),
		releaseLaunch:    make(chan struct{}),
		initialAccepted:  make(chan string, 1),
		processStarted:   make(chan struct{}, 1),
		followUpAccepted: make(chan string, 1),
	}
	service := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, agentManager,
		&mcpReadinessSchedulerTaskRepo{repo: repo}, repo, nil, nil, queue, log,
	)
	service.SetTurnService(taskSvc)
	steps := &peerMessageWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-a": {
			ID: "step-a", WorkflowID: target.WorkflowID, Name: "Backlog", Position: 0,
			Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}}},
		},
		"step-b": {
			ID: "step-b", WorkflowID: target.WorkflowID, Name: "Implementation", Position: 1,
			Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}}},
		},
		"step-c": {
			ID: "step-c", WorkflowID: target.WorkflowID, Name: "Review", Position: 2,
		},
	}}
	service.SetWorkflowStepGetter(steps)
	if err := queue.SetAutoRun(ctx, session.ID, false); err != nil {
		require.NoError(t, err)
	}

	// The original initial-create request advances step-a to step-b, then owns
	// the session while its runtime start is blocked. Its session remains
	// WAITING_FOR_INPUT until runtime startup publishes the execution identity.
	// The follow-up must not advance step-b to step-c.
	initialStart := make(chan error, 1)
	var releaseOnce sync.Once
	releaseInitial := func() { releaseOnce.Do(func() { close(agentManager.releaseLaunch) }) }
	t.Cleanup(releaseInitial)
	go func() {
		_, startErr := service.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
			TaskID: target.ID, SessionID: session.ID, Intent: orchestrator.IntentStartCreated,
			AgentProfileID: session.AgentProfileID, Prompt: "original implementation brief",
			InitialCreatePrompt: true, AutoStart: true,
		})
		initialStart <- startErr
	}()
	select {
	case <-agentManager.launchEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial creation launch did not reach the controlled runtime")
	}
	metadata := map[string]interface{}{
		models.SessionMetaKeyInitialCreatePromptPassthrough: map[string]interface{}{
			"queue_incarnation_id": session.QueueIncarnationID,
			"execution_bound":      false,
			"turn_id":              turn.ID,
		},
		models.SessionMetaKeyPendingStepCompletion: models.PendingStepCompletionSignal{
			StepID: "step-b", Source: models.StepCompletionSourceAgent, Summary: "initial launch signal",
			SignaledAt: time.Now().UTC(),
		},
		"launch_owner_marker": "original-create",
	}
	require.NoError(t, repo.UpdateSessionMetadata(ctx, session.ID, metadata))
	currentBefore, err := taskSvc.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	taskBefore, err := taskSvc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	stepReadsBefore := steps.reads.Load()

	h := &Handlers{
		taskSvc: taskSvc, sessionRepo: repo, taskRepo: repo,
		sessionLauncher: service, logger: log.WithFields(),
	}
	response, err := h.handleMessageTask(ctx, makeWSMessage(
		t, ws.ActionMCPMessageTask,
		senderPayload(target.ID, "follow-up while creation launch owns session", sender.ID),
	))
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	assert.Equal(t, taskMessageStatusQueued, payload[stopTaskStatusKey])

	updatedTask, err := taskSvc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, "step-b", updatedTask.WorkflowStepID, "the losing message must not evaluate the second on_turn_start transition")
	assert.Equal(t, taskBefore.State, updatedTask.State)
	updatedSession, err := taskSvc.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, currentBefore.State, updatedSession.State)
	assert.Equal(t, currentBefore.AgentProfileID, updatedSession.AgentProfileID)
	assert.Equal(t, currentBefore.IsPrimary, updatedSession.IsPrimary)
	assert.Equal(t, currentBefore.Metadata, updatedSession.Metadata, "initial prompt evidence and the pending signal must remain intact")
	primary, err := taskSvc.GetPrimarySession(ctx, target.ID)
	require.NoError(t, err)
	require.NotNil(t, primary)
	assert.Equal(t, session.ID, primary.ID, "the losing message must not change the task's primary recipient")
	activeTurn, err := taskSvc.GetActiveTurn(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	assert.Equal(t, turn.ID, activeTurn.ID)
	assert.Equal(t, stepReadsBefore, steps.reads.Load(), "busy admission must not evaluate workflow steps")
	queued := queue.GetStatus(ctx, session.ID)
	require.Len(t, queued.Entries, 1)
	assert.Contains(t, queued.Entries[0].Content, "follow-up while creation launch owns session")

	releaseInitial()
	select {
	case launchErr := <-initialStart:
		require.NoError(t, launchErr)
	case <-time.After(3 * time.Second):
		t.Fatal("initial creation launch did not finish after releasing runtime")
	}
}

type peerMessageWorkflowStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
	reads atomic.Int32
}

func (g *peerMessageWorkflowStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	g.reads.Add(1)
	return g.steps[stepID], nil
}

func (g *peerMessageWorkflowStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, position int) (*wfmodels.WorkflowStep, error) {
	for _, step := range g.steps {
		if step.WorkflowID == workflowID && step.Position > position {
			return step, nil
		}
	}
	return nil, nil
}

func (*peerMessageWorkflowStepGetter) GetPreviousStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

func (*peerMessageWorkflowStepGetter) GetWorkflowMeta(context.Context, string) (orchestrator.WorkflowMeta, error) {
	return orchestrator.WorkflowMeta{}, nil
}

type peerLaunchAgentManager struct {
	executor.AgentManagerClient
	runtimeStore interface {
		UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error
	}
	launchEntered    chan *executor.LaunchAgentRequest
	releaseLaunch    chan struct{}
	initialAccepted  chan string
	processStarted   chan struct{}
	followUpAccepted chan string
	running          atomic.Bool
}

func (m *peerLaunchAgentManager) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	m.launchEntered <- req
	select {
	case <-m.releaseLaunch:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if m.runtimeStore != nil {
		if err := m.runtimeStore.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:               "runtime-" + req.SessionID,
			TaskID:           req.TaskID,
			SessionID:        req.SessionID,
			Status:           models.ExecutorRunningStatusStarting,
			AgentExecutionID: "execution-" + req.SessionID,
		}); err != nil {
			return nil, err
		}
	}
	m.initialAccepted <- req.TaskDescription
	return &executor.LaunchAgentResponse{AgentExecutionID: "execution-" + req.SessionID}, nil
}

func (m *peerLaunchAgentManager) StartAgentProcess(context.Context, string) error {
	m.running.Store(true)
	m.processStarted <- struct{}{}
	return nil
}

func (*peerLaunchAgentManager) StopAgentWithReason(context.Context, string, string, bool) error {
	return nil
}

func (m *peerLaunchAgentManager) WaitForAgentctlReady(context.Context, string) error { return nil }

func (m *peerLaunchAgentManager) GetGitStatus(context.Context, string) (*runtimeagentctl.GitStatusResult, error) {
	return nil, nil
}

func (m *peerLaunchAgentManager) IsAgentRunningForSession(context.Context, string) bool {
	return m.running.Load()
}

func (m *peerLaunchAgentManager) IsAgentReadyForPrompt(context.Context, string) bool {
	return m.running.Load()
}

func (m *peerLaunchAgentManager) IsPassthroughSession(context.Context, string) bool { return false }

func (m *peerLaunchAgentManager) ResolveAgentProfile(_ context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{ProfileID: profileID}, nil
}

func (m *peerLaunchAgentManager) GetExecutionIDForSession(context.Context, string) (string, error) {
	return "execution-sess-1", nil
}

func (m *peerLaunchAgentManager) PromptAgent(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
) (*executor.PromptResult, error) {
	return m.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, nil)
}

func (m *peerLaunchAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	_ string,
	prompt string,
	_ []v1.MessageAttachment,
	_ bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	if onDispatched != nil {
		onDispatched()
	}
	select {
	case m.followUpAccepted <- prompt:
		return &executor.PromptResult{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.2
func TestPeerMessageInitialLaunch_PreservesOlderFIFOEntry(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.startCreatedErr = executor.ErrExecutionAlreadyRunning
	orch.queue.SetAutoMergeEnabled(false)

	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	older, err := orch.queue.QueueMessageWithMetadataForSession(
		ctx, identity, "older queued work", "", messagequeue.QueuedByAgent, false, nil,
		map[string]interface{}{"sender_task_id": sender.ID},
	)
	require.NoError(t, err)

	result, err := h.dispatchPreparedTaskMessage(
		ctx, target.ID, session, "follow-up", map[string]interface{}{"sender_task_id": sender.ID},
	)

	require.NoError(t, err)
	assert.Equal(t, taskMessageStatusQueued, result.status)
	entries := orch.queue.GetStatus(ctx, session.ID).Entries
	require.Len(t, entries, 2)
	assert.Equal(t, older.ID, entries[0].ID)
	assert.Equal(t, "older queued work", entries[0].Content)
	assert.Equal(t, "follow-up", entries[1].Content)
}

// @covers AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.3
func TestPeerMessageInitialLaunch_QueueRejectionDoesNotRollbackWinningProgress(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	initialTurn, err := svc.StartTurn(ctx, session.ID)
	require.NoError(t, err)
	task, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	task.State = v1.TaskStateReview
	task.WorkflowStepID = "before-message"
	task.Metadata = map[string]interface{}{"owner_marker": "before-message"}
	require.NoError(t, repo.UpdateTask(ctx, task))
	session.Metadata = map[string]interface{}{"session_owner_marker": "before-message"}
	require.NoError(t, repo.UpdateTaskSession(ctx, session))
	require.NoError(t, repo.UpdateSessionMetadata(ctx, session.ID, session.Metadata))
	session, err = svc.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)

	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.queue.SetAutoMergeEnabled(false)
	identity, err := orch.queue.ResolveSessionIdentity(ctx, target.ID, session.ID)
	require.NoError(t, err)
	initialQueueStatus := orch.queue.GetStatus(ctx, session.ID)
	for i := 0; i < initialQueueStatus.Max; i++ {
		_, err := orch.queue.QueueMessageWithMetadataForSession(
			ctx, identity, "pre-existing queued message", "", messagequeue.QueuedByAgent, false, nil,
			map[string]interface{}{"position": i},
		)
		require.NoError(t, err)
	}
	expectedQueue := orch.queue.GetStatus(ctx, session.ID).Entries

	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	orch.peerStartFunc = func(
		_ context.Context,
		_ messagequeue.QueueSessionIdentity,
		_, _ string,
		_, _, _ bool,
		_ []v1.MessageAttachment,
		_ []v1.EntityReference,
	) (*executor.TaskExecution, error) {
		close(startEntered)
		<-releaseStart
		return nil, executor.ErrExecutionAlreadyRunning
	}
	t.Cleanup(func() {
		select {
		case <-releaseStart:
		default:
			close(releaseStart)
		}
	})

	dispatchDone := make(chan struct {
		result taskMessageDispatchResult
		err    error
	}, 1)
	go func() {
		result, dispatchErr := h.dispatchTaskMessage(
			ctx, target.ID, session, "follow-up that loses launch admission",
			map[string]interface{}{"sender_task_id": sender.ID}, false, false,
		)
		dispatchDone <- struct {
			result taskMessageDispatchResult
			err    error
		}{result: result, err: dispatchErr}
	}()
	select {
	case <-startEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("peer message did not reach the controlled launch boundary")
	}

	winnerSession, err := svc.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	winnerSession.State = models.TaskSessionStateRunning
	winnerSession.AgentProfileID = "winning-profile"
	winnerSession.Metadata = map[string]interface{}{"session_owner_marker": "winning-launch"}
	require.NoError(t, repo.UpdateTaskSession(ctx, winnerSession))
	require.NoError(t, repo.UpdateSessionMetadata(ctx, session.ID, winnerSession.Metadata))
	winnerTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	winnerTask.State = v1.TaskStateInProgress
	winnerTask.WorkflowStepID = "winning-step"
	winnerTask.Metadata = map[string]interface{}{"owner_marker": "winning-launch"}
	require.NoError(t, repo.UpdateTask(ctx, winnerTask))
	close(releaseStart)

	var outcome struct {
		result taskMessageDispatchResult
		err    error
	}
	select {
	case outcome = <-dispatchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("peer-message dispatch did not return after the winning launch advanced")
	}
	var queueFull *queueFullDispatchError
	require.Error(t, outcome.err)
	assert.ErrorAs(t, outcome.err, &queueFull)

	updatedSession, err := svc.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.TaskSessionStateRunning, updatedSession.State)
	assert.Equal(t, "winning-profile", updatedSession.AgentProfileID)
	assert.Equal(t, "winning-launch", updatedSession.Metadata["session_owner_marker"])
	updatedTask, err := svc.GetTask(ctx, target.ID)
	require.NoError(t, err)
	assert.Equal(t, v1.TaskStateInProgress, updatedTask.State)
	assert.Equal(t, "winning-step", updatedTask.WorkflowStepID)
	assert.Equal(t, "winning-launch", updatedTask.Metadata["owner_marker"])
	activeTurn, err := svc.GetActiveTurn(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, activeTurn)
	assert.Equal(t, initialTurn.ID, activeTurn.ID)
	updatedQueue := orch.queue.GetStatus(ctx, session.ID).Entries
	require.Len(t, updatedQueue, len(expectedQueue))
	for i := range expectedQueue {
		assert.Equal(t, expectedQueue[i].ID, updatedQueue[i].ID)
		assert.Equal(t, expectedQueue[i].Content, updatedQueue[i].Content)
	}
	messages, err := svc.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	assert.Empty(t, messages, "a rejected follow-up must not remain as an undispatched user message")
}
