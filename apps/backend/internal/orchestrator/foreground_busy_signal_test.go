package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestCheckSessionPromptable_BackgroundTaskAcceptsInput is the red
// characterization test for ADR-0035: a session whose
// foreground turn is idle while a spawned background task is still running must
// accept a new message, not be rejected as "already running".
//
// Before the fix, checkSessionPromptable rejected ANY RUNNING session with
// ErrAgentPromptInProgress, so an operator whose agent kicked off background
// work was locked out of the conversation. After the fix, a RUNNING session
// with an outstanding background task (and no active foreground generation) is
// promptable, while a RUNNING session that is genuinely generating in the
// foreground is still gated.
func TestCheckSessionPromptable_BackgroundTaskAcceptsInput(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-bg"

	// Baseline: a genuinely-generating foreground turn is still gated.
	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("foreground-generating RUNNING session must be gated with ErrAgentPromptInProgress, got: %v", err)
	}

	// The agent spawns a background task and goes idle in the foreground.
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")

	// The session must now accept a new message even though its state is RUNNING.
	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("RUNNING session waiting only on background work must be promptable, got: %v", err)
	}

	// Once the background task finishes, the (still open) turn is once again a
	// genuine foreground turn and input is gated.
	svc.completeBackgroundTask(sessionID, "tool-subagent-1")
	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("after background work completes the RUNNING session must gate input again, got: %v", err)
	}
}

// TestTurnActivity_ForegroundBackgroundTransitions locks in the state machine
// behind isForegroundTurnGenerating.
// TestForegroundActivity_ExportedValue covers the seam the page-load / list
// serialization layer depends on (ADR-0035): the exported
// ForegroundActivity mirror of the in-memory tracker. An untracked session — which
// includes every session after a backend restart, since a restart ends the turn —
// must report the safe "generating" default so a stale "you may type" can never be
// serialized.
func TestForegroundActivity_ExportedValue(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const s = "session-fa"

	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityGenerating {
		t.Fatalf("untracked session must default to generating, got %q", got)
	}

	svc.registerBackgroundTask(s, "t1")
	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityBackground {
		t.Fatalf("after registering background work, got %q, want background", got)
	}

	svc.completeBackgroundTask(s, "t1")
	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityGenerating {
		t.Fatalf("after background work finishes, got %q, want generating", got)
	}

	// clearTurnActivity models a turn-close / restart-adjacent reset back to safe.
	svc.registerBackgroundTask(s, "t2")
	svc.clearTurnActivity(s)
	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityGenerating {
		t.Fatalf("after clearTurnActivity, got %q, want generating", got)
	}
}

func TestTurnActivity_ForegroundBackgroundTransitions(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const s = "session-x"

	// Absent state defaults to "foreground generating" — preserves the historical
	// reject-while-RUNNING contract for sessions with no background work.
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("untracked session must default to foreground-generating")
	}

	// Register a background task: foreground has yielded.
	svc.registerBackgroundTask(s, "t1")
	if svc.isForegroundTurnGenerating(s) {
		t.Fatal("after registering a background task the foreground must be idle")
	}
	svc.completeBackgroundTask(s, "t1")

	// A fresh foreground stream chunk means the agent is generating again.
	svc.markForegroundGenerating(s)
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("streamed foreground output must mark the turn as generating again")
	}

	// Two concurrent background tasks: the foreground is idle until BOTH finish.
	svc.registerBackgroundTask(s, "t2")
	svc.registerBackgroundTask(s, "t3")
	if svc.isForegroundTurnGenerating(s) {
		t.Fatal("with outstanding background tasks the foreground must be idle")
	}
	svc.completeBackgroundTask(s, "t2")
	if svc.isForegroundTurnGenerating(s) {
		t.Fatal("one of two background tasks finishing must not resume foreground")
	}
	svc.completeBackgroundTask(s, "t3")
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("with all background tasks finished the foreground default resumes")
	}

	// Clearing turn activity resets to the default.
	svc.registerBackgroundTask(s, "t4")
	svc.clearTurnActivity(s)
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("clearTurnActivity must reset to the foreground-generating default")
	}
}

// TestCompleteTurnClearsBackgroundActivity confirms that closing a turn drops the
// background-activity tracking so the next turn starts clean.
func TestCompleteTurnClearsBackgroundActivity(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const s = "session-turn"
	svc.registerBackgroundTask(s, "t1")
	if svc.isForegroundTurnGenerating(s) {
		t.Fatal("precondition: session should be waiting on background work")
	}

	// completeTurnForSession must clear turn activity (turnService is nil in this
	// test service, which exercises the early-return path).
	svc.completeTurnForSession(t.Context(), s)
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("completeTurnForSession must clear background activity")
	}
}

