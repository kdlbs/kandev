package acp

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// makeNotification is a helper that wraps a SessionUpdate in a SessionNotification.
func makeNotification(sessionID string, update acp.SessionUpdate) acp.SessionNotification {
	return acp.SessionNotification{
		SessionId: acp.SessionId(sessionID),
		Update:    update,
	}
}

func TestLoadSuppression_MessageEventsAreSuppressed(t *testing.T) {
	a := newTestAdapter()
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	notifications := []acp.SessionNotification{
		makeNotification("s1", acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{},
		}),
		makeNotification("s1", acp.SessionUpdate{
			UserMessageChunk: &acp.SessionUpdateUserMessageChunk{},
		}),
		makeNotification("s1", acp.SessionUpdate{
			AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{},
		}),
		makeNotification("s1", acp.SessionUpdate{
			ToolCall: &acp.SessionUpdateToolCall{},
		}),
		makeNotification("s1", acp.SessionUpdate{
			ToolCallUpdate: &acp.SessionToolCallUpdate{},
		}),
	}

	for _, n := range notifications {
		a.handleACPUpdate(n)
	}

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events during load, got %d: %+v", len(events), events)
	}
}

func TestLoadSuppression_AvailableCommandsPassThroughDuringLoad(t *testing.T) {
	a := newTestAdapter()
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	// AvailableCommandsUpdate should NOT be suppressed during load —
	// it may arrive after replay as a "ready" signal from the agent.
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
			AvailableCommands: []acp.AvailableCommand{
				{Name: "commit", Description: "Commit changes"},
				{Name: "todo", Description: "Manage todos"},
			},
		},
	}))

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event (AvailableCommands passes through during load), got %d", len(events))
	}
	if events[0].Type != streams.EventTypeAvailableCommands {
		t.Errorf("expected event type %q, got %q", streams.EventTypeAvailableCommands, events[0].Type)
	}
	if len(events[0].AvailableCommands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(events[0].AvailableCommands))
	}
}

func TestLoadSuppression_PlanSuppressedAndCaptured(t *testing.T) {
	a := newTestAdapter()
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	// Send two Plan notifications; only the last should be captured.
	first := &acp.SessionUpdatePlan{
		Entries: []acp.PlanEntry{
			{Content: "old task", Status: "in_progress"},
		},
	}
	second := &acp.SessionUpdatePlan{
		Entries: []acp.PlanEntry{
			{Content: "task 1", Status: "completed"},
			{Content: "task 2", Status: "in_progress"},
		},
	}

	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{Plan: first}))
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{Plan: second}))

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events during load, got %d", len(events))
	}

	a.mu.RLock()
	captured := a.loadReplayPlan
	a.mu.RUnlock()

	if captured == nil {
		t.Fatal("expected loadReplayPlan to be captured")
	}
	if len(captured.Entries) != 2 {
		t.Fatalf("expected 2 captured plan entries, got %d", len(captured.Entries))
	}
	if captured.Entries[0].Content != "task 1" {
		t.Errorf("expected last plan update to be captured, got content=%q", captured.Entries[0].Content)
	}
}

func TestLoadSuppression_ModeAndConfigSuppressed(t *testing.T) {
	a := newTestAdapter()
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		CurrentModeUpdate: &acp.SessionCurrentModeUpdate{CurrentModeId: "plan"},
	}))
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{},
	}))

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events during load, got %d: %+v", len(events), events)
	}
}

func TestLoadSuppression_EventsPassThroughWhenNotLoading(t *testing.T) {
	a := newTestAdapter()
	// isLoadingSession defaults to false

	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
			AvailableCommands: []acp.AvailableCommand{
				{Name: "commit", Description: "Commit changes"},
			},
		},
	}))
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		Plan: &acp.SessionUpdatePlan{
			Entries: []acp.PlanEntry{
				{Content: "do something", Status: "in_progress"},
			},
		},
	}))

	events := drainEvents(a)
	if len(events) != 2 {
		t.Fatalf("expected 2 events when not loading, got %d", len(events))
	}
	if events[0].Type != streams.EventTypeAvailableCommands {
		t.Errorf("expected first event type %q, got %q", streams.EventTypeAvailableCommands, events[0].Type)
	}
	if events[1].Type != streams.EventTypePlan {
		t.Errorf("expected second event type %q, got %q", streams.EventTypePlan, events[1].Type)
	}
}

