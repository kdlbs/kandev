package instance

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/pkg/agent"
)

func newTurnOutcomeTestManager(t *testing.T) *Manager {
	t.Helper()
	log := newTestLogger(t)
	cfg := &config.Config{
		Ports:    config.PortConfig{Base: 0, Max: 0},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}
	mgr := NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(context.Background()) })
	return mgr
}

func addTestInstance(t *testing.T, mgr *Manager, id string) *Instance {
	t.Helper()
	inst := &Instance{ID: id}
	mgr.mu.Lock()
	mgr.instances[id] = inst
	mgr.mu.Unlock()
	return inst
}

// TestRetainTurnOutcomeReturnsFalseForUnknownInstance pins that no turn
// identifier is allocated for an instance that does not exist -- a caller
// racing instance teardown should never burn an identifier nobody can
// retrieve.
func TestRetainTurnOutcomeReturnsFalseForUnknownInstance(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)

	turnID, ok := mgr.RetainTurnOutcome("missing", streams.AgentEvent{Type: streams.EventTypeComplete})
	if ok {
		t.Fatal("RetainTurnOutcome() ok = true for an unknown instance, want false")
	}
	if turnID != 0 {
		t.Fatalf("RetainTurnOutcome() turnID = %d for an unknown instance, want 0", turnID)
	}
}

// TestPeekTurnOutcomeIsRepeatableAndNonDiscarding pins
// AC-EXECUTORS-SURVIVAL-004.6: retrieval never clears the retained outcome,
// so repeated reads return the identical outcome and turn identifier.
func TestPeekTurnOutcomeIsRepeatableAndNonDiscarding(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)
	addTestInstance(t, mgr, "inst-1")

	turnID, ok := mgr.RetainTurnOutcome("inst-1", streams.AgentEvent{Type: streams.EventTypeComplete, SessionID: "sess-1"})
	if !ok {
		t.Fatal("RetainTurnOutcome() ok = false, want true")
	}

	for i := 0; i < 3; i++ {
		outcome, hasOutcome, instanceFound := mgr.PeekTurnOutcome("inst-1")
		if !instanceFound {
			t.Fatalf("iteration %d: instanceFound = false, want true", i)
		}
		if !hasOutcome {
			t.Fatalf("iteration %d: hasOutcome = false, want true (retrieval must not discard)", i)
		}
		if outcome.TurnID != turnID {
			t.Fatalf("iteration %d: TurnID = %d, want %d", i, outcome.TurnID, turnID)
		}
		if outcome.Event.SessionID != "sess-1" {
			t.Fatalf("iteration %d: Event.SessionID = %q, want %q", i, outcome.Event.SessionID, "sess-1")
		}
	}
}

// TestPeekTurnOutcomeDistinguishesUnknownInstanceFromNothingRetained pins
// that a 404-worthy "no such instance" is distinguishable from the normal
// "instance exists, nothing retained yet" answer.
func TestPeekTurnOutcomeDistinguishesUnknownInstanceFromNothingRetained(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)
	addTestInstance(t, mgr, "inst-1")

	_, hasOutcome, instanceFound := mgr.PeekTurnOutcome("inst-1")
	if !instanceFound {
		t.Fatal("instanceFound = false for a real instance, want true")
	}
	if hasOutcome {
		t.Fatal("hasOutcome = true before anything was retained, want false")
	}

	_, _, instanceFound = mgr.PeekTurnOutcome("missing")
	if instanceFound {
		t.Fatal("instanceFound = true for an unknown instance, want false")
	}
}

// TestAckTurnOutcomeDiscardsOnlyTheMatchingIdentifier pins the core of
// AC-EXECUTORS-SURVIVAL-004.6: acking the wrong identifier changes nothing,
// while acking the right one discards it.
func TestAckTurnOutcomeDiscardsOnlyTheMatchingIdentifier(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)
	addTestInstance(t, mgr, "inst-1")

	turnID, _ := mgr.RetainTurnOutcome("inst-1", streams.AgentEvent{Type: streams.EventTypeComplete})

	mgr.AckTurnOutcome("inst-1", turnID+999)
	if _, hasOutcome, _ := mgr.PeekTurnOutcome("inst-1"); !hasOutcome {
		t.Fatal("acking the wrong turn ID discarded the outcome, want it left retained")
	}

	mgr.AckTurnOutcome("inst-1", turnID)
	if _, hasOutcome, _ := mgr.PeekTurnOutcome("inst-1"); hasOutcome {
		t.Fatal("acking the matching turn ID left the outcome retained, want it discarded")
	}
}

// TestAckTurnOutcomeOnUnknownInstanceIsANoOp pins that acking an instance
// that no longer exists (e.g. already torn down) never panics and never
// errors -- an unknown identifier is always accepted and does nothing.
func TestAckTurnOutcomeOnUnknownInstanceIsANoOp(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)
	mgr.AckTurnOutcome("missing", 1)
}

// TestRetainTurnOutcomeAllocatesStrictlyIncreasingIdentifiersAcrossInstances
// pins AC-EXECUTORS-SURVIVAL-004.1: the identifier is unique across every
// turn of every instance this control server supervises, not just within
// one instance.
func TestRetainTurnOutcomeAllocatesStrictlyIncreasingIdentifiersAcrossInstances(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)
	addTestInstance(t, mgr, "inst-1")
	addTestInstance(t, mgr, "inst-2")

	id1, _ := mgr.RetainTurnOutcome("inst-1", streams.AgentEvent{Type: streams.EventTypeComplete})
	id2, _ := mgr.RetainTurnOutcome("inst-2", streams.AgentEvent{Type: streams.EventTypeComplete})
	id3, _ := mgr.RetainTurnOutcome("inst-1", streams.AgentEvent{Type: streams.EventTypeError})

	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Fatalf("turn identifiers were not unique: id1=%d id2=%d id3=%d", id1, id2, id3)
	}
}

// TestRetainTurnOutcomeReplacesThePreviouslyRetainedOutcome pins design 03's
// "agentctl therefore retains each instance's last terminal turn outcome" --
// only the most recent terminal outcome is kept, not a history.
func TestRetainTurnOutcomeReplacesThePreviouslyRetainedOutcome(t *testing.T) {
	mgr := newTurnOutcomeTestManager(t)
	addTestInstance(t, mgr, "inst-1")

	firstID, _ := mgr.RetainTurnOutcome("inst-1", streams.AgentEvent{Type: streams.EventTypeError})
	secondID, _ := mgr.RetainTurnOutcome("inst-1", streams.AgentEvent{Type: streams.EventTypeComplete})

	outcome, hasOutcome, _ := mgr.PeekTurnOutcome("inst-1")
	if !hasOutcome {
		t.Fatal("hasOutcome = false, want true")
	}
	if outcome.TurnID != secondID {
		t.Fatalf("TurnID = %d, want the second (most recent) outcome's ID %d (first was %d)", outcome.TurnID, secondID, firstID)
	}
	if outcome.Event.Type != streams.EventTypeComplete {
		t.Fatalf("Event.Type = %q, want the most recent outcome's type %q", outcome.Event.Type, streams.EventTypeComplete)
	}
}
