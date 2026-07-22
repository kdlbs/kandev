package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	eventtypes "github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func registerAsyncWorkForExecution(
	t *testing.T,
	svc *Service,
	taskID, sessionID, executionID, toolCallID, workID string,
) {
	t.Helper()
	payload := streams.NewSubagentTask("background work", "do it", "general-purpose")
	payload.SubagentTask().IsAsync = true
	payload.SubagentTask().AgentID = workID
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: toolCallID,
			ToolStatus: "in_progress",
			Normalized: payload,
		},
	})
}

func TestBackgroundCompletion_IdentifiedRetiresExactWorkAndDuplicateIsHarmless(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID, executionID = "task-accounting", "session-accounting", "execution-accounting"

	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-one", "work-one")
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-two", "work-two")
	svc.markForegroundIdle(sessionID)

	completion := &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       streams.EventTypeBackgroundComplete,
			ToolCallID: "work-two",
		},
	}
	svc.handleAgentStreamEvent(t.Context(), completion)
	if !svc.hasBackgroundTask(sessionID, "tool-one") || svc.hasBackgroundTask(sessionID, "tool-two") {
		t.Fatalf("identified completion did not retire exact work: one=%t two=%t",
			svc.hasBackgroundTask(sessionID, "tool-one"), svc.hasBackgroundTask(sessionID, "tool-two"))
	}

	// Re-delivery of the same provider completion must not consume another job.
	svc.handleAgentStreamEvent(t.Context(), completion)
	if !svc.hasBackgroundTask(sessionID, "tool-one") {
		t.Fatal("duplicate identified completion retired unrelated outstanding work")
	}
}

func TestBackgroundCompletion_IdentifiedRemainsExecutionScoped(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-exact-scope", "session-exact-scope"
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, "execution-old", "tool-old", "work-old")
	svc.markForegroundIdle(sessionID)

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "execution-successor",
		Data: &lifecycle.AgentStreamEventData{
			Type: streams.EventTypeBackgroundComplete, ToolCallID: "work-old",
		},
	})
	if !svc.hasBackgroundTask(sessionID, "tool-old") {
		t.Fatal("identified completion attributed to successor cleared predecessor work")
	}
}

func TestBackgroundCompletion_UnidentifiedFailsClosedWithMultipleWorkloads(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID, executionID = "task-fallback", "session-fallback", "execution-fallback"

	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-one", "work-one")
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-two", "work-two")
	svc.markForegroundIdle(sessionID)
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})

	if !svc.hasBackgroundTask(sessionID, "tool-one") || !svc.hasBackgroundTask(sessionID, "tool-two") {
		t.Fatal("unidentified completion must not guess among multiple outstanding workloads")
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("ambiguous completion changed visible activity to %q", got)
	}
}

func TestBackgroundCompletion_UnidentifiedSuccessorCycleRetiresSoleSessionWork(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-successor-complete", "session-successor-complete"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	registerAsyncWorkForExecution(
		t, svc, taskID, sessionID, "execution-launch", "tool-launch", "work-launch",
	)
	svc.markForegroundIdle(sessionID)

	completion := &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "execution-successor",
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	}
	svc.handleAgentStreamEvent(t.Context(), completion)
	if svc.hasBackgroundTask(sessionID, "tool-launch") {
		t.Fatal("ID-less successor-cycle notification did not retire sole session workload")
	}

	activityClears := 0
	for _, record := range recorded.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		data, _ := record.event.Data.(map[string]interface{})
		if value, present := data["foreground_activity"]; present && value == nil {
			activityClears++
		}
	}
	if activityClears != 1 || len(taskEvents.activityTaskIDs) != 1 {
		t.Fatalf("sole completion clear cardinality: session=%d task=%v", activityClears, taskEvents.activityTaskIDs)
	}

	// The same ID-less notification re-delivered after retirement is a no-op.
	svc.handleAgentStreamEvent(t.Context(), completion)
	if len(taskEvents.activityTaskIDs) != 1 {
		t.Fatalf("duplicate completion republished task activity: %v", taskEvents.activityTaskIDs)
	}
}

