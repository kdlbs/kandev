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

func enableClaudeBackgroundPromptHandoffForTest(t *testing.T, svc *Service) {
	t.Helper()
	svc.config.ClaudeBackgroundPromptHandoff = true
}

func setSessionAgentNameForTest(
	t *testing.T,
	svc *Service,
	sessionID, agentName string,
) {
	t.Helper()
	session, err := svc.repo.GetTaskSession(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession(%q): %v", sessionID, err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{
		"agent_id":   "persisted-agent-record-id",
		"agent_name": agentName,
	}
	if err := svc.repo.UpdateTaskSession(t.Context(), session); err != nil {
		t.Fatalf("UpdateTaskSession(%q): %v", sessionID, err)
	}
}

func emitForegroundIdle(svc *Service, taskID, sessionID string) {
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID: taskID, SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{Type: streams.EventTypeForegroundIdle},
	})
}

// TestCheckSessionPromptable_BackgroundTaskRemainsBusy pins the conservative
// admission contract from ADR-2026-07-28: provider background-work inference
// may remain useful for accounting, but it cannot relax a RUNNING session's
// prompt gate.
func TestCheckSessionPromptable_BackgroundTaskRemainsBusy(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-bg"

	// Baseline: a genuinely-generating foreground turn is still gated.
	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("foreground-generating RUNNING session must be gated with ErrAgentPromptInProgress, got: %v", err)
	}

	// The agent spawns a background task and goes idle in the foreground.
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	if got := svc.foregroundActivityValue(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("precondition: private tracker activity = %q, want background", got)
	}

	// Background accounting must not make the RUNNING session promptable.
	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("RUNNING session waiting on background work must remain gated, got: %v", err)
	}

	// Once the background task finishes, the (still open) turn is once again a
	// genuine foreground turn and input is gated.
	svc.completeBackgroundTask(sessionID, "tool-subagent-1")
	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("after background work completes the RUNNING session must gate input again, got: %v", err)
	}
}

func TestCheckSessionPromptable_ClaudeExperimentAcceptsBackgroundIdle(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	enableClaudeBackgroundPromptHandoffForTest(t, svc)

	const taskID = "task-claude-experiment"
	const sessionID = "session-claude-experiment"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	setSessionAgentNameForTest(t, svc, sessionID, "claude-acp")

	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityBackground {
		t.Fatalf("enabled Claude activity = %q, want background", got)
	}
	if err := svc.checkSessionPromptable(
		taskID,
		sessionID,
		models.TaskSessionStateRunning,
	); err != nil {
		t.Fatalf("enabled Claude background-idle session rejected: %v", err)
	}
}

func TestCheckSessionPromptable_ClaudeExperimentRejectsNonClaude(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	enableClaudeBackgroundPromptHandoffForTest(t, svc)

	const taskID = "task-codex-experiment"
	const sessionID = "session-codex-experiment"
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	setSessionAgentNameForTest(t, svc, sessionID, "codex-acp")

	// Deliberately forge the private tracker state. Provider gating must remain
	// a separate fail-closed boundary even if a future normalizer regression
	// incorrectly attests a non-Claude workload.
	svc.registerBackgroundTask(sessionID, "tool-subagent-1")
	svc.markForegroundIdle(sessionID)

	if got := svc.ForegroundActivity(sessionID); got != v1.ForegroundActivityGenerating {
		t.Fatalf("enabled non-Claude activity = %q, want generating", got)
	}
	if err := svc.checkSessionPromptable(
		taskID,
		sessionID,
		models.TaskSessionStateRunning,
	); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("enabled non-Claude background-idle session must remain gated, got: %v", err)
	}
}

func TestForegroundToolCallClosesBackgroundIdleGate(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const sessionID = "session-foreground-tool"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "read-1",
			ToolStatus: "running",
			Normalized: streams.NewReadFile("/repo/main.go", 0, 0),
		},
	})

	if err := svc.checkSessionPromptable("task1", sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("top-level foreground tool activity must close the prompt gate, got: %v", err)
	}
}

