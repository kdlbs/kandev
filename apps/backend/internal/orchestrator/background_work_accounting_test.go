package orchestrator

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	eventtypes "github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type blockingRetirementEventBus struct {
	*recordingEventBus
	mu                sync.Mutex
	retirementEntered chan struct{}
	releaseRetirement chan struct{}
	once              sync.Once
}

func (b *blockingRetirementEventBus) Publish(
	ctx context.Context,
	subject string,
	event *bus.Event,
) error {
	if subject == eventtypes.TaskSessionActivityChanged {
		data, _ := event.Data.(map[string]interface{})
		if count, present := data["active_subagent_count"]; present && count == 0 {
			b.once.Do(func() { close(b.retirementEntered) })
			<-b.releaseRetirement
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.recordingEventBus.Publish(ctx, subject, event)
}

func registerAsyncWorkForExecution(
	t *testing.T,
	svc *Service,
	taskID, sessionID, executionID, toolCallID, workID string,
) {
	t.Helper()
	payload := attestedSubagentPayload("background work", "do it", "general-purpose")
	payload.SubagentTask().IsAsync = true
	payload.SubagentTask().AgentID = workID
	payload.SetBackgroundWorkIdentity(streams.BackgroundWorkKindSubagent, workID, true, false)
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

func turnActivityRecord(t *testing.T, svc *Service, sessionID string) (*turnActivity, bool) {
	t.Helper()
	svc.foregroundActivityMu.Lock()
	defer svc.foregroundActivityMu.Unlock()
	activity, ok := svc.foregroundActivity[sessionID]
	return activity, ok
}

func TestActiveSubagentCount_DerivesOnlyLiveSubagentRegistrations(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const sessionID = "session-subagent-count"

	svc.registerBackgroundWorkKind(
		sessionID, "subagent-one", "execution", "child-one", streams.BackgroundWorkKindSubagent,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "subagent-two", "execution", "child-two", streams.BackgroundWorkKindSubagent,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "shell", "execution", "shell-one", streams.BackgroundWorkKindShell,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "monitor", "execution", "monitor-one", streams.BackgroundWorkKindMonitor,
	)
	if got := svc.ActiveSubagentCount(sessionID); got != 2 {
		t.Fatalf("active subagent count = %d, want 2", got)
	}

	svc.completeBackgroundTaskForExecution(sessionID, "subagent-one", "execution")
	svc.completeBackgroundTaskForExecution(sessionID, "subagent-one", "execution")
	if got := svc.ActiveSubagentCount(sessionID); got != 1 {
		t.Fatalf("count after duplicate terminal completion = %d, want 1", got)
	}
}

func TestBackgroundCompletion_IntermediateSubagentPublishesGenerating(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-intermediate", "session-intermediate", "execution-intermediate"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWorkKind(
		sessionID, "tool-one", executionID, "child-one", streams.BackgroundWorkKindSubagent,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "tool-two", executionID, "child-two", streams.BackgroundWorkKindSubagent,
	)
	svc.markForegroundIdle(sessionID)

	publication, changed := svc.completeBackgroundWorkSnapshot(
		sessionID, executionID, "child-one", string(v1.ForegroundActivityGenerating),
	)
	if !changed {
		t.Fatal("intermediate subagent completion must publish the count-only transition")
	}
	svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)

	if len(recorded.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(recorded.events))
	}
	data, _ := recorded.events[0].event.Data.(map[string]interface{})
	if got := data["foreground_activity"]; got != string(v1.ForegroundActivityGenerating) {
		t.Fatalf("published foreground activity = %#v, want generating", got)
	}
	if got := data["active_subagent_count"]; got != 1 {
		t.Fatalf("published active subagent count = %#v, want 1", got)
	}
}

func TestBackgroundCompletion_SettledSessionClearsActivityForCountOnlyUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-settled-intermediate", "session-settled-intermediate", "execution-settled-intermediate"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWorkKind(
		sessionID, "tool-one", executionID, "child-one", streams.BackgroundWorkKindSubagent,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "tool-two", executionID, "child-two", streams.BackgroundWorkKindSubagent,
	)
	svc.markForegroundIdle(sessionID)

	publication, changed := svc.completeBackgroundWorkSnapshot(
		sessionID, executionID, "child-one", string(v1.ForegroundActivityBackground),
	)
	if !changed {
		t.Fatal("intermediate settled completion must publish the count-only transition")
	}
	svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)

	if len(recorded.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(recorded.events))
	}
	data, _ := recorded.events[0].event.Data.(map[string]interface{})
	if got := data["foreground_activity"]; got != nil {
		t.Fatalf("settled session activity = %#v, want nil", got)
	}
	if got := data["active_subagent_count"]; got != 1 {
		t.Fatalf("settled session active subagent count = %#v, want 1", got)
	}
}

