package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/acp/protocol"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const mcpStopResponseDeadline = time.Second

type mcpStopRecordingAgentManager struct {
	*SimulatedAgentManagerClient
	promptCalls  atomic.Int32
	stopCalls    atomic.Int32
	stopEntered  chan struct{}
	allowStop    chan struct{}
	stopFinished chan struct{}
	enterOnce    sync.Once
	finishOnce   sync.Once
	releaseOnce  sync.Once
}

func newMCPStopRecordingAgentManager(eventBus bus.EventBus, log *logger.Logger) *mcpStopRecordingAgentManager {
	return &mcpStopRecordingAgentManager{
		SimulatedAgentManagerClient: NewSimulatedAgentManager(eventBus, log),
		stopEntered:                 make(chan struct{}),
		allowStop:                   make(chan struct{}),
		stopFinished:                make(chan struct{}),
	}
}

func (m *mcpStopRecordingAgentManager) StartAgentProcess(
	ctx context.Context,
	executionID string,
) error {
	if err := m.SimulatedAgentManagerClient.StartAgentProcess(ctx, executionID); err != nil {
		return err
	}
	m.mu.Lock()
	instance := m.instances[executionID]
	m.mu.Unlock()
	if instance != nil {
		instance.statusMu.Lock()
		instance.status = v1.AgentStatusRunning
		instance.statusMu.Unlock()
	}
	return nil
}

func (m *mcpStopRecordingAgentManager) PromptAgent(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
) (*executor.PromptResult, error) {
	m.promptCalls.Add(1)
	return m.SimulatedAgentManagerClient.PromptAgent(
		ctx, executionID, prompt, attachments, dispatchOnly,
	)
}

func (m *mcpStopRecordingAgentManager) StopAgentWithReason(
	ctx context.Context,
	executionID, reason string,
	force bool,
) error {
	m.stopCalls.Add(1)
	m.enterOnce.Do(func() { close(m.stopEntered) })
	<-m.allowStop
	err := m.SimulatedAgentManagerClient.StopAgentWithReason(ctx, executionID, reason, force)
	m.finishOnce.Do(func() { close(m.stopFinished) })
	return err
}

func (m *mcpStopRecordingAgentManager) IsAgentRunningForSession(
	_ context.Context,
	sessionID string,
) bool {
	status, ok := m.runtimeStatus(sessionID)
	return ok && status == v1.AgentStatusRunning
}

func (m *mcpStopRecordingAgentManager) IsAgentReadyForPrompt(
	ctx context.Context,
	sessionID string,
) bool {
	return m.IsAgentRunningForSession(ctx, sessionID)
}

func (m *mcpStopRecordingAgentManager) runtimeStatus(sessionID string) (v1.AgentStatus, bool) {
	m.mu.Lock()
	var instance *simulatedInstance
	for _, candidate := range m.instances {
		if candidate.sessionID == sessionID {
			instance = candidate
			break
		}
	}
	m.mu.Unlock()
	if instance == nil {
		return "", false
	}
	instance.statusMu.Lock()
	defer instance.statusMu.Unlock()
	return instance.status, true
}

func (m *mcpStopRecordingAgentManager) releaseStop() {
	m.releaseOnce.Do(func() { close(m.allowStop) })
}

type mcpStopDispatchOutcome struct {
	response *ws.Message
	err      error
	elapsed  time.Duration
}

