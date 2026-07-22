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

// beforeDispatchAgentManager models agentctl's real ordering: provider stream
// events may be forwarded by the prompt goroutine before the accepted response
// reaches finishAcceptedPrompt and invokes onDispatched.
type beforeDispatchAgentManager struct {
	*mockAgentManager
	beforeDispatched func()
}

func (m *beforeDispatchAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	result, err := m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
	if m.beforeDispatched != nil {
		m.beforeDispatched()
	}
	if err == nil && onDispatched != nil {
		onDispatched()
	}
	return result, err
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

func TestForegroundActivitySignal_SamePromptOutputAfterIdleDoesNotInvalidateCompletion(t *testing.T) {
	tests := []struct {
		name       string
		outputType string
		outputData *lifecycle.AgentStreamEventData
	}{
		{
			name:       "final assistant output",
			outputType: "message_streaming",
			outputData: &lifecycle.AgentStreamEventData{
				Type:      "message_streaming",
				MessageID: "message-1",
				Text:      "final answer",
			},
		},
		{
			name:       "final thinking output",
			outputType: "thinking_streaming",
			outputData: &lifecycle.AgentStreamEventData{
				Type:      "thinking_streaming",
				MessageID: "thinking-1",
				Text:      "final reasoning",
			},
		},
		{
			name:       "terminal foreground tool update with output",
			outputType: "tool_update",
			outputData: &lifecycle.AgentStreamEventData{
				Type:       "tool_update",
				ToolCallID: "foreground-tool-1",
				ToolStatus: agentEventCompleted,
				ToolCallContents: []streams.ToolCallContentItem{
					{
						Type: "content",
						Content: &streams.ContentBlock{
							Type: "text",
							Text: "command finished successfully",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			eb := &recordingEventBus{}
			svc.eventBus = eb
			svc.messageCreator = &mockMessageCreator{}
			taskEvents := &recordingTaskEvents{}
			svc.SetTaskEventPublisher(taskEvents)

			const (
				taskID    = "task-same-prompt-output"
				sessionID = "session-same-prompt-output"
			)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)

			// This is the async-subagent ordering observed from the provider: the
			// foreground first yields, then emits one last frame from the same
			// prompt before its eventual completion arrives.
			svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
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
			svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
				TaskID:    taskID,
				SessionID: sessionID,
				Data:      tt.outputData,
			})
			svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)

			if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
				t.Fatalf("same-prompt %s made completion stale: got activity %q", tt.outputType, got)
			}
			if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
				t.Fatalf("background-only session must accept immediate input: %v", err)
			}
			got := activityValues(eb)
			want := []string{
				string(v1.ForegroundActivityBackground),
				string(v1.ForegroundActivityGenerating),
				string(v1.ForegroundActivityBackground),
			}
			if len(got) != len(want) {
				t.Fatalf("activity publications = %v, want exactly %v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("activity publication %d = %q, want %q (all %v)", i, got[i], want[i], got)
				}
			}
			if len(taskEvents.activityTaskIDs) != 3 {
				t.Fatalf("task activity publications = %v, want exactly three", taskEvents.activityTaskIDs)
			}
			for _, publishedTaskID := range taskEvents.activityTaskIDs {
				if publishedTaskID != taskID {
					t.Fatalf("task activity publication used task %q, want %q", publishedTaskID, taskID)
				}
			}
		})
	}
}

func TestForegroundActivitySignal_DetachedLaunchTerminalOutputStaysBackground(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task-detached-launch-terminal"
		sessionID = "session-detached-launch-terminal"
		toolID    = "async-subagent-launch"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	launch := streams.NewSubagentTask("explore", "find files", "general-purpose")
	launch.SubagentTask().IsAsync = true
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: agentEventToolCall, ToolCallID: toolID, ToolStatus: "running", Normalized: launch,
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: toolID,
			ToolStatus: agentEventCompleted,
			Normalized: launch,
			ToolCallContents: []streams.ToolCallContentItem{{
				Type: "content", Content: &streams.ContentBlock{Type: "text", Text: "agent launched"},
			}},
		},
	})

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("terminal launch card must not impersonate foreground output: got %q", got)
	}
	if !svc.hasBackgroundTask(sessionID, toolID) {
		t.Fatal("terminal launch card must not complete its detached workload")
	}
	if got := activityValues(eb); len(got) != 1 || got[0] != string(v1.ForegroundActivityBackground) {
		t.Fatalf("detached launch should publish only the foreground idle transition, got %v", got)
	}
}