func TestBackgroundCompletion_EnabledClaudeSettledSessionPublishesBackground(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-settled-claude", "session-settled-claude", "execution-settled-claude"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	enableClaudeBackgroundPromptHandoffForTest(t, svc)
	setSessionAgentNameForTest(t, svc, sessionID, "claude-acp")
	advertisePromptQueueingForTest(t, svc, sessionID)
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	svc.registerBackgroundWorkKind(
		sessionID, "tool-one", executionID, "child-one", streams.BackgroundWorkKindSubagent,
	)
	svc.registerBackgroundWorkKind(
		sessionID, "tool-two", executionID, "child-two", streams.BackgroundWorkKindSubagent,
	)
	svc.markForegroundIdle(sessionID)

	publication, changed := svc.completeBackgroundWorkSnapshot(
		sessionID, executionID, "child-one", string(v1.ForegroundActivityBackground),
	)
	if !changed {
		t.Fatal("intermediate enabled Claude completion must publish the count-only transition")
	}
	svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)

	if len(recorded.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(recorded.events))
	}
	data, _ := recorded.events[0].event.Data.(map[string]interface{})
	if got := data["foreground_activity"]; got != string(v1.ForegroundActivityBackground) {
		t.Fatalf("enabled Claude settled activity = %#v, want background", got)
	}
	if got := data["active_subagent_count"]; got != 1 {
		t.Fatalf("enabled Claude settled active subagent count = %#v, want 1", got)
	}
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

func TestBackgroundCompletion_UnidentifiedRetiresOldestOfMultipleWorkloadsFIFO(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID, executionID = "task-fallback", "session-fallback", "execution-fallback"

	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-one", "work-one")
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-two", "work-two")
	svc.markForegroundIdle(sessionID)
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})

	// An ID-less completion retires exactly one registration — the oldest
	// (FIFO) — and leaves the remainder live, per the spec's "ambiguous
	// remainder live" contract. It must not retire zero registrations just
	// because more than one candidate exists.
	if svc.hasBackgroundTask(sessionID, "tool-one") {
		t.Fatal("unidentified completion must retire the oldest registered workload")
	}
	if !svc.hasBackgroundTask(sessionID, "tool-two") {
		t.Fatal("unidentified completion must leave the remaining workload live")
	}
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("completion with a live remainder changed visible activity to %q", got)
	}
}

// TestBackgroundCompletion_TwoUnidentifiedCompletionsRetireBothWorkloadsFIFO is
// the multi-workload regression: two background workloads are registered
// (distinct tool-call IDs, same execution), then two ID-less
// streams.EventTypeBackgroundComplete payloads are delivered through the real
// event-handler path (handleAgentStreamEvent), exactly as Claude's
// task-notification completion arrives in production. Before the fix, the
// second-candidate-is-ambiguous branch bailed out entirely on any completion
// once two or more registrations existed, so neither payload ever retired
// anything and the session over-reported background-running forever.
func TestBackgroundCompletion_TwoUnidentifiedCompletionsRetireBothWorkloadsFIFO(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-two-idless", "session-two-idless", "execution-two-idless"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	// Registration order establishes FIFO precedence: tool-first registers
	// before tool-second.
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-first", "work-first")
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, executionID, "tool-second", "work-second")
	svc.markForegroundIdle(sessionID)

	idlessCompletion := &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	}

	// First ID-less completion retires exactly the first-registered workload
	// (FIFO), leaving the second one live and the session still yielded.
	svc.handleAgentStreamEvent(t.Context(), idlessCompletion)
	if svc.hasBackgroundTask(sessionID, "tool-first") {
		t.Fatal("first ID-less completion did not retire the first-registered workload")
	}
	if !svc.hasBackgroundTask(sessionID, "tool-second") {
		t.Fatal("first ID-less completion retired the second-registered workload out of FIFO order")
	}
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("session left background-idle with a live remainder, got %q", got)
	}

	// Second ID-less completion retires the sole remaining registration and
	// the session leaves background (activity publication fires).
	svc.handleAgentStreamEvent(t.Context(), idlessCompletion)
	if svc.hasBackgroundTask(sessionID, "tool-second") {
		t.Fatal("second ID-less completion did not retire the last remaining workload")
	}
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("session did not leave background after final completion, got %q", got)
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

	activityUpdates := 0
	for _, record := range recorded.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		data, _ := record.event.Data.(map[string]interface{})
		if count, present := data["active_subagent_count"]; present && count == 0 {
			activityUpdates++
		}
	}
	if activityUpdates != 1 || len(taskEvents.activityTaskIDs) != 2 {
		t.Fatalf("sole completion update cardinality: session=%d task=%v", activityUpdates, taskEvents.activityTaskIDs)
	}

	// The same ID-less notification re-delivered after retirement is a no-op.
	svc.handleAgentStreamEvent(t.Context(), completion)
	if len(taskEvents.activityTaskIDs) != 2 {
		t.Fatalf("duplicate completion republished task activity: %v", taskEvents.activityTaskIDs)
	}
}

