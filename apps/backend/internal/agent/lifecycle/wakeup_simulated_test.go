package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestWakeup_SecondTurnAgentReadyIsSuppressed reproduces the bug deterministically
// without needing the bridge or auth.
//
// Scenario: an execution receives events for two consecutive turns. The first
// turn is user-initiated (status flips Running → Ready on complete, agent.ready
// fires). The second turn is wakeup-initiated — kandev's wakeup scheduler calls
// adapter.Prompt directly, which does NOT flip the execution back to Running
// (that's done by SessionManager.SendPrompt, which fireWakeup bypasses).
//
// When the second `complete` event arrives, the manager calls MarkReady. But
// MarkReady at manager_interaction.go:896 has:
//
//	if execution.Status == v1.AgentStatusReady {
//	    return nil
//	}
//
// Since the execution is still Ready (never went back to Running), this early-
// returns, no agent.ready is published, and the orchestrator never sees the
// wakeup turn end → completeTurnForSession never fires → workflow on_turn_complete
// never evaluates.
//
// The wakeup-turn's message_streaming events DO still reach the bus (those
// don't gate on execution status), so the persisted chat history is correct.
// But workflow state and queued-message dispatch are broken.
func TestWakeup_SecondTurnAgentReadyIsSuppressed(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	_ = mgr.executionStore.Add(execution)

	// === Turn 1: user-initiated prompt ===
	//
	// In production: SessionManager.SendPrompt() runs and calls
	//   sm.executionStore.UpdateStatus(execution.ID, v1.AgentStatusRunning)
	// We mirror that here.
	mgr.executionStore.UpdateStatus(execution.ID, v1.AgentStatusRunning)

	// Assistant text + tool call + complete (the agent calls ScheduleWakeup).
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk", Text: "Scheduled.\n",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete"})

	// === Turn 2: wakeup-initiated prompt ===
	//
	// In production: the wakeupScheduler timer fires → adapter.fireWakeup() →
	// adapter.Prompt(ctx, prompt, nil). The lifecycle layer is NOT involved
	// in initiating this prompt, so executionStore.UpdateStatus(Running) is
	// NEVER called. We mirror that by leaving the execution status as-is.

	// Wakeup-turn text + complete arrive via the adapter's update stream.
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "message_chunk", Text: "WAKEUP_FIRED\n",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete"})

	// === Inspect what the manager published ===
	readyCount := 0
	streamingTexts := []string{}
	streamCompleteCount := 0
	for _, te := range eventBus.PublishedEvents {
		if te.Event == nil {
			continue
		}
		if te.Event.Type == events.AgentReady {
			readyCount++
		}
		if payload, ok := te.Event.Data.(AgentStreamEventPayload); ok && payload.Data != nil {
			switch payload.Data.Type {
			case "message_streaming":
				streamingTexts = append(streamingTexts, payload.Data.Text)
			case "complete":
				streamCompleteCount++
			}
		}
	}

	t.Logf("agent.ready events published: %d (expect 2)", readyCount)
	t.Logf("message_streaming events: %v", streamingTexts)
	t.Logf("AgentStreamEventPayload type=complete events: %d (expect 2)", streamCompleteCount)
	t.Logf("execution.Status at end: %s", execution.Status)

	// The agent-stream-level events (message_streaming, complete) are NOT
	// gated on execution status — they should reach the bus for BOTH turns.
	if streamCompleteCount != 2 {
		t.Errorf("expected 2 stream-level complete events, got %d", streamCompleteCount)
	}
	if len(streamingTexts) != 2 {
		t.Errorf("expected 2 message_streaming events (one per turn), got %d: %v",
			len(streamingTexts), streamingTexts)
	}

	// The bug: agent.ready is suppressed on the second turn because the
	// execution status is already Ready when MarkReady runs.
	if readyCount < 2 {
		t.Errorf(
			"BUG CONFIRMED: only %d agent.ready event(s) published, expected 2. "+
				"MarkReady at manager_interaction.go:896 suppresses the wakeup turn's "+
				"AgentReady because fireWakeup bypasses SessionManager.SendPrompt and "+
				"never flips the execution back to Running. The orchestrator therefore "+
				"never receives agent.ready for the wakeup turn → "+
				"completeTurnForSession is not called → workflow on_turn_complete is "+
				"not evaluated → queued messages are not dispatched.",
			readyCount,
		)
	}
}
