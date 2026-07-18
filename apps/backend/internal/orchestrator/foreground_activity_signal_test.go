package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// activityValues returns the foreground_activity payloads of every
// task_session.activity_changed event recorded on the bus, in publish order.
// It is the operator-facing WS1 signal the composer and status indicator read.
func activityValues(eb *recordingEventBus) []string {
	var vals []string
	for _, rec := range eb.events {
		if rec.subject != events.TaskSessionActivityChanged {
			continue
		}
		if data, ok := rec.event.Data.(map[string]interface{}); ok {
			if v, ok := data["foreground_activity"].(string); ok {
				vals = append(vals, v)
			}
		}
	}
	return vals
}

// TestForegroundActivitySignal_PublishesOnFlips proves the WS1 producer emits
// the fine-grained busy signal exactly when the foreground/background substate
// flips — background when the agent yields to a spawned task, generating again
// when it streams foreground output — so the web composer/status can distinguish
// the three conditions without a coarse session-state transition.
func TestForegroundActivitySignal_PublishesOnFlips(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-activity"
	)

	// A top-level subagent tool_call: the foreground yields to background work.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
		},
	})

	// A streamed foreground message: the agent is generating again even though
	// the subagent is still outstanding.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:      "message_streaming",
			MessageID: "m1",
			Text:      "still working on it",
		},
	})

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground), string(v1.ForegroundActivityGenerating)}
	if len(got) != len(want) {
		t.Fatalf("expected activity signal on each flip %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("activity flip %d: expected %q, got %q (all: %v)", i, want[i], got[i], got)
		}
	}
}

// TestForegroundActivitySignal_NoPublishWithoutFlip proves the signal is emitted
// only on a real substate transition, never per background frame: a second
// concurrent background task, and completing all-but-the-last, must NOT publish.
func TestForegroundActivitySignal_NoPublishWithoutFlip(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-activity-2"
	)

	subagent := func(id string) *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:    taskID,
			SessionID: sessionID,
			Data: &lifecycle.AgentStreamEventData{
				Type:       agentEventToolCall,
				ToolCallID: id,
				ToolStatus: "running",
				Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
			},
		}
	}
	terminal := func(id string) *lifecycle.AgentStreamEventPayload {
		return &lifecycle.AgentStreamEventPayload{
			TaskID:    taskID,
			SessionID: sessionID,
			Data: &lifecycle.AgentStreamEventData{
				Type:       "tool_update",
				ToolCallID: id,
				ToolStatus: agentEventComplete,
				Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
			},
		}
	}

	// First background task flips to background (publish #1). The second does not
	// flip anything (already yielded) — no publish.
	svc.handleAgentStreamEvent(context.Background(), subagent("subagent-1"))
	svc.handleAgentStreamEvent(context.Background(), subagent("subagent-2"))

	// Completing the first while the second is still outstanding does not flip —
	// no publish. Completing the last flips back to generating (publish #2).
	svc.handleAgentStreamEvent(context.Background(), terminal("subagent-1"))
	svc.handleAgentStreamEvent(context.Background(), terminal("subagent-2"))

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground), string(v1.ForegroundActivityGenerating)}
	if len(got) != len(want) {
		t.Fatalf("expected exactly one publish per real flip %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flip %d: expected %q, got %q (all: %v)", i, want[i], got[i], got)
		}
	}
}

func TestForegroundActivitySignal_ClaimReleasePublishesRestoredBackground(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	eb := &recordingEventBus{}
	svc.eventBus = eb

	const (
		taskID    = "task1"
		sessionID = "session-release-signal"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionID, "background-1")

	// Admission claims the foreground, but the missing executor row makes
	// ensureSessionRunning fail before the prompt reaches the agent. Releasing
	// that claim must tell every client that background-idle was restored.
	if _, err := svc.PromptTask(context.Background(), taskID, sessionID, "retry", "", false, nil, false); err == nil {
		t.Fatal("expected prompt preflight to fail without an executor record")
	}

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground)}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected restored background activity broadcast %v, got %v", want, got)
	}
}

func TestForegroundActivitySignal_ModelSwitchPublishesClaimedGenerating(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-2"}, nil
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	eb := &recordingEventBus{}
	svc.eventBus = eb

	const (
		taskID    = "task1"
		sessionID = "session-model-switch-signal"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
	svc.registerBackgroundTask(sessionID, "background-1")

	result, err := svc.PromptTask(context.Background(), taskID, sessionID, "continue", "new-model", false, nil, false)
	if err != nil {
		t.Fatalf("model-switch prompt failed: %v", err)
	}
	if result == nil || result.StopReason != "model_switched" {
		t.Fatalf("expected restart-based model switch result, got %#v", result)
	}

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityGenerating)}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected claimed foreground activity broadcast %v, got %v", want, got)
	}
}

func TestForegroundActivitySignal_DispatchPublishesBackgroundRegisteredDuringClaim(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb

	const (
		taskID    = "task1"
		sessionID = "session-dispatch-signal"
	)
	svc.registerBackgroundTask(sessionID, "background-1")
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("prompt must claim the background-idle turn")
	}
	if svc.registerBackgroundTask(sessionID, "background-2") {
		t.Fatal("background registration must not publish through an active claim")
	}
	if s := svc.ForegroundActivity(sessionID); s != v1.ForegroundActivityGenerating {
		t.Fatalf("active claim must remain generating, got %q", s)
	}

	if !svc.completeForegroundClaim(claim) {
		t.Fatal("dispatch must expose the background work registered during admission")
	}
	svc.publishForegroundActivityChanged(context.Background(), taskID, sessionID)

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground)}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected dispatch-time background activity broadcast %v, got %v", want, got)
	}
}

// recordingTaskEvents captures task-level publish calls so the test can assert
// the per-session activity flip is propagated to the task-level aggregate.
type recordingTaskEvents struct {
	activityTaskIDs []string
}

func (r *recordingTaskEvents) PublishTaskUpdated(context.Context, *models.Task) {}

func (r *recordingTaskEvents) PublishTaskStateChanged(context.Context, *models.Task, v1.TaskState) {
}

func (r *recordingTaskEvents) PublishTaskActivityIfChanged(_ context.Context, taskID string) {
	r.activityTaskIDs = append(r.activityTaskIDs, taskID)
}

// TestForegroundActivitySignal_PropagatesToTaskLevel proves each per-session flip
// also drives the task-level MOST-ACTIVE-WINS recompute so at-a-glance task
// surfaces (board card, task list) update live (§spec:task-level-indicator).
func TestForegroundActivitySignal_PropagatesToTaskLevel(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID    = "task1"
		sessionID = "session-tasklevel"
	)

	// Yield to background work, then stream foreground output again: two flips.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
		},
	})
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:      "message_streaming",
			MessageID: "m1",
			Text:      "still working on it",
		},
	})

	want := []string{taskID, taskID}
	if len(taskEvents.activityTaskIDs) != len(want) {
		t.Fatalf("expected task-level recompute on each flip %v, got %v", want, taskEvents.activityTaskIDs)
	}
	for i := range want {
		if taskEvents.activityTaskIDs[i] != want[i] {
			t.Fatalf("task-level recompute %d: got %q, want %q", i, taskEvents.activityTaskIDs[i], want[i])
		}
	}
}
