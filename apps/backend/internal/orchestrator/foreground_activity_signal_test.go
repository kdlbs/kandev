package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type failOnceGetTaskSessionRepo struct {
	repoStore
	err    error
	failed bool
}

func (r *failOnceGetTaskSessionRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	if !r.failed {
		r.failed = true
		return nil, r.err
	}
	return r.repoStore.GetTaskSession(ctx, id)
}

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

	// A top-level subagent tool_call registers background work, but the launch
	// itself cannot override the foreground.
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
	emitForegroundIdle(svc, taskID, sessionID)

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
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeForegroundIdle},
	})

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
	svc.markForegroundIdle(sessionID)

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
	svc.markForegroundIdle(sessionID)

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
	svc.markForegroundIdle(sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("prompt must claim the background-idle turn")
	}
	svc.registerBackgroundTask(sessionID, "background-2")
	if s := svc.ForegroundActivity(sessionID); s != v1.ForegroundActivityGenerating {
		t.Fatalf("active claim must remain generating, got %q", s)
	}

	if svc.completeForegroundClaim(claim) {
		t.Fatal("dispatch alone must not expose background work before foreground idle")
	}
	if !svc.markForegroundIdle(sessionID) {
		t.Fatal("foreground idle must expose work registered during admission")
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

func (r *recordingTaskEvents) PublishTaskUpdated(context.Context, *models.Task, ...string) {}

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
	emitForegroundIdle(svc, taskID, sessionID)
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

func TestForegroundActivitySignal_TurnCompletionPublishesBackgroundExactlyOnce(t *testing.T) {
	tests := []struct {
		name string
		act  func(*Service, string, string)
	}{
		{
			name: "completion without provider idle",
			act: func(svc *Service, _, sessionID string) {
				svc.completeTurnForSession(t.Context(), sessionID)
			},
		},
		{
			name: "repeated completion",
			act: func(svc *Service, _, sessionID string) {
				svc.completeTurnForSession(t.Context(), sessionID)
				svc.completeTurnForSession(t.Context(), sessionID)
			},
		},
		{
			name: "provider idle before completion",
			act: func(svc *Service, taskID, sessionID string) {
				emitForegroundIdle(svc, taskID, sessionID)
				svc.completeTurnForSession(t.Context(), sessionID)
			},
		},
		{
			name: "provider idle after completion",
			act: func(svc *Service, taskID, sessionID string) {
				svc.completeTurnForSession(t.Context(), sessionID)
				emitForegroundIdle(svc, taskID, sessionID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			eb := &recordingEventBus{}
			svc.eventBus = eb
			taskEvents := &recordingTaskEvents{}
			svc.SetTaskEventPublisher(taskEvents)

			const (
				taskID    = "task-completion-signal"
				sessionID = "session-completion-signal"
			)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
			svc.registerBackgroundTask(sessionID, "subagent-1")

			tt.act(svc, taskID, sessionID)

			got := activityValues(eb)
			want := []string{string(v1.ForegroundActivityBackground)}
			if len(got) != len(want) || got[0] != want[0] {
				t.Fatalf("expected exactly one session background publication %v, got %v", want, got)
			}
			if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
				t.Fatalf("expected exactly one task aggregate publication for %q, got %v", taskID, taskEvents.activityTaskIDs)
			}
			for _, rec := range eb.events {
				if rec.subject != events.TaskSessionActivityChanged {
					continue
				}
				data, ok := rec.event.Data.(map[string]interface{})
				if !ok || data[metaKeyTaskID] != taskID || data[metaKeySessionID] != sessionID {
					t.Fatalf("activity publication lost task/session identity: %#v", rec.event.Data)
				}
			}
		})
	}
}

func TestForegroundActivitySignal_TurnCompletionPreservesFinalBackgroundCompletion(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID    = "task-completion-finished"
		sessionID = "session-completion-finished"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionID, "subagent-1")
	svc.completeTurnForSession(t.Context(), sessionID)

	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data:      &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})

	got := activityValues(eb)
	want := []string{string(v1.ForegroundActivityBackground), string(v1.ForegroundActivityGenerating)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected completion and final-background publications %v, got %v", want, got)
	}
	if len(taskEvents.activityTaskIDs) != 2 || taskEvents.activityTaskIDs[0] != taskID || taskEvents.activityTaskIDs[1] != taskID {
		t.Fatalf("expected one task aggregate publication per real flip for %q, got %v", taskID, taskEvents.activityTaskIDs)
	}
}