// TestForegroundActivity_ExportedValue proves the public seam stays coarse
// while the dormant tracker retains its fine-grained value.
func TestForegroundActivity_ExportedValue(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const s = "session-fa"

	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityGenerating {
		t.Fatalf("untracked session must default to generating, got %q", got)
	}

	svc.registerBackgroundTask(s, "t1")
	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityGenerating {
		t.Fatalf("registration must not override foreground activity, got %q", got)
	}
	svc.markForegroundIdle(s)
	if got := svc.foregroundActivityValue(s); got != v1.ForegroundActivityBackground {
		t.Fatalf("private activity after foreground idle = %q, want background", got)
	}
	if got := svc.ForegroundActivity(s); got != v1.ForegroundActivityGenerating {
		t.Fatalf("public activity after foreground idle = %q, want generating", got)
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

	// Registration records liveness but cannot override a busy foreground.
	svc.registerBackgroundTask(s, "t1")
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("background registration must not override foreground generation")
	}
	svc.markForegroundIdle(s)
	if svc.isForegroundTurnGenerating(s) {
		t.Fatal("after the foreground-idle boundary the background task must be visible")
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
	svc.markForegroundIdle(s)
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

// TestCompleteTurnPreservesBackgroundActivity confirms that foreground turn
// completion does not erase detached work that outlives it.
func TestCompleteTurnPreservesBackgroundActivity(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const (
		taskID = "task-turn"
		s      = "session-turn"
	)
	seedTaskAndSession(t, repo, taskID, s, models.TaskSessionStateRunning)
	svc.registerBackgroundTask(s, "t1")
	if !svc.isForegroundTurnGenerating(s) {
		t.Fatal("precondition: registration alone must retain foreground precedence")
	}

	// completeTurnForSession must leave detached work background-idle even when
	// turnService is nil (the early-return path).
	svc.completeTurnForSession(t.Context(), s)
	if svc.isForegroundTurnGenerating(s) {
		t.Fatal("completeTurnForSession must preserve background activity")
	}
}

func TestForegroundIdleEventRestoresOutstandingBackground(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-idle-signal"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundGenerating(sessionID)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: sessionID,
		Data:      &lifecycle.AgentStreamEventData{Type: streams.EventTypeForegroundIdle},
	})

	if svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("human-cycle completion must yield to outstanding background work")
	}
}

func TestBackgroundCompleteEventRetiresOneOutstandingTask(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())

	const sessionID = "session-background-complete"
	svc.registerBackgroundTask(sessionID, "background-1")
	svc.markForegroundIdle(sessionID)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: sessionID,
		Data:      &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})

	if !svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("the final background completion must return to the idle/default state")
	}
}

// TestForegroundBusySignal_WiredThroughStreamEvents drives the real agent
// stream-event dispatch to prove background accounting does not relax the
// coarse RUNNING gate.
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

	// A top-level subagent Task tool_call registers work; the provider then marks
	// the foreground cycle idle.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: attestedSubagentPayload("explore", "find files", "general-purpose"),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)

	// The private tracker yields, but the public gate remains closed.
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("background subagent work must not make RUNNING promptable, got: %v", err)
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
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a subagent-internal child tool_call must leave RUNNING gated, got: %v", err)
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

	// A top-level subagent tool_call is tracked without opening the coarse gate.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: attestedSubagentPayload("explore", "find files", "general-purpose"),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("background subagent tool_call must leave RUNNING gated, got: %v", err)
	}

	// The subagent's own terminal tool_update arrives on the stream.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "subagent-1",
			ToolStatus: agentEventComplete,
			Normalized: attestedSubagentPayload("explore", "find files", "general-purpose"),
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

	// A top-level subagent tool_call is tracked without opening the coarse gate.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: attestedSubagentPayload("explore", "find files", "general-purpose"),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("background subagent tool_call must leave RUNNING gated, got: %v", err)
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

	// A top-level subagent tool_call is tracked without opening the coarse gate.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       agentEventToolCall,
			ToolCallID: "subagent-1",
			ToolStatus: "running",
			Normalized: attestedSubagentPayload("explore", "find files", "general-purpose"),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("background subagent tool_call must leave RUNNING gated, got: %v", err)
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

	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("an unrelated terminal update must leave RUNNING gated, got: %v", err)
	}
}