func TestMCPStopTask_DirectParentStopsLongRunningChild(t *testing.T) {
	ts, parentTaskID, _, _, _ := setupMCPTestServer(t)
	defer ts.Close()

	manager := newMCPStopRecordingAgentManager(ts.EventBus, ts.Logger)
	manager.SetLaunchDelay(10 * time.Millisecond)
	manager.SetExecutionTime(30 * time.Second)
	manager.SetACPMessageFn(func(taskID, _ string) []protocol.Message {
		return []protocol.Message{{
			Type: protocol.MessageTypeProgress, TaskID: taskID, Timestamp: time.Now(),
			Data: map[string]interface{}{
				"type":         "tool_call",
				"tool_call_id": "long-running-work",
				"tool_name":    "integration_wait",
				"tool_title":   "Long-running integration work",
				"tool_status":  "running",
			},
		}}
	})

	cfg := orchestrator.DefaultServiceConfig()
	cfg.Scheduler.ProcessInterval = 50 * time.Millisecond
	taskRepoAdapter := &taskRepositoryAdapter{repo: ts.TaskRepo, svc: ts.TaskSvc}
	orchestratorSvc := orchestrator.NewService(
		cfg, ts.EventBus, manager, taskRepoAdapter, ts.TaskRepo, nil, nil, nil, ts.Logger,
	)
	orchestratorSvc.SetMessageCreator(&testMessageCreatorAdapter{svc: ts.TaskSvc})
	orchestratorSvc.SetTurnService(&testTurnServiceAdapter{svc: ts.TaskSvc})
	orchestratorCtx, cancelOrchestrator := context.WithCancel(context.Background())
	require.NoError(t, orchestratorSvc.Start(orchestratorCtx))
	defer func() {
		if err := orchestratorSvc.Stop(); err != nil {
			t.Errorf("stop orchestrator: %v", err)
		}
		manager.Close()
		cancelOrchestrator()
	}()
	defer manager.releaseStop()

	// Register the real backend MCP handler on the shared dispatcher. Calls below
	// dispatch the internal action directly: local task-mode MCP transport is
	// intentionally outside this integration test and has separate adapter tests.
	mcpHandlers := mcphandlers.NewHandlers(
		ts.TaskSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		orchestratorSvc, orchestratorSvc.GetMessageQueue(), ts.Logger,
	)
	mcpHandlers.RegisterHandlers(ts.Gateway.Dispatcher)

	parent, err := ts.TaskRepo.GetTask(context.Background(), parentTaskID)
	require.NoError(t, err)
	child, err := ts.TaskSvc.CreateTask(context.Background(), &taskservice.CreateTaskRequest{
		WorkspaceID: parent.WorkspaceID, WorkflowID: parent.WorkflowID, WorkflowStepID: parent.WorkflowStepID,
		Title: "Long-running child", Description: "Keep working until the parent stops this task.",
		Priority: "medium", ParentID: parentTaskID,
	})
	require.NoError(t, err)

	running := make(chan struct{})
	var runningOnce sync.Once
	subscription, err := ts.EventBus.Subscribe(events.TaskSessionStateChanged, func(_ context.Context, event *bus.Event) error {
		data, ok := event.Data.(map[string]interface{})
		if ok && data["task_id"] == child.ID && data["new_state"] == string(models.TaskSessionStateRunning) {
			runningOnce.Do(func() { close(running) })
		}
		return nil
	})
	require.NoError(t, err)
	defer func() {
		if err := subscription.Unsubscribe(); err != nil {
			t.Errorf("unsubscribe session-state observer: %v", err)
		}
	}()

	launch, err := orchestratorSvc.LaunchSession(context.Background(), &orchestrator.LaunchSessionRequest{
		TaskID: child.ID, Intent: orchestrator.IntentStart, AgentProfileID: "integration-agent",
		Prompt: "Keep working until explicitly stopped.",
	})
	require.NoError(t, err)
	require.True(t, launch.Success)
	require.NotEmpty(t, launch.SessionID)
	mcpStopAwaitSignal(t, running, 2*time.Second, "child RUNNING state")

	sessionBefore, err := ts.TaskRepo.GetTaskSession(context.Background(), launch.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateRunning, sessionBefore.State)
	taskBefore, err := ts.TaskRepo.GetTask(context.Background(), child.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateInProgress, taskBefore.State)
	require.True(t, manager.IsAgentRunningForSession(context.Background(), launch.SessionID))
	promptsBefore := manager.promptCalls.Load()
	messagesBefore, err := ts.TaskRepo.ListMessages(context.Background(), launch.SessionID)
	require.NoError(t, err)

	stopRequest, err := ws.NewRequest("mcp-stop-1", ws.ActionMCPStopTask, map[string]interface{}{
		"task_id": child.ID, "sender_task_id": parentTaskID,
	})
	require.NoError(t, err)
	dispatchDone := make(chan mcpStopDispatchOutcome, 1)
	dispatchStarted := time.Now()
	go func() {
		response, dispatchErr := ts.Gateway.Dispatcher.Dispatch(context.Background(), stopRequest)
		dispatchDone <- mcpStopDispatchOutcome{
			response: response, err: dispatchErr, elapsed: time.Since(dispatchStarted),
		}
	}()

	mcpStopAwaitSignal(t, manager.stopEntered, mcpStopResponseDeadline, "asynchronous runtime teardown")
	first := mcpStopAwaitDispatch(t, dispatchDone, mcpStopResponseDeadline)
	require.NoError(t, first.err)
	require.Less(t, first.elapsed, mcpStopResponseDeadline)
	require.Equal(t, ws.MessageTypeResponse, first.response.Type)
	firstPayload := mcpStopParseResponse(t, first.response)
	require.Equal(t, child.ID, firstPayload.TaskID)
	require.Equal(t, orchestrator.CoordinatorTaskStopStatusStopped, firstPayload.Status)

	// The response lands while simulated teardown is deliberately blocked,
	// proving it confirms logical cancellation rather than process exit.
	require.True(t, manager.IsAgentRunningForSession(context.Background(), launch.SessionID))
	stoppedSession, err := ts.TaskRepo.GetTaskSession(context.Background(), launch.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateCancelled, stoppedSession.State)
	stoppedTask, err := ts.TaskRepo.GetTask(context.Background(), child.ID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, stoppedTask.State)
	require.Equal(t, promptsBefore, manager.promptCalls.Load(), "stop must not dispatch a replacement prompt")
	require.Zero(t, orchestratorSvc.GetMessageQueue().GetStatus(context.Background(), launch.SessionID).Count)

	manager.releaseStop()
	mcpStopAwaitSignal(t, manager.stopFinished, 2*time.Second, "simulated runtime stop")
	status, exists := manager.runtimeStatus(launch.SessionID)
	require.True(t, exists)
	require.Equal(t, v1.AgentStatusStopped, status)
	require.False(t, manager.IsAgentRunningForSession(context.Background(), launch.SessionID))

	messagesAfterStop, err := ts.TaskRepo.ListMessages(context.Background(), launch.SessionID)
	require.NoError(t, err)
	require.Len(t, messagesAfterStop, len(messagesBefore))
	launchesAfterStop := manager.GetLaunchCount()
	promptsAfterStop := manager.promptCalls.Load()
	stopCallsAfterStop := manager.stopCalls.Load()

	repeatRequest, err := ws.NewRequest("mcp-stop-2", ws.ActionMCPStopTask, map[string]interface{}{
		"task_id": child.ID, "sender_task_id": parentTaskID,
	})
	require.NoError(t, err)
	repeatResponse, err := ts.Gateway.Dispatcher.Dispatch(context.Background(), repeatRequest)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, repeatResponse.Type)
	repeatPayload := mcpStopParseResponse(t, repeatResponse)
	require.Equal(t, child.ID, repeatPayload.TaskID)
	require.Equal(t, orchestrator.CoordinatorTaskStopStatusNotRunning, repeatPayload.Status)

	repeatedSession, err := ts.TaskRepo.GetTaskSession(context.Background(), launch.SessionID)
	require.NoError(t, err)
	repeatedTask, err := ts.TaskRepo.GetTask(context.Background(), child.ID)
	require.NoError(t, err)
	require.Equal(t, stoppedSession.UpdatedAt, repeatedSession.UpdatedAt)
	require.Equal(t, stoppedTask.UpdatedAt, repeatedTask.UpdatedAt)
	require.Equal(t, launchesAfterStop, manager.GetLaunchCount())
	require.Equal(t, promptsAfterStop, manager.promptCalls.Load())
	require.Equal(t, stopCallsAfterStop, manager.stopCalls.Load())
	repeatedMessages, err := ts.TaskRepo.ListMessages(context.Background(), launch.SessionID)
	require.NoError(t, err)
	require.Len(t, repeatedMessages, len(messagesAfterStop))
}

type mcpStopResponsePayload struct {
	TaskID string                                 `json:"task_id"`
	Status orchestrator.CoordinatorTaskStopStatus `json:"status"`
}

func mcpStopParseResponse(t *testing.T, response *ws.Message) mcpStopResponsePayload {
	t.Helper()
	var payload mcpStopResponsePayload
	require.NoError(t, response.ParsePayload(&payload))
	return payload
}

func mcpStopAwaitSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func mcpStopAwaitDispatch(
	t *testing.T,
	done <-chan mcpStopDispatchOutcome,
	timeout time.Duration,
) mcpStopDispatchOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(timeout):
		t.Fatal("coordinator stop waited for simulated runtime teardown")
		return mcpStopDispatchOutcome{}
	}
}