func TestBackgroundCompletion_UnidentifiedAcrossExecutionsRetiresOldestFIFO(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-cross-exec", "session-cross-exec"
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, "execution-old", "tool-old", "work-old")
	registerAsyncWorkForExecution(t, svc, taskID, sessionID, "execution-new", "tool-new", "work-new")
	svc.markForegroundIdle(sessionID)

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "execution-current",
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})
	// An uncorrelated completion is not execution-scoped (there is no ID to
	// scope by); it retires the oldest registration across the whole session,
	// regardless of which execution launched it.
	if svc.hasBackgroundTask(sessionID, "tool-old") {
		t.Fatal("cross-execution unidentified completion must retire the oldest registered workload")
	}
	if !svc.hasBackgroundTask(sessionID, "tool-new") {
		t.Fatal("cross-execution unidentified completion must leave the newer workload live")
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

func TestExecutionTeardown_StaleToolOwnershipIsNotVisibleToReusedSuccessorID(t *testing.T) {
	repo := setupTestRepo(t)
	const (
		taskID       = "task-tool-collision"
		sessionID    = "session-tool-collision"
		oldExecution = "execution-old"
		newExecution = "execution-new"
		toolCallID   = "provider-reused-tool-id"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return newExecution, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.messageCreator = &mockMessageCreator{}

	// The predecessor never emits a terminal update for this foreground tool.
	svc.recordToolOwnership(sessionID, toolCallID, oldExecution, toolOwnershipForeground)

	// The successor is already background-idle when delayed predecessor teardown
	// arrives. Its later update reuses the provider-local tool-call ID but carries
	// no positive ownership metadata.
	svc.registerBackgroundWork(sessionID, "successor-work", newExecution, "work-new")
	svc.markForegroundIdle(sessionID)
	svc.handleAgentStopped(t.Context(), watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: oldExecution,
	})
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: newExecution,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: toolCallID,
			ToolStatus: agentEventCompleted,
			ToolCallContents: []streams.ToolCallContentItem{{
				Type: "content",
				Content: &streams.ContentBlock{
					Type: "text",
					Text: "successor update",
				},
			}},
		},
	})

	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("stale predecessor tool ownership changed successor activity to %q", got)
	}
}