func TestBackgroundCompletion_UnidentifiedFailsClosedAcrossExecutions(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-cross-exec", "session-cross-exec"
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, "execution-old", "tool-old", "work-old")
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, "execution-new", "tool-new", "work-new")
	svc.markForegroundIdle(sessionID)

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "execution-current",
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})
	if !svc.hasBackgroundTask(sessionID, "tool-old") || !svc.hasBackgroundTask(sessionID, "tool-new") {
		t.Fatal("ambiguous cross-execution completion guessed an owning workload")
	}
}

func TestDelayedOldExecutionToolCompletionPreservesSuccessorRegistration(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID, toolCallID = "session-rotated-tool", "provider-reused-tool-id"

	// A successor execution can reuse a provider-local tool-call ID. Its newer
	// registration replaces ownership of that key.
	svc.registerBackgroundWork(sessionID, toolCallID, "execution-old", "work-old")
	svc.registerBackgroundWork(sessionID, toolCallID, "execution-new", "work-new")
	svc.markForegroundIdle(sessionID)

	if svc.completeBackgroundTaskForExecution(sessionID, toolCallID, "execution-old") {
		t.Fatal("delayed old completion changed successor-visible activity")
	}
	if !svc.hasBackgroundTask(sessionID, toolCallID) {
		t.Fatal("delayed old completion removed successor registration")
	}
}

func TestExecutionStop_RetiresOnlyOwnedBackgroundWorkAndPublishesFinalTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-stop-accounting", "session-stop-accounting", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	events := &recordingEventBus{}
	svc.eventBus = events

	registerAsyncWorkForExecution(t, svc, "task-stop-accounting", "session-stop-accounting", "execution-old", "tool-old", "work-old")
	registerAsyncWorkForExecution(t, svc, "task-stop-accounting", "session-stop-accounting", "execution-new", "tool-new", "work-new")
	svc.markForegroundIdle("session-stop-accounting")

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: "task-stop-accounting", SessionID: "session-stop-accounting", AgentExecutionID: "execution-old",
	})
	if svc.hasBackgroundTask("session-stop-accounting", "tool-old") {
		t.Fatal("stopped execution left its background registration behind")
	}
	if !svc.hasBackgroundTask("session-stop-accounting", "tool-new") {
		t.Fatal("old execution cleanup removed successor execution background work")
	}
	if got := svc.ForegroundActivity("session-stop-accounting"); got != v1.ForegroundActivityBackground {
		t.Fatalf("successor background work should remain visible, got %q", got)
	}

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: "task-stop-accounting", SessionID: "session-stop-accounting", AgentExecutionID: "execution-new",
	})
	if svc.hasBackgroundTask("session-stop-accounting", "tool-new") {
		t.Fatal("final stopped execution left its background registration behind")
	}
	if got := svc.ForegroundActivity("session-stop-accounting"); got != v1.ForegroundActivityGenerating {
		t.Fatalf("final execution cleanup left stale background activity: %q", got)
	}

	activityEvents := 0
	for _, record := range events.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		activityEvents++
		data, ok := record.event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("terminal cleanup activity payload = %#v", record.event.Data)
		}
		value, present := data["foreground_activity"]
		if !present || value != nil {
			t.Fatalf("terminal cleanup must explicitly clear foreground_activity, got %#v", data)
		}
	}
	if activityEvents != 1 {
		t.Fatalf("terminal cleanup activity events = %d, want exactly one", activityEvents)
	}
}

func TestExecutionCleanup_DelayedPublicationCannotOverwriteSuccessorActivity(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-cleanup-race", "session-cleanup-race"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	svc.registerBackgroundWork(sessionID, "tool-old", "execution-old", "work-old")
	svc.markForegroundIdle(sessionID)

	cleanupMutated := make(chan struct{})
	releaseCleanupPublish := make(chan struct{})
	cleanupDone := make(chan struct{})
	go func() {
		publication, changed := svc.clearExecutionBackgroundWorkSnapshot(sessionID, "execution-old")
		if !changed {
			close(cleanupMutated)
			close(cleanupDone)
			return
		}
		close(cleanupMutated)
		<-releaseCleanupPublish
		svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)
		close(cleanupDone)
	}()
	<-cleanupMutated

	svc.registerBackgroundWork(sessionID, "tool-new", "execution-new", "work-new")
	svc.markForegroundIdle(sessionID)
	svc.publishForegroundActivityChanged(t.Context(), taskID, sessionID)
	close(releaseCleanupPublish)
	<-cleanupDone

	var values []interface{}
	for _, record := range recorded.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		data, _ := record.event.Data.(map[string]interface{})
		values = append(values, data["foreground_activity"])
	}
	if len(values) != 1 || values[0] != string(v1.ForegroundActivityBackground) {
		t.Fatalf("activity publications = %#v, want only successor background", values)
	}
	if len(taskEvents.activityTaskIDs) != 1 {
		t.Fatalf("task aggregate publications = %v, want successor only", taskEvents.activityTaskIDs)
	}
}