func TestForegroundActivitySignal_DelayedOldCompletionCannotYieldSuccessor(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID    = "task-delayed-completion"
		sessionID = "session-delayed-completion"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionID, "subagent-1")
	emitForegroundIdle(svc, taskID, sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("successor prompt must claim the background-idle foreground")
	}
	svc.completeForegroundClaim(claim)
	svc.markForegroundGenerating(sessionID)
	eb.events = nil
	taskEvents.activityTaskIDs = nil

	// This is the old cycle's delayed completion, arriving after its successor
	// has already claimed and begun generating in the same session.
	svc.completeTurnForSession(t.Context(), sessionID)

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("delayed old completion yielded successor foreground: got %q", got)
	}
	if got := activityValues(eb); len(got) != 0 {
		t.Fatalf("delayed old completion must not publish successor as background, got %v", got)
	}
	if len(taskEvents.activityTaskIDs) != 0 {
		t.Fatalf("delayed old completion must not recompute task activity, got %v", taskEvents.activityTaskIDs)
	}
}

func TestForegroundActivitySignal_OldCompletionBeforeSuccessorLeavesSuccessorCompletionValid(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID    = "task-ordered-completion"
		sessionID = "session-ordered-completion"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionID, "subagent-1")
	emitForegroundIdle(svc, taskID, sessionID)
	svc.completeTurnForSession(t.Context(), sessionID)

	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("successor prompt must claim after old completion")
	}
	svc.completeForegroundClaim(claim)
	svc.markForegroundGenerating(sessionID)
	eb.events = nil
	taskEvents.activityTaskIDs = nil

	// With the old completion consumed before the successor began, completing
	// the successor is current and must expose the still-running background task.
	svc.completeTurnForSession(t.Context(), sessionID)

	if got := activityValues(eb); len(got) != 1 || got[0] != string(v1.ForegroundActivityBackground) {
		t.Fatalf("current successor completion must publish background once, got %v", got)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
		t.Fatalf("current successor completion must recompute task once, got %v", taskEvents.activityTaskIDs)
	}
}

func TestForegroundActivitySignal_TaskLookupFailureDoesNotConsumeCompletionYield(t *testing.T) {
	baseRepo := setupTestRepo(t)
	svc := createTestService(baseRepo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID    = "task-lookup-retry"
		sessionID = "session-lookup-retry"
	)
	seedTaskAndSession(t, baseRepo, taskID, sessionID, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionID, "subagent-1")
	svc.repo = &failOnceGetTaskSessionRepo{repoStore: baseRepo, err: errors.New("transient lookup failure")}

	svc.completeTurnForSession(t.Context(), sessionID)
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("failed identity lookup must not consume transition, got %q", got)
	}
	if got := activityValues(eb); len(got) != 0 {
		t.Fatalf("failed identity lookup must not publish, got %v", got)
	}

	svc.completeTurnForSession(t.Context(), sessionID)
	if got := activityValues(eb); len(got) != 1 || got[0] != string(v1.ForegroundActivityBackground) {
		t.Fatalf("retry must publish preserved background transition once, got %v", got)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
		t.Fatalf("retry must recompute owning task once, got %v", taskEvents.activityTaskIDs)
	}
}