func TestToolOwnership_EmptyIDDoesNotLeakActivityLock(t *testing.T) {
	for _, testCase := range []struct {
		name string
		call func(*Service, string)
	}{
		{
			name: "lookup",
			call: func(svc *Service, sessionID string) {
				svc.toolOwnership(sessionID, "", "execution")
			},
		},
		{
			name: "clear",
			call: func(svc *Service, sessionID string) {
				svc.clearToolOwnership(sessionID, "", "execution")
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			const sessionID = "session-empty-tool-id"
			svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
			svc.recordToolOwnership(sessionID, "real-tool", "execution", toolOwnershipForeground)

			testCase.call(svc, sessionID)

			activity, ok := turnActivityRecord(t, svc, sessionID)
			if !ok {
				t.Fatal("tool ownership precondition did not create activity")
			}
			if !activity.mu.TryLock() {
				t.Fatal("empty tool ID returned while retaining the activity lock")
			}
			activity.mu.Unlock()
		})
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
	svc.recordToolOwnership(
		"session-stop-accounting",
		"foreground-old",
		"execution-old",
		toolOwnershipForeground,
	)
	svc.recordToolOwnership(
		"session-stop-accounting",
		"child-new",
		"execution-new",
		toolOwnershipChild,
	)
	svc.markForegroundIdle("session-stop-accounting")

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: "task-stop-accounting", SessionID: "session-stop-accounting", AgentExecutionID: "execution-old",
	})
	if _, ok := turnActivityRecord(t, svc, "session-stop-accounting"); !ok {
		t.Fatal("predecessor cleanup retired activity already owned by the successor execution")
	}
	if svc.hasBackgroundTask("session-stop-accounting", "tool-old") {
		t.Fatal("stopped execution left its background registration behind")
	}
	if !svc.hasBackgroundTask("session-stop-accounting", "tool-new") {
		t.Fatal("old execution cleanup removed successor execution background work")
	}
	if got := svc.toolOwnership(
		"session-stop-accounting",
		"foreground-old",
		"execution-old",
	); got != toolOwnershipUnknown {
		t.Fatalf("predecessor tool ownership survived teardown: %d", got)
	}
	if got := svc.toolOwnership(
		"session-stop-accounting",
		"child-new",
		"execution-new",
	); got != toolOwnershipChild {
		t.Fatalf("successor tool ownership = %d, want child", got)
	}
	if got := svc.foregroundActivityValue("session-stop-accounting"); got != v1.ForegroundActivityBackground {
		t.Fatalf("successor background work should remain visible, got %q", got)
	}

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: "task-stop-accounting", SessionID: "session-stop-accounting", AgentExecutionID: "execution-new",
	})
	if svc.hasBackgroundTask("session-stop-accounting", "tool-new") {
		t.Fatal("final stopped execution left its background registration behind")
	}
	if got := svc.foregroundActivityValue("session-stop-accounting"); got != v1.ForegroundActivityGenerating {
		t.Fatalf("final execution cleanup left stale background activity: %q", got)
	}
	if _, ok := turnActivityRecord(t, svc, "session-stop-accounting"); ok {
		t.Fatal("final stopped execution retained the session activity record")
	}

	var activityValues []interface{}
	var subagentCounts []int
	for _, record := range events.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		data, ok := record.event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("terminal cleanup activity payload = %#v", record.event.Data)
		}
		activityValues = append(activityValues, data["foreground_activity"])
		subagentCounts = append(subagentCounts, data["active_subagent_count"].(int))
	}
	wantValues := []interface{}{"generating", "generating", "generating", nil}
	if !slices.Equal(activityValues, wantValues) {
		t.Fatalf("execution cleanup activity values = %#v, want %#v", activityValues, wantValues)
	}
	if !slices.Equal(subagentCounts, []int{1, 2, 1, 0}) {
		t.Fatalf("execution cleanup subagent counts = %v, want [1 2 1 0]", subagentCounts)
	}
}

func TestExecutionStop_DropsLateFramesAfterActivityRetirement(t *testing.T) {
	const (
		taskID      = "task-stopped-late-frame"
		sessionID   = "session-stopped-late-frame"
		executionID = "execution-stopped-late-frame"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.registerBackgroundWork(sessionID, "background-old", executionID, "work-old")

	svc.handleAgentStopped(t.Context(), watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
	})
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "late-tool",
			ToolStatus: "in_progress",
		},
	})

	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("late stopped-execution frame recreated retired activity")
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
	retiredActivity := make(chan *turnActivity, 1)
	go func() {
		publication, changed := svc.retireExecutionActivitySnapshot(sessionID, "execution-old")
		retiredActivity <- publication.activity
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
	current, ok := turnActivityRecord(t, svc, sessionID)
	if !ok {
		t.Fatal("successor registration did not create an activity record")
	}
	if current == <-retiredActivity {
		t.Fatal("successor mutated the predecessor activity record after it was retired")
	}
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
	if len(values) != 1 || values[0] != nil {
		t.Fatalf("activity publications = %#v, want only successor clear", values)
	}
	if len(taskEvents.activityTaskIDs) != 1 {
		t.Fatalf("task aggregate publications = %v, want successor only", taskEvents.activityTaskIDs)
	}
}