func TestExecutionCleanup_DelayedNullCannotOverwriteClaimlessSuccessorStart(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-cleanup-claimless-race", "session-cleanup-claimless-race"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWork(sessionID, "tool-old", "execution-old", "work-old")
	svc.markForegroundIdle(sessionID)

	cleanupMutated := make(chan struct{})
	releaseCleanupPublish := make(chan struct{})
	cleanupDone := make(chan struct{})
	go func() {
		publication, changed := svc.clearExecutionBackgroundWorkSnapshot(sessionID, "execution-old")
		close(cleanupMutated)
		if changed {
			<-releaseCleanupPublish
			svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)
		}
		close(cleanupDone)
	}()
	<-cleanupMutated

	dispatch := svc.beginForegroundDispatch(sessionID, nil)
	if dispatch == nil {
		t.Fatal("claimless successor must establish prompt-cycle ownership")
	}
	svc.publishForegroundActivityChanged(t.Context(), taskID, sessionID)
	close(releaseCleanupPublish)
	<-cleanupDone

	var values []interface{}
	for _, record := range recorded.events {
		if record.subject == eventtypes.TaskSessionActivityChanged {
			data, _ := record.event.Data.(map[string]interface{})
			values = append(values, data["foreground_activity"])
		}
	}
	if len(values) != 1 || values[0] != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("activity publications = %#v, want only successor generating", values)
	}
}

func TestBackgroundCompletion_IDLessSingletonDelayedNullCannotOverwriteSuccessor(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-idless-race", "session-idless-race", "execution-idless-race"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWork(sessionID, "tool-old", executionID, "work-old")
	svc.markForegroundIdle(sessionID)

	completionMutated := make(chan struct{})
	releaseCompletionPublish := make(chan struct{})
	completionDone := make(chan struct{})
	go func() {
		publication, changed := svc.completeBackgroundWorkSnapshot(sessionID, executionID, "", nil)
		close(completionMutated)
		if changed {
			<-releaseCompletionPublish
			svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)
		}
		close(completionDone)
	}()
	<-completionMutated

	dispatch := svc.beginForegroundDispatch(sessionID, nil)
	if dispatch == nil {
		t.Fatal("claimless successor must establish prompt-cycle ownership")
	}
	svc.publishForegroundActivityChanged(t.Context(), taskID, sessionID)
	close(releaseCompletionPublish)
	<-completionDone

	var values []interface{}
	for _, record := range recorded.events {
		if record.subject == eventtypes.TaskSessionActivityChanged {
			data, _ := record.event.Data.(map[string]interface{})
			values = append(values, data["foreground_activity"])
		}
	}
	if len(values) != 1 || values[0] != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("activity publications = %#v, want only successor generating", values)
	}
}

func TestExecutionCleanup_AfterClaimlessBeginCannotCreateSuccessorNull(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-cleanup-after-begin", "session-cleanup-after-begin"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWork(sessionID, "tool-old", "execution-old", "work-old")
	svc.markForegroundIdle(sessionID)

	dispatch := svc.beginForegroundDispatch(sessionID, nil)
	if dispatch == nil {
		t.Fatal("claimless successor must establish prompt-cycle ownership")
	}
	cleanupMutated := make(chan struct{})
	releaseCleanupPublish := make(chan struct{})
	cleanupDone := make(chan struct{})
	go func() {
		publication, changed := svc.clearExecutionBackgroundWorkSnapshot(sessionID, "execution-old")
		close(cleanupMutated)
		<-releaseCleanupPublish
		if changed {
			svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)
		}
		close(cleanupDone)
	}()
	<-cleanupMutated

	svc.publishForegroundActivityChanged(t.Context(), taskID, sessionID)
	close(releaseCleanupPublish)
	<-cleanupDone

	var values []interface{}
	for _, record := range recorded.events {
		if record.subject == eventtypes.TaskSessionActivityChanged {
			data, _ := record.event.Data.(map[string]interface{})
			values = append(values, data["foreground_activity"])
		}
	}
	if len(values) != 1 || values[0] != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("activity publications = %#v, want only successor generating", values)
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("cleanup after begin displaced successor ownership: got %q", got)
	}
}