// TestForegroundBusySignal_WiredThroughStreamEvents drives the real agent
// stream-event dispatch to prove the WS1 producer → WS2 gate wiring end to end:
// a subagent tool_call arriving on the stream flips the promptable gate from
// "reject (agent running)" to "accept (only background work outstanding)".
func TestForegroundBusySignal_WiredThroughStreamEvents(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const (
		taskID    = "task1"
		sessionID = "session-stream"
	)

	// Before any background work, a RUNNING session gates input.
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("precondition: RUNNING session should gate input, got: %v", err)
	}

	// A top-level subagent Task tool_call arrives on the stream — the agent has
	// spawned background work and yielded the foreground.
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

	// The gate now accepts a new message even though the session state is RUNNING.
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("after a background subagent tool_call the session must be promptable, got: %v", err)
	}

	// A child tool_call from inside the subagent (ParentToolCallID set) is the
	// subagent's own work, not a new background task, and must not change the
	// signal.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:             agentEventToolCall,
			ToolCallID:       "child-1",
			ParentToolCallID: "subagent-1",
			ToolStatus:       "running",
			Normalized:       streams.NewShellExec("ls", "", "", 0, false),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("a subagent-internal child tool_call must not re-gate input, got: %v", err)
	}
}

// TestForegroundBusySignal_TerminalToolUpdateReclosesGate proves the completion
// half of the WS1 -> WS2 wiring: once a background subagent tool_call has
// opened the promptable gate, a TERMINAL tool_update for that same tool-call ID
// dispatched through the real stream handler closes the gate again — the
// background task is done, so an open turn is once again a genuine foreground
// turn.
func TestForegroundBusySignal_TerminalToolUpdateReclosesGate(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-stream"
	)

	// A top-level subagent tool_call opens the gate.
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
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("precondition: background subagent tool_call should open the gate, got: %v", err)
	}

	// The subagent's own terminal tool_update arrives on the stream.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "subagent-1",
			ToolStatus: agentEventComplete,
			Normalized: streams.NewSubagentTask("explore", "find files", "general-purpose"),
		},
	})

	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a terminal tool_update for the outstanding background task must re-close the gate, got: %v", err)
	}
}

// TestForegroundBusySignal_TerminalToolUpdateReclosesGateByIDNotKind proves the
// completion clears by tool-call ID membership, not by re-classifying the
// terminal payload: a terminal tool_update whose Normalized payload is a plain
// (non-background) tool must still re-close the gate when its ToolCallID
// matches the registered background task. An adapter that rebuilds Normalized
// per update (or drops the Background flag on the terminal frame) would
// otherwise never match on kind, leaving the session permanently "not
// generating" for the rest of the turn.
func TestForegroundBusySignal_TerminalToolUpdateReclosesGateByIDNotKind(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-stream"
	)

	// A top-level subagent tool_call opens the gate.
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
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("precondition: background subagent tool_call should open the gate, got: %v", err)
	}

	// The terminal update for the SAME tool-call ID carries a plain, non-background
	// Normalized payload (e.g. the adapter rebuilt it without the Background flag).
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "subagent-1",
			ToolStatus: agentEventComplete,
			Normalized: streams.NewGeneric("SomeTool", nil),
		},
	})

	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a terminal tool_update matching the registered ID must re-close the gate regardless of its Normalized kind, got: %v", err)
	}
}

// TestForegroundBusySignal_UnregisteredTerminalToolUpdateLeavesGateOpen proves
// completeBackgroundTask is a no-op for IDs that were never registered: a
// terminal tool_update for an unrelated tool-call ID must not spuriously clear
// the still-outstanding background task.
func TestForegroundBusySignal_UnregisteredTerminalToolUpdateLeavesGateOpen(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-stream"
	)

	// A top-level subagent tool_call opens the gate.
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
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("precondition: background subagent tool_call should open the gate, got: %v", err)
	}

	// A terminal tool_update for an ID that was never registered as a background
	// task arrives — must not clear the still-outstanding "subagent-1" task.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "unregistered-tool",
			ToolStatus: agentEventComplete,
			Normalized: streams.NewGeneric("SomeTool", nil),
		},
	})

	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("a terminal update for an unregistered tool-call ID must not re-gate input while background work is still outstanding, got: %v", err)
	}
}

// monitorGenericPayload builds the Generic payload the ACP adapter emits for a
// Claude Monitor: kind=generic with the structured `{monitor:{...}}` view tucked
// into Output. `ended` toggles whether the watch is still live.
func monitorGenericPayload(ended bool) *streams.NormalizedPayload {
	p := streams.NewGeneric("Monitor", map[string]any{})
	p.Generic().Output = map[string]any{
		"monitor": map[string]any{
			"kind":    streams.MonitorSubkind,
			"ended":   ended,
			"task_id": "task-1",
			"command": "gh pr checks --watch",
		},
	}
	return p
}