func TestExecutionCleanup_PublicationDeliveryIsOrderedAcrossRecordGenerations(t *testing.T) {
	repo := setupTestRepo(t)
	const taskID, sessionID = "task-delivery-race", "session-delivery-race"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	recorded := &recordingEventBus{}
	blocking := &blockingRetirementEventBus{
		recordingEventBus: recorded,
		retirementEntered: make(chan struct{}),
		releaseRetirement: make(chan struct{}),
	}
	svc.eventBus = blocking
	svc.registerBackgroundWork(sessionID, "tool-old", "execution-old", "work-old")
	svc.markForegroundIdle(sessionID)

	publication, changed := svc.retireExecutionActivitySnapshot(sessionID, "execution-old")
	if !changed {
		t.Fatal("final execution retirement did not create a publication")
	}
	retirementDone := make(chan struct{})
	go func() {
		svc.publishForegroundActivitySnapshot(t.Context(), taskID, sessionID, publication)
		close(retirementDone)
	}()
	<-blocking.retirementEntered

	svc.registerBackgroundWorkKind(
		sessionID,
		"tool-new",
		"execution-new",
		"work-new",
		streams.BackgroundWorkKindSubagent,
	)
	svc.markForegroundIdle(sessionID)
	successorDone := make(chan struct{})
	go func() {
		svc.publishForegroundActivityChanged(t.Context(), taskID, sessionID)
		close(successorDone)
	}()

	// The predecessor has passed identity validation and is blocked in delivery.
	// A successor publication must queue behind that delivery even though it owns
	// a different turnActivity record.
	select {
	case <-successorDone:
		t.Fatal("successor publication completed before predecessor delivery was released — publication guard is not serializing")
	case <-time.After(100 * time.Millisecond):
	}
	close(blocking.releaseRetirement)
	<-retirementDone
	<-successorDone

	var values []interface{}
	var counts []int
	for _, record := range recorded.events {
		if record.subject != eventtypes.TaskSessionActivityChanged {
			continue
		}
		data, _ := record.event.Data.(map[string]interface{})
		values = append(values, data["foreground_activity"])
		counts = append(counts, data["active_subagent_count"].(int))
	}
	want := []interface{}{
		nil,
		nil,
	}
	if !slices.Equal(values, want) {
		t.Fatalf("cross-generation publications = %#v, want %#v", values, want)
	}
	if !slices.Equal(counts, []int{0, 1}) {
		t.Fatalf("cross-generation subagent counts = %v, want [0 1]", counts)
	}
	svc.activityPublicationGuardsMu.Lock()
	guardCount := len(svc.activityPublicationGuards)
	svc.activityPublicationGuardsMu.Unlock()
	if guardCount != 0 {
		t.Fatalf("publication guard registry retained %d idle entries", guardCount)
	}
}

func TestExecutionCleanup_PreservesSuccessorPromptClaimAndGeneration(t *testing.T) {
	const (
		sessionID    = "session-successor-claim"
		oldExecution = "execution-old"
		newExecution = "execution-new"
	)
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.registerBackgroundWork(sessionID, "old-work", oldExecution, "work-old")
	svc.markForegroundIdle(sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("successor prompt did not claim predecessor background-idle activity")
	}
	svc.retireExecutionActivitySnapshot(sessionID, oldExecution)
	if !svc.isForegroundClaimCurrent(sessionID, claim) {
		t.Fatal("predecessor cleanup invalidated the successor prompt claim")
	}

	dispatch := svc.beginForegroundDispatch(sessionID, claim, newExecution)
	if dispatch == nil {
		t.Fatal("successor prompt claim could not establish its prompt generation")
	}
	if dispatch.generation == 0 {
		t.Fatal("successor prompt generation was not advanced")
	}
	svc.acceptForegroundDispatch(dispatch)

	ta := svc.lockTurnActivity(sessionID, false)
	if ta == nil {
		t.Fatal("predecessor cleanup retired the successor-owned activity record")
	}
	defer ta.mu.Unlock()
	if _, ok := ta.executionClaims[newExecution]; !ok {
		t.Fatalf("successor execution %q did not retain its activity claim", newExecution)
	}
	if ta.promptCycleGeneration != dispatch.generation {
		t.Fatalf("prompt generation = %d, want %d", ta.promptCycleGeneration, dispatch.generation)
	}
}

func TestExecutionCleanup_RetiresRecordAfterSuccessorClaimIsAbandoned(t *testing.T) {
	const (
		sessionID    = "session-abandoned-successor-claim"
		oldExecution = "execution-old"
	)
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.registerBackgroundWork(sessionID, "old-work", oldExecution, "work-old")
	svc.markForegroundIdle(sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("successor prompt did not claim predecessor background-idle activity")
	}
	svc.retireExecutionActivitySnapshot(sessionID, oldExecution)
	if svc.releaseForegroundClaim(claim) {
		t.Fatal("abandoned claim reopened background-idle without live background work")
	}
	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("abandoned successor claim retained an otherwise unused activity record")
	}
}