func TestBackgroundCompletion_AfterClaimlessBeginCannotCreateSuccessorNull(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-completion-after-begin", "session-completion-after-begin", "execution-old"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWork(sessionID, "tool-old", executionID, "work-old")
	svc.markForegroundIdle(sessionID)

	dispatch := svc.beginForegroundDispatch(sessionID, nil)
	if dispatch == nil {
		t.Fatal("claimless successor must establish prompt-cycle ownership")
	}
	completionMutated := make(chan struct{})
	releaseCompletionPublish := make(chan struct{})
	completionDone := make(chan struct{})
	go func() {
		publication, changed := svc.completeBackgroundWorkSnapshot(sessionID, executionID, "", nil)
		close(completionMutated)
		<-releaseCompletionPublish
		if changed {
			svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)
		}
		close(completionDone)
	}()
	<-completionMutated

	svc.publishForegroundActivityChanged(t.Context(), taskID, sessionID)
	close(releaseCompletionPublish)
	<-completionDone

	var values []interface{}
	for _, record := range recorded.events {
		if record.subject == eventtypes.TaskSessionActivityChanged {
			data, _ := record.event.Data.(map[string]interface{})
			values = append(values, data["foreground_activity"])
		}
	}
	if len(values) != 1 || values[0] != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("activity publications = %#v, want only successor generating", values)
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("completion after begin displaced successor ownership: got %q", got)
	}
}

func TestTerminalSessionStateChangeExplicitlyClearsBackgroundActivity(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-terminal-clear", "session-terminal-clear"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	svc.registerBackgroundWork(sessionID, "tool-background", "execution-terminal", "work-terminal")
	svc.markForegroundIdle(sessionID)

	svc.updateTaskSessionState(
		t.Context(), taskID, sessionID, models.TaskSessionStateCancelled, "operator stopped", false,
	)

	for _, record := range recorded.events {
		if record.subject != eventtypes.TaskSessionStateChanged {
			continue
		}
		data, ok := record.event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("terminal state payload = %#v", record.event.Data)
		}
		value, present := data["foreground_activity"]
		if !present || value != nil {
			t.Fatalf("terminal state change must explicitly clear activity, got %#v", data)
		}
		return
	}
	t.Fatal("terminal state change event was not published")
}

func TestStopSessionPathPublishesStateAndTeardownActivityClears(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-stop-path", "session-stop-path", "execution-stop-path"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), agentManager)
	svc.executor.SetOnSessionStateChange(func(
		ctx context.Context,
		taskID, sessionID string,
		state models.TaskSessionState,
		errorMessage string,
	) error {
		svc.updateTaskSessionState(ctx, taskID, sessionID, state, errorMessage, true)
		return nil
	})
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	svc.registerBackgroundWork(sessionID, "tool-stop-path", executionID, "work-stop-path")
	svc.markForegroundIdle(sessionID)

	if err := svc.StopSession(ctx, sessionID, "operator stopped", true); err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	waitForStopCall(t, agentManager)
	// The lifecycle manager publishes this after StopAgentWithReason completes.
	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
	})

	var stateClears, activityClears int
	for _, record := range recorded.events {
		data, ok := record.event.Data.(map[string]interface{})
		if !ok {
			continue
		}
		value, present := data["foreground_activity"]
		if !present || value != nil {
			continue
		}
		switch record.subject {
		case eventtypes.TaskSessionStateChanged:
			stateClears++
		case eventtypes.TaskSessionActivityChanged:
			activityClears++
		}
	}
	if stateClears != 1 || activityClears != 1 {
		t.Fatalf("session.stop clear cardinality: state=%d activity=%d, want 1/1", stateClears, activityClears)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
		t.Fatalf("task aggregate cleanup publications = %v, want [%s]", taskEvents.activityTaskIDs, taskID)
	}
	if svc.hasBackgroundTask(sessionID, "tool-stop-path") {
		t.Fatal("session.stop teardown retained owned background work")
	}
}