func TestForegroundActivitySignal_FollowUpPromptKeepsIndependentCompletionIdentity(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	eb := &recordingEventBus{}
	svc.eventBus = eb
	svc.messageCreator = &mockMessageCreator{}
	taskEvents := &recordingTaskEvents{}
	svc.SetTaskEventPublisher(taskEvents)

	const (
		taskID      = "task-follow-up-prompt-cycle"
		sessionID   = "session-follow-up-prompt-cycle"
		executionID = "execution-follow-up-prompt-cycle"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	// The same long-lived execution already completed an earlier prompt. This is
	// the history the direct stream-only regression lacked: completion generation
	// zero has already been consumed before the follow-up prompt is dispatched.
	svc.markForegroundGenerating(sessionID)
	svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)

	if _, err := svc.PromptTask(
		t.Context(), taskID, sessionID, "/async-subagent-lifecycle", "", false, nil, false,
	); err != nil {
		t.Fatalf("dispatch follow-up prompt: %v", err)
	}

	// Lifecycle delivers these through the orchestrator stream in this order:
	// async Agent launch, foreground idle, then buffered final thought/text as
	// completion flushes. agent.ready then completes the prompt before the
	// duplicate complete stream arrives.
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID:      taskID,
		SessionID:   sessionID,
		ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-follow-up",
			ToolStatus: "running",
			Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: "thinking_streaming", MessageID: "thinking-follow-up", Text: "return control",
		},
	})
	svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: "message_streaming", MessageID: "message-follow-up", Text: "foreground done",
		},
	})
	svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)
	svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("follow-up completion was mistaken for its predecessor: activity = %q", got)
	}
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("follow-up left background-only session unpromptable: %v", err)
	}
	got := activityValues(eb)
	want := []string{
		string(v1.ForegroundActivityBackground),
		string(v1.ForegroundActivityGenerating),
		string(v1.ForegroundActivityBackground),
	}
	if len(got) != len(want) {
		t.Fatalf("activity publications = %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("activity publication %d = %q, want %q (all %v)", i, got[i], want[i], got)
		}
	}
	if len(taskEvents.activityTaskIDs) != len(want) {
		t.Fatalf("task activity publications = %v, want exactly %d", taskEvents.activityTaskIDs, len(want))
	}
}

func TestForegroundActivitySignal_PreDispatchEventsUseCurrentPromptCycle(t *testing.T) {
	repo := setupTestRepo(t)
	baseAgentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	agentMgr := &beforeDispatchAgentManager{mockAgentManager: baseAgentMgr}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID      = "task-pre-dispatch-events"
		sessionID   = "session-pre-dispatch-events"
		executionID = "execution-pre-dispatch-events"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	// Consume the predecessor cycle. Without a new immutable identity before
	// PromptAgent starts, the completion emitted below is rejected as its duplicate.
	svc.markForegroundGenerating(sessionID)
	svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)

	eb := &recordingEventBus{}
	svc.eventBus = eb
	agentMgr.beforeDispatched = func() {
		svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
			TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
			Data: &lifecycle.AgentStreamEventData{
				Type:       agentEventToolCall,
				ToolCallID: "subagent-before-dispatch",
				ToolStatus: "running",
				Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
			},
		})
		emitForegroundIdle(svc, taskID, sessionID)
		svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
			TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
			Data: &lifecycle.AgentStreamEventData{
				Type: "message_streaming", MessageID: "final-before-dispatch", Text: "foreground done",
			},
		})
		svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)
	}

	if _, err := svc.PromptTask(
		t.Context(), taskID, sessionID, "launch async subagent", "", false, nil, false,
	); err != nil {
		t.Fatalf("dispatch follow-up prompt: %v", err)
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("pre-callback completion did not own the new prompt cycle: activity = %q", got)
	}
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("detached work after pre-callback completion must be promptable: %v", err)
	}
	got := activityValues(eb)
	want := []string{
		string(v1.ForegroundActivityBackground),
		string(v1.ForegroundActivityGenerating),
		string(v1.ForegroundActivityBackground),
	}
	if len(got) != len(want) {
		t.Fatalf("activity publications = %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("activity publication %d = %q, want %q (all %v)", i, got[i], want[i], got)
		}
	}
}