// TestNormalizedIsBackgroundTask pins the predicate that classifies which tool
// calls represent spawned background work the foreground turn waits on.
func TestNormalizedIsBackgroundTask(t *testing.T) {
	cases := []struct {
		name string
		n    *streams.NormalizedPayload
		want bool
	}{
		{"nil", nil, false},
		{"subagent task", streams.NewSubagentTask("explore", "find files", "general-purpose"), true},
		{"background shell", streams.NewShellExec("sleep 30", "", "", 0, true), true},
		{"foreground shell", streams.NewShellExec("ls", "", "", 0, false), false},
		{"active monitor", monitorGenericPayload(false), true},
		{"ended monitor", monitorGenericPayload(true), false},
		{"read file", streams.NewReadFile("/tmp/x", 0, 0), false},
		{"generic tool", streams.NewGeneric("SomeTool", nil), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizedIsBackgroundTask(tc.n); got != tc.want {
				t.Errorf("normalizedIsBackgroundTask(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestForegroundBusySignal_BackgroundShellViaUpdate proves Gap 1 end-to-end at
// the orchestrator boundary: a run_in_background Bash arrives as an initial
// tool_call with an empty (foreground-looking) payload, then a non-terminal
// tool_call_update whose Normalized ShellExec carries Background:true. The
// update must open checkSessionPromptable for a RUNNING session; the terminal
// update re-closes it. Before the wiring fix the update path never registered
// background work, so the gate stayed shut for the whole watch.
func TestForegroundBusySignal_BackgroundShellViaUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-bgshell"
	)

	// Initial tool_call: empty, foreground-looking. The gate stays shut.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "bash-1",
			ToolStatus: "pending",
			Normalized: streams.NewShellExec("", "", "", 0, false),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("precondition: empty initial tool_call must not open the gate, got: %v", err)
	}

	// Non-terminal tool_call_update carries the command + run_in_background flag.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "bash-1",
			ToolStatus: "in_progress",
			Normalized: streams.NewShellExec("npm run dev", "", "", 0, true),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("a run_in_background shell tool_update must open the gate, got: %v", err)
	}

	// Terminal update for the same tool-call ID re-closes the gate.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "bash-1",
			ToolStatus: agentEventComplete,
			Normalized: streams.NewShellExec("npm run dev", "", "", 0, true),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("terminal shell tool_update must re-close the gate, got: %v", err)
	}
}

// TestForegroundBusySignal_MonitorViaUpdate proves Gap 2 end-to-end at the
// orchestrator boundary: a Claude Monitor normalizes to a Generic payload whose
// structured view is only seeded on its registration tool_call_update. That
// non-terminal update must open checkSessionPromptable for a RUNNING session so
// the operator isn't locked out while the Monitor watches. The terminal update
// the adapter emits from sweepMonitorsOnPromptEnd (status "complete") must clear
// the background hold and re-close the gate.
func TestForegroundBusySignal_MonitorViaUpdate(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-monitor"
	)

	// Initial Monitor tool_call: Generic payload, view not seeded yet — the
	// adapter can't recognize the Monitor until the registration banner. The gate
	// stays shut.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "monitor-1",
			ToolStatus: "pending",
			Normalized: streams.NewGeneric("Monitor", map[string]any{}),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("precondition: pre-registration Monitor tool_call must not open the gate, got: %v", err)
	}

	// Registration tool_call_update: the adapter has seeded the `{monitor:...}`
	// view and flipped status to in_progress. The gate opens.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "monitor-1",
			ToolStatus: "in_progress",
			Normalized: monitorGenericPayload(false),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("an active Monitor registration tool_update must open the gate, got: %v", err)
	}

	// sweepMonitorsOnPromptEnd emits a terminal "complete" tool_update with the
	// view marked ended. It must clear the background hold and re-close the gate.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "monitor-1",
			ToolStatus: agentEventComplete,
			Normalized: monitorGenericPayload(true),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a swept/ended Monitor terminal tool_update must re-close the gate, got: %v", err)
	}
}

// TestForegroundBusySignal_UpdateDoesNotReYieldAfterForeground guards the
// register-only-on-first-recognition rule: once a background task is
// outstanding and the foreground has streamed output (marking the turn
// generating again), a later non-terminal tool_update for that SAME background
// task must NOT re-yield the turn — otherwise a background progress frame would
// spuriously re-open the gate while the foreground is generating.
func TestForegroundBusySignal_UpdateDoesNotReYieldAfterForeground(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const (
		taskID    = "task1"
		sessionID = "session-reyield"
	)

	bgUpdate := func() {
		svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
			TaskID:    taskID,
			SessionID: sessionID,
			Data: &lifecycle.AgentStreamEventData{
				Type:       "tool_update",
				ToolCallID: "bash-1",
				ToolStatus: "in_progress",
				Normalized: streams.NewShellExec("npm run dev", "", "", 0, true),
			},
		})
	}

	// A background run_in_background shell is recognized on its first update.
	bgUpdate()
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); err != nil {
		t.Fatalf("precondition: background shell update should open the gate, got: %v", err)
	}

	// The foreground resumes generating (streamed message chunk).
	svc.markForegroundGenerating(sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("foreground generating again must re-gate input, got: %v", err)
	}

	// A later non-terminal progress update for the same background task must not
	// re-yield the turn.
	bgUpdate()
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a later background progress update must not re-open the gate after foreground resumed, got: %v", err)
	}
}
