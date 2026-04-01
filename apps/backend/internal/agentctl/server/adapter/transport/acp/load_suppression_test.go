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

func TestLoadSuppression_AvailableCommandsSuppressedAndCaptured(t *testing.T) {
	a := newTestAdapter()
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	// Send two AvailableCommandsUpdate notifications; only the last should be captured.
	first := &acp.SessionAvailableCommandsUpdate{
		AvailableCommands: []acp.AvailableCommand{
			{Name: "old-cmd", Description: "stale command"},
		},
	}
	second := &acp.SessionAvailableCommandsUpdate{
		AvailableCommands: []acp.AvailableCommand{
			{Name: "todo-write", Description: "Write todos"},
			{Name: "commit", Description: "Commit changes"},
		},
	}

	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{AvailableCommandsUpdate: first}))
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{AvailableCommandsUpdate: second}))

	events := drainEvents(a)
	if len(events) != 0 {
		t.Errorf("expected 0 events during load, got %d", len(events))
	}

	a.mu.RLock()
	captured := a.loadReplayAvailableCommands
	a.mu.RUnlock()

	if captured == nil {
		t.Fatal("expected loadReplayAvailableCommands to be captured")
	}
	if len(captured.AvailableCommands) != 2 {
		t.Fatalf("expected 2 captured commands, got %d", len(captured.AvailableCommands))
	}
	if captured.AvailableCommands[0].Name != "todo-write" {
		t.Errorf("expected last update to be captured, got name=%q", captured.AvailableCommands[0].Name)
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

func TestLoadSuppression_CapturedStateReemittedAfterLoad(t *testing.T) {
	a := newTestAdapter()

	// Simulate a load: set the flag and send replay notifications.
	a.mu.Lock()
	a.isLoadingSession = true
	a.mu.Unlock()

	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
			AvailableCommands: []acp.AvailableCommand{
				{Name: "todo-write", Description: "Write todos"},
			},
		},
	}))
	a.handleACPUpdate(makeNotification("s1", acp.SessionUpdate{
		Plan: &acp.SessionUpdatePlan{
			Entries: []acp.PlanEntry{
				{Content: "implement feature", Status: "in_progress", Priority: "high"},
			},
		},
	}))

	// Verify nothing was emitted during load.
	events := drainEvents(a)
	if len(events) != 0 {
		t.Fatalf("expected 0 events during load, got %d", len(events))
	}

	// Simulate what LoadSession() does after the RPC returns:
	// read and clear the captured state, then re-emit it.
	a.mu.Lock()
	replayCommands := a.loadReplayAvailableCommands
	replayPlan := a.loadReplayPlan
	a.loadReplayAvailableCommands = nil
	a.loadReplayPlan = nil
	a.mu.Unlock()

	if replayCommands != nil {
		a.sendUpdate(*a.convertAvailableCommands("s1", replayCommands))
	}
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

	// Now we should see exactly the re-emitted events.
	events = drainEvents(a)
	if len(events) != 2 {
		t.Fatalf("expected 2 re-emitted events, got %d", len(events))
	}

	// Verify available commands event.
	if events[0].Type != streams.EventTypeAvailableCommands {
		t.Errorf("expected event type %q, got %q", streams.EventTypeAvailableCommands, events[0].Type)
	}
	if len(events[0].AvailableCommands) != 1 || events[0].AvailableCommands[0].Name != "todo-write" {
		t.Errorf("unexpected available commands: %+v", events[0].AvailableCommands)
	}

	// Verify plan event.
	if events[1].Type != streams.EventTypePlan {
		t.Errorf("expected event type %q, got %q", streams.EventTypePlan, events[1].Type)
	}
	if len(events[1].PlanEntries) != 1 || events[1].PlanEntries[0].Description != "implement feature" {
		t.Errorf("unexpected plan entries: %+v", events[1].PlanEntries)
	}
	if events[1].PlanEntries[0].Status != "in_progress" {
		t.Errorf("expected plan status 'in_progress', got %q", events[1].PlanEntries[0].Status)
	}
	if events[1].PlanEntries[0].Priority != "high" {
		t.Errorf("expected plan priority 'high', got %q", events[1].PlanEntries[0].Priority)
	}

	// Verify captured state was cleared.
	a.mu.RLock()
	if a.loadReplayAvailableCommands != nil {
		t.Error("loadReplayAvailableCommands should be nil after re-emit")
	}
	if a.loadReplayPlan != nil {
		t.Error("loadReplayPlan should be nil after re-emit")
	}
	a.mu.RUnlock()
}