func TestExecutionCleanup_RetiresRecordAfterDispatchAcceptReleasesLastToken(t *testing.T) {
	const sessionID = "session-dispatch-accept-retirement"
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.registerBackgroundWork(sessionID, "old-work", "execution-old", "work-old")
	dispatch := svc.beginForegroundDispatch(sessionID, nil)
	if dispatch == nil {
		t.Fatal("failed to begin successor dispatch")
	}

	svc.retireExecutionActivitySnapshot(sessionID, "execution-old")
	svc.acceptForegroundDispatch(dispatch)

	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("dispatch accept retained an ownerless record after terminal teardown")
	}
}

func TestExecutionCleanup_RetiresRecordAfterDispatchRollbackReleasesLastToken(t *testing.T) {
	const sessionID = "session-dispatch-rollback-retirement"
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.registerBackgroundWork(sessionID, "old-work", "execution-old", "work-old")
	dispatch := svc.beginForegroundDispatch(sessionID, nil)
	if dispatch == nil {
		t.Fatal("failed to begin successor dispatch")
	}

	svc.retireExecutionActivitySnapshot(sessionID, "execution-old")
	svc.rollbackForegroundDispatch(dispatch)

	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("dispatch rollback retained an ownerless record after terminal teardown")
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
		publication, changed := svc.retireExecutionActivitySnapshot(sessionID, "execution-old")
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
	if len(values) != 1 || values[0] != nil {
		t.Fatalf("activity publications = %#v, want only successor clear", values)
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
	if len(values) != 1 || values[0] != nil {
		t.Fatalf("activity publications = %#v, want only successor clear", values)
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
		publication, changed := svc.retireExecutionActivitySnapshot(sessionID, "execution-old")
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
	if len(values) != 1 || values[0] != nil {
		t.Fatalf("activity publications = %#v, want only successor clear", values)
	}
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
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
	if len(values) != 1 || values[0] != nil {
		t.Fatalf("activity publications = %#v, want only successor clear", values)
	}
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityGenerating {
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

	var stateClears, activityRetirements int
	for _, record := range recorded.events {
		data, ok := record.event.Data.(map[string]interface{})
		if !ok {
			continue
		}
		switch record.subject {
		case eventtypes.TaskSessionStateChanged:
			if value, present := data["foreground_activity"]; present && value == nil {
				stateClears++
			}
		case eventtypes.TaskSessionActivityChanged:
			if count, present := data["active_subagent_count"]; present && count == 0 {
				activityRetirements++
			}
		}
	}
	if stateClears != 1 || activityRetirements != 1 {
		t.Fatalf(
			"session.stop retirement cardinality: state=%d activity=%d, want 1/1",
			stateClears,
			activityRetirements,
		)
	}
	// Two task-aggregate recomputes: the teardown activity clear, plus the
	// republish when the session leaves RUNNING (republishTaskActivityOnSettle),
	// which corrects a task aggregate that a detached record would otherwise leave
	// stuck at generating.
	if len(taskEvents.activityTaskIDs) != 2 ||
		taskEvents.activityTaskIDs[0] != taskID ||
		taskEvents.activityTaskIDs[1] != taskID {
		t.Fatalf("task aggregate cleanup publications = %v, want [%s %s]", taskEvents.activityTaskIDs, taskID, taskID)
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
			if _, ok := turnActivityRecord(t, svc, sessionID); ok {
				t.Fatal("terminal execution event retained the session activity record")
			}
			if got := countActivityClears(recorded); got != 1 {
				t.Fatalf("terminal session activity clears = %d, want exactly one", got)
			}
			// Three task-aggregate recomputes: the retirement activity clear, the
			// reconciled background completion, plus the settle republish when the
			// session leaves RUNNING (republishTaskActivityOnSettle).
			if len(taskEvents.activityTaskIDs) != 3 ||
				taskEvents.activityTaskIDs[0] != taskID ||
				taskEvents.activityTaskIDs[1] != taskID ||
				taskEvents.activityTaskIDs[2] != taskID {
				t.Fatalf("terminal task recomputes = %v, want [%s %s %s]", taskEvents.activityTaskIDs, taskID, taskID, taskID)
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
		if count, present := data["active_subagent_count"]; present && count == 0 {
			clears++
		}
	}
	return clears
}

func TestTransientFailurePreservesBackgroundRegistration(t *testing.T) {
	svc, _ := newTransientTestService(t)
	t.Cleanup(svc.cancelAllTransientRetries)
	svc.beginPromptAttempt("s1", "exec-transient", 7, false)
	recorded := &recordingEventBus{}
	svc.eventBus = recorded
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)
	svc.registerBackgroundWork("s1", "tool-transient", "exec-transient", "work-transient")
	svc.markForegroundIdle("s1")

	svc.handleAgentFailed(t.Context(), watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-transient",
		PromptGeneration: 7,
		ErrorMessage:     overloaded529,
	})

	if !svc.hasBackgroundTask("s1", "tool-transient") {
		t.Fatal("transient failure cleaned background registration before retry teardown")
	}
	// The transient path keeps the background registration and never clears the
	// session-level activity (no session activity_changed clear). It does park the
	// session in WAITING_FOR_INPUT, so the task aggregate is republished once
	// (republishTaskActivityOnSettle) to reflect the calm, non-generating retry
	// state on the sidebar/board.
	if got := countActivityClears(recorded); got != 0 {
		t.Fatalf("transient cleanup must not clear session activity, got %d", got)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != "t1" {
		t.Fatalf("transient park must republish task aggregate once, got %v", taskEvents.activityTaskIDs)
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
	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("forced final cleanup retained the session activity record")
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
		t.Fatalf("forced cleanup task recomputes = %v, want [%s]", taskEvents.activityTaskIDs, taskID)
	}
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: "execution-new",
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventToolCall, ToolCallID: "late-forced-tool", ToolStatus: "in_progress",
		},
	})
	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("late forced-cleanup frame recreated the retired activity record")
	}
}

