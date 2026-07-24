package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

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
