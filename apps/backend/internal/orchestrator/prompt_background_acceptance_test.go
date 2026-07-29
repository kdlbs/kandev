package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// TestPromptTask_BackgroundWorkStaysBusy drives the real operator entrypoint
// for both recognized background-work wire shapes. Accounting remains live,
// but neither a background shell nor a chatty Monitor can bypass the coarse
// RUNNING gate.
func TestPromptTask_BackgroundWorkStaysBusy(t *testing.T) {
	cases := []struct {
		name       string
		toolCallID string
		// burst is how many non-terminal tool_call_updates the agent streams
		// before the operator prompts. >1 models a chatty Monitor whose bursts
		// keep re-extending #1600's debounce.
		burst      int
		normalized func() *streams.NormalizedPayload
	}{
		{
			name:       "run_in_background shell",
			toolCallID: "bash-1",
			burst:      1,
			normalized: func() *streams.NormalizedPayload {
				return attestedBackgroundShellPayload("npm run dev")
			},
		},
		{
			name:       "chatty monitor watch",
			toolCallID: "monitor-1",
			burst:      4,
			normalized: func() *streams.NormalizedPayload {
				return monitorGenericPayload(false)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.messageCreator = &mockMessageCreator{}

			const (
				taskID    = "task1"
				sessionID = "session1"
			)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
			session, err := repo.GetTaskSession(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			session.AgentExecutionID = "exec-1"
			seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
			if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
				t.Fatalf("update session: %v", err)
			}

			// Lockout: a RUNNING session whose foreground turn is generating rejects
			// the next message — the exact symptom the operator hits.
			if _, err := svc.PromptTask(context.Background(), taskID, sessionID, "hey", "", false, nil, false); !errors.Is(err, ErrAgentPromptInProgress) {
				t.Fatalf("precondition: RUNNING session must reject input with ErrAgentPromptInProgress, got: %v", err)
			}
			if len(agentMgr.capturedPrompts) != 0 {
				t.Fatalf("precondition: rejected prompt must not reach the agent, captured=%d", len(agentMgr.capturedPrompts))
			}

			// The agent's background work becomes recognizable on non-terminal
			// tool_call_updates streamed from the live agent. Repeated bursts model
			// a chatty Monitor re-extending #1600's debounce.
			for i := 0; i < tc.burst; i++ {
				svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
					TaskID:      taskID,
					SessionID:   sessionID,
					ExecutionID: "exec-1",
					Data: &lifecycle.AgentStreamEventData{
						Type:       "tool_update",
						ToolCallID: tc.toolCallID,
						ToolStatus: "in_progress",
						Normalized: tc.normalized(),
					},
				})
			}
			emitForegroundIdle(svc, taskID, sessionID)

			// The session is still RUNNING, so durable state remains the admission
			// authority regardless of the private background estimate.
			refreshed, err := repo.GetTaskSession(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("reload session: %v", err)
			}
			if refreshed.State != models.TaskSessionStateRunning {
				t.Fatalf("expected session to remain RUNNING while background work is outstanding, got %s", refreshed.State)
			}

			if _, err := svc.PromptTask(
				context.Background(), taskID, sessionID, "are you still working?", "", false, nil, false,
			); !errors.Is(err, ErrAgentPromptInProgress) {
				t.Fatalf("RUNNING session with background work must reject input, got: %v", err)
			}
			if len(agentMgr.capturedPrompts) != 0 {
				t.Fatalf("rejected prompt reached the agent, captured=%d", len(agentMgr.capturedPrompts))
			}
		})
	}
}

func TestPromptTask_ClaudeExperimentAdmitsRecognizedBackgroundWork(t *testing.T) {
	cases := []struct {
		name       string
		toolCallID string
		normalized func() *streams.NormalizedPayload
	}{
		{
			name:       "async subagent",
			toolCallID: "agent-1",
			normalized: func() *streams.NormalizedPayload {
				payload := attestedSubagentPayload("background work", "do it", "general-purpose")
				payload.SubagentTask().IsAsync = true
				payload.SetBackgroundWorkIdentity(
					streams.BackgroundWorkKindSubagent,
					"child-1",
					true,
					false,
				)
				return payload
			},
		},
		{
			name:       "run_in_background shell",
			toolCallID: "bash-1",
			normalized: func() *streams.NormalizedPayload {
				return attestedBackgroundShellPayload("npm run dev")
			},
		},
		{
			name:       "monitor",
			toolCallID: "monitor-1",
			normalized: func() *streams.NormalizedPayload {
				return monitorGenericPayload(false)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.messageCreator = &mockMessageCreator{}
			enableClaudeBackgroundPromptHandoffForTest(t, svc)

			const (
				taskID    = "task1"
				sessionID = "session1"
			)
			seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
			setSessionAgentNameForTest(t, svc, sessionID, "claude-acp")
			session, err := repo.GetTaskSession(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			session.AgentExecutionID = "exec-1"
			seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
			if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
				t.Fatalf("update session: %v", err)
			}

			svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
				TaskID:      taskID,
				SessionID:   sessionID,
				ExecutionID: "exec-1",
				Data: &lifecycle.AgentStreamEventData{
					Type:       "tool_update",
					ToolCallID: tc.toolCallID,
					ToolStatus: "in_progress",
					Normalized: tc.normalized(),
				},
			})
			emitForegroundIdle(svc, taskID, sessionID)

			if _, err := svc.PromptTask(
				context.Background(), taskID, sessionID, "are you still working?", "", false, nil, false,
			); err != nil {
				t.Fatalf("enabled Claude background handoff rejected input: %v", err)
			}
			if len(agentMgr.capturedPrompts) != 1 {
				t.Fatalf("accepted prompt count = %d, want 1", len(agentMgr.capturedPrompts))
			}
		})
	}
}

// TestPromptTask_NonClaudeFramesStayBusy is the explicit non-Claude regression
// assertion (ADR-0049 "byte-for-byte unchanged" default):
// a codex/opencode-shaped in-flight tool call is not recognized as background
// work, so a RUNNING session driving one must keep rejecting operator input
// exactly as it did before the fine-grained gate existed.
func TestPromptTask_NonClaudeFramesStayBusy(t *testing.T) {
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-codex"
	)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, sessionID, taskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	// A non-Claude agent streams ordinary foreground tool calls (an edit, a
	// foreground shell) — none of which normalize to a recognized background
	// shape. These must never open the gate.
	frames := []*streams.NormalizedPayload{
		streams.NewShellExec("go build ./...", "", "", 0, false),
		streams.NewReadFile("/repo/main.go", 0, 0),
		streams.NewGeneric("codex_apply_patch", map[string]any{"raw_input": map[string]any{"patch": "..."}}),
	}
	for _, n := range frames {
		svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:      taskID,
			SessionID:   sessionID,
			ExecutionID: "exec-1",
			Data: &lifecycle.AgentStreamEventData{
				Type:       "tool_update",
				ToolCallID: "codex-tool",
				ToolStatus: "in_progress",
				Normalized: n,
			},
		})
	}

	// The gate is unchanged: a RUNNING session with only unrecognized foreground
	// work outstanding still rejects input, and nothing reaches the agent.
	if _, err := svc.PromptTask(context.Background(), taskID, sessionID, "hey", "", false, nil, false); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("unrecognized (non-Claude) work must keep the RUNNING session busy, got: %v", err)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("rejected prompt must not reach the agent, captured=%d", len(agentMgr.capturedPrompts))
	}
}
