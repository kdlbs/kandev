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

// TestPromptTask_BackgroundWorkAcceptsInput is the falsifiable acceptance proof
// for ADR-0035, driven through
// the REAL operator entrypoint (PromptTask) rather than checkSessionPromptable
// in isolation — it reproduces the operator-lockout→fixed transition end to end
// for both of Claude's background-work wire shapes.
//
// The symptom: an operator whose agent launched background work (a Monitor
// watch or a run_in_background shell) is locked out — PromptTask rejects the
// next message with ErrAgentPromptInProgress because the session state still
// reads RUNNING, and the only recovery is a service restart. Both shapes only
// become recognizable on a tool_call_update (the Monitor registration banner
// seeds its Generic view; the run_in_background flag and command are streamed
// after the initial empty tool_call), so this exercises the full producer path:
// a live stream update → background registration → the prompt gate opening.
//
// Why #1600 does not already cover this: the upstream idle-turn completion
// (#1600) only synthesizes a turn-complete after async content has been idle
// for a ~5s debounce, and a chatty Monitor re-extends that debounce on every
// event burst — so the synthetic completion never fires while the watch is
// active. The monitor case below drives repeated event bursts and asserts the
// session is STILL RUNNING (the #1600 window is open, not closed) before it
// sends the mid-burst prompt: without the fine-grained gate, that prompt is
// rejected even though #1600 is armed. See the RED/GREEN note in the batch plan.
//
// Deterministic backend integration harness chosen over Playwright by design:
// surfacing the fine-grained signal to the web composer is a documented
// follow-up (Batch 2 — the composer still derives "busy" from state ===
// RUNNING), so a UI test cannot isolate this fix without a timing-flaky
// assertion. This harness drives PromptTask against a live-agent mock and
// asserts the message is forwarded to the agent, which is exactly the
// acceptance the operator sees.
func TestPromptTask_BackgroundWorkAcceptsInput(t *testing.T) {
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
				return streams.NewShellExec("npm run dev", "", "", 0, true)
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

			// The session is still RUNNING: #1600's synthetic turn-complete has not
			// fired (the bursts kept its debounce alive), so the durable state alone
			// would keep the operator locked out. This is the window the
			// fine-grained gate must open.
			refreshed, err := repo.GetTaskSession(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("reload session: %v", err)
			}
			if refreshed.State != models.TaskSessionStateRunning {
				t.Fatalf("expected session to remain RUNNING while background work is outstanding, got %s", refreshed.State)
			}

			// Fixed: the same message now goes through — the session accepts input
			// while only background work is outstanding, and it is forwarded to the
			// live agent instead of being dropped as "busy".
			if _, err := svc.PromptTask(context.Background(), taskID, sessionID, "are you still working?", "", false, nil, false); err != nil {
				t.Fatalf("session with only background work outstanding must accept input, got: %v", err)
			}
			if len(agentMgr.capturedPrompts) != 1 {
				t.Fatalf("accepted prompt must be forwarded to the agent, captured=%d", len(agentMgr.capturedPrompts))
			}
		})
	}
}

// TestPromptTask_NonClaudeFramesStayBusy is the explicit non-Claude regression
// assertion (ADR-0035 "byte-for-byte unchanged" default):
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