func TestForegroundActivitySignal_TurnCompletionIsSessionIsolated(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskA    = "task-completion-a"
		sessionA = "session-completion-a"
		taskB    = "task-completion-b"
		sessionB = "session-completion-b"
	)
	seedTaskAndSession(t, repo, taskA, sessionA, models.TaskSessionStateRunning)
	seedTaskAndSession(t, repo, taskB, sessionB, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionA, "subagent-a")
	svc.registerBackgroundTask(sessionB, "subagent-b")

	svc.completeTurnForSession(t.Context(), sessionA)

	if got := svc.ForegroundActivity(sessionA); got != v1.ForegroundActivityBackground {
		t.Fatalf("completed session A must become background, got %q", got)
	}
	if got := svc.ForegroundActivity(sessionB); got != v1.ForegroundActivityGenerating {
		t.Fatalf("completion for session A mutated session B: got %q", got)
	}
	if got := activityValues(eb); len(got) != 1 || got[0] != string(v1.ForegroundActivityBackground) {
		t.Fatalf("expected one session A background event, got %v", got)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskA {
		t.Fatalf("completion for session A published another task: %v", taskEvents.activityTaskIDs)
	}
	for _, rec := range eb.events {
		if rec.subject != events.TaskSessionActivityChanged {
			continue
		}
		data, ok := rec.event.Data.(map[string]interface{})
		if !ok || data[metaKeyTaskID] != taskA || data[metaKeySessionID] != sessionA {
			t.Fatalf("completion for session A published another session: %#v", rec.event.Data)
		}
	}
}

func TestForegroundActivitySignal_DelayedOldProviderIdleCannotYieldSuccessor(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{currentPromptExecutionID: "execution-provider-idle"}
	agentMgr.currentPromptGeneration.Store(1)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	eb := &recordingEventBus{}
	svc.eventBus = eb
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID    = "task-delayed-provider-idle"
		sessionID = "session-delayed-provider-idle"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(sessionID, "subagent-1")

	// The old turn completes without its provider-idle frame arriving yet.
	svc.completeTurnForSession(t.Context(), sessionID)
	claim := svc.claimForegroundTurn(sessionID)
	if claim == nil {
		t.Fatal("successor prompt must claim the completion-yielded foreground")
	}
	svc.completeForegroundClaim(claim)
	svc.markForegroundGenerating(sessionID)
	agentMgr.currentPromptGeneration.Store(2)
	eb.events = nil
	taskEvents.activityTaskIDs = nil

	// The old turn's delayed provider-idle arrives after the successor started.
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:      taskID,
		SessionID:   sessionID,
		ExecutionID: "execution-provider-idle",
		Data: &lifecycle.AgentStreamEventData{
			Type:             streams.EventTypeForegroundIdle,
			PromptGeneration: 1,
		},
	})

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("delayed old provider idle yielded successor foreground: got %q", got)
	}
	if got := activityValues(eb); len(got) != 0 {
		t.Fatalf("delayed old provider idle must not publish background, got %v", got)
	}
	if len(taskEvents.activityTaskIDs) != 0 {
		t.Fatalf("delayed old provider idle must not recompute task activity, got %v", taskEvents.activityTaskIDs)
	}

	// The actual successor's idle signal carries the current immutable prompt
	// generation and must still expose background work immediately.
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:      taskID,
		SessionID:   sessionID,
		ExecutionID: "execution-provider-idle",
		Data: &lifecycle.AgentStreamEventData{
			Type:             streams.EventTypeForegroundIdle,
			PromptGeneration: 2,
		},
	})
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("current provider idle must yield promptly, got %q", got)
	}
	if got := activityValues(eb); len(got) != 1 || got[0] != string(v1.ForegroundActivityBackground) {
		t.Fatalf("current provider idle must publish background once, got %v", got)
	}
	if len(taskEvents.activityTaskIDs) != 1 || taskEvents.activityTaskIDs[0] != taskID {
		t.Fatalf("current provider idle must recompute task once, got %v", taskEvents.activityTaskIDs)
	}
}