func TestExecutionTerminalEvents_ReconcileMissingBackgroundCompletion(t *testing.T) {
	tests := []struct {
		name   string
		handle func(*Service, *mockAgentManager, watcher.AgentEventData)
	}{
		{
			name: "completed",
			handle: func(svc *Service, _ *mockAgentManager, data watcher.AgentEventData) {
				svc.handleAgentCompleted(t.Context(), data)
			},
		},
		{
			name: "failed",
			handle: func(svc *Service, _ *mockAgentManager, data watcher.AgentEventData) {
				svc.handleAgentFailed(t.Context(), data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			const taskID, sessionID, executionID = "task-terminal", "session-terminal", "execution-terminal"
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
			agentManager := &mockAgentManager{repoForExecutionLookup: repo}
			svc := createTestServiceWithScheduler(
				repo, newMockStepGetter(), newMockTaskRepo(), agentManager,
			)
			svc.messageCreator = &mockMessageCreator{}
			recorded := &recordingEventBus{}
			svc.eventBus = recorded
			taskEvents := &recordingTaskEvents{}
			svc.SetTaskEventPublisher(taskEvents)
			registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-terminal", "work-terminal")
			svc.markForegroundIdle(sessionID)

			tt.handle(svc, agentManager, watcher.AgentEventData{
				TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
				ErrorMessage: "terminal failure",
			})

			if svc.hasBackgroundTask(sessionID, "tool-terminal") {
				t.Fatal("terminal execution event left missing-completion registration behind")
			}
			if got := countActivityClears(recorded); got != 1 {
				t.Fatalf("terminal session activity clears = %d, want exactly one", got)
			}
			if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
				t.Fatalf("terminal task recomputes = %v, want [%s]", taskEvents.activityTaskIDs, taskID)
			}
			waitForStopCall(t, agentManager)
		})
	}
}

func countActivityClears(recorded *recordingEventBus) int {
	clears := 0
	for _, record := range recorded.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		data, _ := record.event.Data.(map[string]interface{})
		if value, present := data["foreground_activity"]; present && value == nil {
			clears++
		}
	}
	return clears
}

func TestTransientFailurePreservesBackgroundRegistration(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	svc.registerBackgroundWork("s1", "tool-transient", "exec-transient", "work-transient")
	svc.markForegroundIdle("s1")

	svc.handleAgentFailed(t.Context(), watcher.AgentEventData{
		TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-transient", ErrorMessage: overloaded529,
	})

	if !svc.hasBackgroundTask("s1", "tool-transient") {
		t.Fatal("transient failure cleaned background registration before retry teardown")
	}
	if got := countActivityClears(recorded); got != 0 || len(taskEvents.activityTaskIDs) != 0 {
		t.Fatalf("transient cleanup publications: session=%d task=%v", got, taskEvents.activityTaskIDs)
	}
}

func TestCleanupAgentExecution_ForcedPathIsOwnedAndIdempotent(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-forced-cleanup", "session-forced-cleanup"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	agentManager := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(
		repo, newMockStepGetter(), newMockTaskRepo(), agentManager,
	)
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	svc.registerBackgroundWork(sessionID, "tool-old", "execution-old", "work-old")
	svc.registerBackgroundWork(sessionID, "tool-new", "execution-new", "work-new")
	svc.markForegroundIdle(sessionID)

	svc.cleanupAgentExecution("execution-old", taskID, sessionID)
	if svc.hasBackgroundTask(sessionID, "tool-old") {
		t.Fatal("forced cleanup retained owned predecessor work")
	}
	if !svc.hasBackgroundTask(sessionID, "tool-new") {
		t.Fatal("forced predecessor cleanup removed successor work")
	}
	if got := countActivityClears(recorded); got != 0 {
		t.Fatalf("predecessor cleanup cleared live successor activity %d times", got)
	}

	svc.cleanupAgentExecution("execution-new", taskID, sessionID)
	svc.cleanupAgentExecution("execution-new", taskID, sessionID)
	if svc.hasBackgroundTask(sessionID, "tool-new") {
		t.Fatal("forced cleanup retained final owned work")
	}
	if got := countActivityClears(recorded); got != 1 {
		t.Fatalf("forced final cleanup clears = %d, want exactly one", got)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
		t.Fatalf("forced cleanup task recomputes = %v, want [%s]", taskEvents.activityTaskIDs, taskID)
	}
}