// monitorGenericPayload builds the Generic payload the ACP adapter emits for a
// Claude Monitor: kind=generic with the structured `{monitor:{...}}` view tucked
// into Output. `ended` toggles whether the watch is still live.
// monitorGenericPayload mirrors what the ACP adapter emits for a recognized
// Monitor: the presentation view in Generic.Output (what the frontend card reads)
// AND the adapter's out-of-band attestation (what the background-work classifier
// reads). Monitor arrives with ACP kind "other", hence the generic name.
//
// The attestation is not decoration — IsActiveMonitor classifies on it alone,
// because Generic.Output is agent-supplied and so can't vouch for its own origin.
// A fixture that only set the Output map would be a forgery, and would (correctly)
// no longer register as background work.
func monitorGenericPayload(ended bool) *streams.NormalizedPayload {
	p := streams.NewGeneric("other", map[string]any{})
	p.Generic().Output = map[string]any{
		streams.MonitorViewKey: map[string]any{
			streams.MonitorViewKindKey:    streams.MonitorSubkind,
			streams.MonitorViewEndedKey:   ended,
			streams.MonitorViewTaskIDKey:  "task-1",
			streams.MonitorViewCommandKey: "gh pr checks --watch",
		},
	}
	p.SetMonitorIdentity("task-1", ended)
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
		{"unattested subagent task", streams.NewSubagentTask("explore", "find files", "general-purpose"), false},
		{"unattested background shell", streams.NewShellExec("sleep 30", "", "", 0, true), false},
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
// launch-card update must leave it open until workload completion. Before the
// wiring fix the update path never registered background work, so the gate stayed
// shut for the whole watch.
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
			Normalized: attestedBackgroundShellPayload("npm run dev"),
		},
	})
	emitForegroundIdle(svc, taskID, sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("a background shell update must leave RUNNING gated, got: %v", err)
	}

	// Terminal update closes the launch card, not the detached workload.
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "bash-1",
			ToolStatus: agentEventComplete,
			Normalized: attestedBackgroundShellPayload("npm run dev"),
		},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("terminal shell launch update must leave RUNNING gated, got: %v", err)
	}

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data:      &lifecycle.AgentStreamEventData{Type: streams.EventTypeBackgroundComplete},
	})
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("workload completion must re-close the RUNNING gate, got: %v", err)
	}
}

func TestForegroundBusySignal_AsyncSubagentLaunchCompletionPreservesWorkload(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.messageCreator = &mockMessageCreator{}

	const sessionID = "session-async-subagent"
	payload := attestedSubagentPayload("background", "sleep", "general-purpose")
	payload.SubagentTask().IsAsync = true
	payload.SetBackgroundWorkIdentity(
		streams.BackgroundWorkKindSubagent,
		"test-subagent",
		true,
		false,
	)
	svc.registerBackgroundTask(sessionID, "subagent-1")
	svc.markForegroundIdle(sessionID)

	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "task1",
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:       "tool_update",
			ToolCallID: "subagent-1",
			ToolStatus: agentEventComplete,
			Normalized: payload,
		},
	})

	if svc.isForegroundTurnGenerating(sessionID) {
		t.Fatal("async launch-card completion must preserve subagent workload liveness")
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
	emitForegroundIdle(svc, taskID, sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("an active Monitor update must leave RUNNING gated, got: %v", err)
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
				Normalized: attestedBackgroundShellPayload("npm run dev"),
			},
		})
	}

	// A background run_in_background shell is recognized on its first update.
	bgUpdate()
	emitForegroundIdle(svc, taskID, sessionID)
	if err := svc.checkSessionPromptable(taskID, sessionID, models.TaskSessionStateRunning); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("background shell update must leave RUNNING gated, got: %v", err)
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