func TestClearTurnActivity_InvalidatesOutstandingTokensAndIsIdempotent(t *testing.T) {
	const sessionID = "session-delete-activity"
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.registerBackgroundWork(sessionID, "work", "execution", "work")
	svc.markForegroundIdle(sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("failed to create foreground claim")
	}

	svc.clearTurnActivity(sessionID)
	svc.clearTurnActivity(sessionID)

	if svc.releaseForegroundClaim(claim) {
		t.Fatal("deleted activity record accepted a stale foreground claim")
	}
	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("repeated activity deletion recreated the record")
	}
}

func TestDeleteSession_StopsLiveExecutionAndDropsLateFrames(t *testing.T) {
	const (
		taskID      = "task-delete-live"
		sessionID   = "session-delete-live"
		executionID = "execution-delete-live"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.registerBackgroundWork(sessionID, "background-live", executionID, "work-live")

	if err := svc.DeleteSession(t.Context(), sessionID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	manager.mu.Lock()
	stopCalls := append([]stopAgentCall(nil), manager.stopAgentWithReasonArgs...)
	manager.mu.Unlock()
	if len(stopCalls) != 1 || stopCalls[0] != (stopAgentCall{
		ExecutionID: executionID,
		Reason:      "session deleted",
		Force:       true,
	}) {
		t.Fatalf("session deletion stop calls = %#v", stopCalls)
	}

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "late-tool",
			ToolStatus: "in_progress",
		},
	})
	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("late deleted-session frame recreated activity")
	}
}

func TestDeleteSession_StopFailurePreservesSessionAndActivity(t *testing.T) {
	const (
		taskID      = "task-delete-stop-failure"
		sessionID   = "session-delete-stop-failure"
		executionID = "execution-delete-stop-failure"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		stopAgentWithReasonErr: errors.New("runtime still active"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.registerBackgroundWork(sessionID, "background-live", executionID, "work-live")

	if err := svc.DeleteSession(t.Context(), sessionID); err == nil {
		t.Fatal("DeleteSession succeeded while its live execution could not be stopped")
	}
	if _, err := repo.GetTaskSession(t.Context(), sessionID); err != nil {
		t.Fatalf("session was deleted after stop failure: %v", err)
	}
	if !svc.hasBackgroundTask(sessionID, "background-live") {
		t.Fatal("stop failure retired activity still owned by the live execution")
	}
	if svc.isExecutionCompleted(sessionID, executionID) {
		t.Fatal("stop failure terminal-marked a still-live execution")
	}
}

func TestDeleteSession_AlreadyMissingRuntimeStillDeletesSession(t *testing.T) {
	const (
		taskID      = "task-delete-runtime-gone"
		sessionID   = "session-delete-runtime-gone"
		executionID = "execution-delete-runtime-gone"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return executionID, nil
		},
		stopAgentWithReasonErr: lifecycle.ErrExecutionNotFound,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.registerBackgroundWork(sessionID, "background-stale", executionID, "work-stale")

	if err := svc.DeleteSession(t.Context(), sessionID); err != nil {
		t.Fatalf("DeleteSession with already-missing runtime: %v", err)
	}
	if _, err := repo.GetTaskSession(t.Context(), sessionID); err == nil {
		t.Fatal("session remained after its runtime was already gone")
	}
	if _, ok := turnActivityRecord(t, svc, sessionID); ok {
		t.Fatal("already-missing runtime left stale activity after session deletion")
	}
	if !svc.isExecutionCompleted(sessionID, executionID) {
		t.Fatal("already-missing runtime was not terminal-marked before deletion")
	}
}

func TestBackgroundActivity_ClaudeMetadataOnlyChildUpdateDoesNotReopenForeground(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	const taskID, sessionID = "task-claude-child", "session-claude-child"

	svc.registerBackgroundTask(sessionID, "async-agent")
	svc.markForegroundIdle(sessionID)
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("precondition activity = %q, want background", got)
	}

	// Claude ACP 0.62.0 emits an initial result frame for a tool inside an async
	// Agent without status, parentToolUseId, or a normalized payload. The next
	// frame attributes the tool to its parent, but this incomplete first frame
	// must not impersonate new top-level foreground work in the meantime.
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "child-bash",
		},
	})

	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("metadata-only child update changed activity to %q, want background", got)
	}
}