func TestForegroundActivitySignal_ClaimedPreDispatchCompletionReconcilesOnAccept(t *testing.T) {
	tests := []struct {
		name        string
		finalOutput bool
		keepWork    bool
	}{
		{name: "idle then complete with detached work", keepWork: true},
		{name: "idle final output then complete with detached work", finalOutput: true, keepWork: true},
		{name: "idle then complete after detached work finished"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			baseAgentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
			agentMgr := &beforeDispatchAgentManager{mockAgentManager: baseAgentMgr}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.messageCreator = &mockMessageCreator{}
			eb := &recordingEventBus{}
			svc.eventBus = eb

			const taskID, sessionID, executionID = "task-claimed-pre-callback", "session-claimed-pre-callback", "execution-claimed-pre-callback"
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
			seedExecutorRunning(t, repo, sessionID, taskID, executionID)
			svc.registerBackgroundWork(sessionID, "detached-work", executionID, "work-1")
			emitForegroundIdle(svc, taskID, sessionID)
			eb.events = nil

			agentMgr.beforeDispatched = func() {
				emitForegroundIdle(svc, taskID, sessionID)
				if tt.finalOutput {
					svc.handleAgentStreamEvent(t.Context(), &lifecycle.AgentStreamEventPayload{
						TaskID: taskID, SessionID: sessionID, ExecutionID: executionID,
						Data: &lifecycle.AgentStreamEventData{
							Type: "message_streaming", MessageID: "pre-callback-final", Text: "done",
						},
					})
				}
				svc.completeTurnForTaskSession(t.Context(), taskID, sessionID)
				if !tt.keepWork {
					svc.completeBackgroundTaskForExecution(sessionID, "detached-work", executionID)
				}
			}

			if _, err := svc.PromptTask(t.Context(), taskID, sessionID, "follow up", "", false, nil, false); err != nil {
				t.Fatalf("dispatch claimed follow-up: %v", err)
			}
			got := activityValues(eb)
			if tt.keepWork {
				want := []string{string(v1.ForegroundActivityGenerating), string(v1.ForegroundActivityBackground)}
				if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
					t.Fatalf("claimed pre-callback publications = %v, want %v", got, want)
				}
				if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
					t.Fatalf("accepted completed foreground with detached work must be promptable: %v", err)
				}
				return
			}
			if len(got) != 1 || got[0] != string(v1.ForegroundActivityGenerating) {
				t.Fatalf("no-work completion publications = %v, want generating claim only", got)
			}
			if gotActivity := svc.ForegroundActivity(sessionID); gotActivity != v1.ForegroundActivityGenerating {
				t.Fatalf("no-work completion must settle without background hold, got %q", gotActivity)
			}
		})
	}
}

func TestForegroundActivitySignal_SynchronousDispatchFailureReleasesCycleClaim(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptErr:              errors.New("provider rejected prompt before acceptance"),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID      = "task-dispatch-failure-cycle"
		sessionID   = "session-dispatch-failure-cycle"
		executionID = "execution-dispatch-failure-cycle"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)
	svc.registerBackgroundTask(sessionID, "background-survives-failure")
	emitForegroundIdle(svc, taskID, sessionID)

	if _, err := svc.PromptTask(
		t.Context(), taskID, sessionID, "this dispatch fails", "", false, nil, false,
	); err == nil {
		t.Fatal("expected synchronous provider dispatch failure")
	}
	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("failed dispatch must release its foreground claim, got %q", got)
	}
	if retryClaim := svc.claimForegroundTurn(sessionID); retryClaim == nil {
		t.Fatal("failed dispatch rollback must leave the detached-work turn retryable")
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