func TestLoadSuppression_PlanReemittedAfterLoad(t *testing.T) {
	a := newTestAdapter()

	// Simulate a load: set the flag and send replay notifications.
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	// AvailableCommandsUpdate passes through during load.
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
			AvailableCommands: []acp.AvailableCommand{
				{Name: "todo-write", Description: "Write todos"},
			},
		},
	}))
	// Plan is suppressed and captured.
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		Plan: &acp.SessionUpdatePlan{
			Entries: []acp.PlanEntry{
				{Content: "implement feature", Status: "in_progress", Priority: "high"},
			},
		},
	}))

	// Drain: only AvailableCommands should have passed through.
	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event during load (AvailableCommands), got %d", len(events))
	}
	if events[0].Type != streams.EventTypeAvailableCommands {
		t.Errorf("expected event type %q, got %q", streams.EventTypeAvailableCommands, events[0].Type)
	}

	// Simulate what LoadSession() does after the RPC returns:
	// read and clear the captured plan, clear the loading flag, then re-emit.
	a.mu.Lock()
	replayPlan := a.loadReplayPlan
	a.loadReplayPlan = nil
	a.isLoadingSession = false
	a.mu.Unlock()

	if replayPlan != nil {
		entries := make([]PlanEntry, len(replayPlan.Entries))
		for i, e := range replayPlan.Entries {
			entries[i] = PlanEntry{
				Description: e.Content,
				Status:      string(e.Status),
				Priority:    string(e.Priority),
			}
		}
		a.sendUpdate(AgentEvent{
			Type:        streams.EventTypePlan,
			SessionID:   "s1",
			PlanEntries: entries,
		})
	}

	// Now we should see the re-emitted plan.
	events = drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 re-emitted event (Plan), got %d", len(events))
	}
	if events[0].Type != streams.EventTypePlan {
		t.Errorf("expected event type %q, got %q", streams.EventTypePlan, events[0].Type)
	}
	if len(events[0].PlanEntries) != 1 || events[0].PlanEntries[0].Description != "implement feature" {
		t.Errorf("unexpected plan entries: %+v", events[0].PlanEntries)
	}
	if events[0].PlanEntries[0].Status != "in_progress" {
		t.Errorf("expected plan status 'in_progress', got %q", events[0].PlanEntries[0].Status)
	}

	// Verify loading flag was cleared and captured state is empty.
	a.mu.RLock()
	if a.isLoadingSession {
		t.Error("isLoadingSession should be false after load completes")
	}
	if a.loadReplayPlan != nil {
		t.Error("loadReplayPlan should be nil after re-emit")
	}
	a.mu.RUnlock()
}

func TestLoadSuppression_PostReplayEventsPassThrough(t *testing.T) {
	a := newTestAdapter()

	// Simulate load and then clearing the flag (as LoadSession does).
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	// Replay a message — should be suppressed.
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{},
	}))

	events := drainEvents(a)
	if len(events) != 0 {
		t.Fatalf("expected 0 events during load, got %d", len(events))
	}

	// Clear the flag (simulating LoadSession completion).
	a.mu.Lock()
	a.isLoadingSession = false
	a.mu.Unlock()

	// Post-replay AvailableCommandsUpdate should pass through.
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
			AvailableCommands: []acp.AvailableCommand{
				{Name: "commit", Description: "Commit changes"},
			},
		},
	}))

	events = drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after load cleared, got %d", len(events))
	}
	if events[0].Type != streams.EventTypeAvailableCommands {
		t.Errorf("expected event type %q, got %q", streams.EventTypeAvailableCommands, events[0].Type)
	}
}