func TestBackgroundActivity_ClaudeCachedChildUpdateDoesNotReopenForeground(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}
	const taskID, sessionID = "task-claude-cached-child", "session-claude-cached-child"

	svc.registerBackgroundTask(sessionID, "async-agent")
	svc.markForegroundIdle(sessionID)

	childPayload := streams.NewGeneric("other", map[string]any{"query": "select:Monitor"})
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventToolCall,
			ToolCallID:       "child-tool-search",
			ParentToolCallID: "async-agent",
			ToolStatus:       "running",
			Normalized:       childPayload,
		},
	})

	// Claude's next update temporarily omits parentToolUseId. The ACP adapter
	// still attaches the normalized payload cached from the initial child call,
	// so ownership must come from the original call rather than this partial
	// update's shape.
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "child-tool-search",
			Normalized: childPayload,
		},
	})

	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("cached child update changed activity to %q, want background", got)
	}
}

func TestBackgroundActivity_UnknownToolUpdatePreservesActivity(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	const taskID, sessionID = "task-unknown-tool", "session-unknown-tool"

	svc.registerBackgroundTask(sessionID, "async-agent")
	svc.markForegroundIdle(sessionID)
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "update-without-initial-call",
			ToolStatus: "in_progress",
			Normalized: streams.NewGeneric("other", map[string]any{"result": "partial"}),
		},
	})

	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("unknown tool update changed activity to %q, want background", got)
	}
}

// TestBackgroundWork_RegisteredFromInitialToolCallIsRetiredOnExecutionTeardown
// covers the initial tool_call registration path (as opposed to the
// tool_update path exercised by registerAsyncWorkForExecution above). The
// initial tool_call is the only frame handleToolCallEvent sees before
// hasBackgroundTask starts short-circuiting later updates, so it must carry
// the launching execution ID itself. Without that, execution teardown
// (retireExecutionActivitySnapshot, keyed by executionID) can never
// match this registration, and the session reports background-running
// forever even after its execution has died.
func TestBackgroundWork_RegisteredFromInitialToolCallIsRetiredOnExecutionTeardown(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID, sessionID, executionID = "task-initial-call", "session-initial-call", "execution-initial-call"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	payload := attestedSubagentPayload("background work", "do it", "general-purpose")
	payload.SubagentTask().IsAsync = true
	payload.SubagentTask().AgentID = "work-initial-call"
	payload.SetBackgroundWorkIdentity(
		streams.BackgroundWorkKindSubagent,
		"work-initial-call",
		true,
		false,
	)
	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "tool-initial-call",
			ToolStatus: "pending",
			Normalized: payload,
		},
	})
	svc.markForegroundIdle(sessionID)
	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("initial tool_call registration did not surface as background, got %q", got)
	}

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
	})

	if svc.hasBackgroundTask(sessionID, "tool-initial-call") {
		t.Fatal("execution teardown left the initial tool_call registration behind")
	}
	if got := svc.foregroundActivityValue(sessionID); got == v1.ForegroundActivityBackground {
		t.Fatalf("session still reports background after owning execution died, got %q", got)
	}
}
