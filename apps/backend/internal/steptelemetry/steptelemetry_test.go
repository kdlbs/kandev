package steptelemetry

import (
	"context"
	"testing"
)

func TestFromContextDefaultsToUnknownWhenNothingAttached(t *testing.T) {
	a := FromContext(context.Background())
	if a.Trigger != TriggerUnknown {
		t.Errorf("Trigger = %q, want %q", a.Trigger, TriggerUnknown)
	}
	if a.ActorKind != ActorUnknown {
		t.Errorf("ActorKind = %q, want %q", a.ActorKind, ActorUnknown)
	}
	if a.ActorID != "" {
		t.Errorf("ActorID = %q, want empty", a.ActorID)
	}
	if a.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", a.SessionID)
	}
}

func TestWithAttributionRoundTrips(t *testing.T) {
	want := Attribution{
		Trigger:   TriggerMCPMove,
		ActorKind: ActorAgent,
		ActorID:   "session-1",
		SessionID: "session-1",
	}
	ctx := WithAttribution(context.Background(), want)
	got := FromContext(ctx)
	if got != want {
		t.Errorf("FromContext = %+v, want %+v", got, want)
	}
}

func TestFromContextNormalizesPartialAttribution(t *testing.T) {
	// A caller that sets only a trigger still gets ActorUnknown, never a
	// zero-value empty string treated specially.
	ctx := WithAttribution(context.Background(), Attribution{Trigger: TriggerBulkMove})
	got := FromContext(ctx)
	if got.Trigger != TriggerBulkMove {
		t.Errorf("Trigger = %q, want %q", got.Trigger, TriggerBulkMove)
	}
	if got.ActorKind != ActorUnknown {
		t.Errorf("ActorKind = %q, want %q", got.ActorKind, ActorUnknown)
	}
}

func TestHasTriggerDistinguishesAbsentFromExplicit(t *testing.T) {
	if HasTrigger(context.Background()) {
		t.Error("HasTrigger on bare context = true, want false")
	}
	ctx := WithAttribution(context.Background(), Attribution{Trigger: TriggerMCPMove})
	if !HasTrigger(ctx) {
		t.Error("HasTrigger after WithAttribution = false, want true")
	}
}

func TestWithAttributionOuterCallerWinsWhenInnerCheckFirst(t *testing.T) {
	// Simulates the outermost-caller-wins rule: MoveTaskWithOptions must set
	// manual_move only when the context carries no trigger already, so an
	// mcp_move set by the MCP handler survives the inner call.
	outer := WithAttribution(context.Background(), Attribution{Trigger: TriggerMCPMove, ActorKind: ActorAgent, ActorID: "s1", SessionID: "s1"})

	inner := outer
	if !HasTrigger(inner) {
		inner = WithAttribution(inner, Attribution{Trigger: TriggerManualMove, ActorKind: ActorHuman})
	}

	got := FromContext(inner)
	if got.Trigger != TriggerMCPMove {
		t.Errorf("Trigger after inner-defaulting = %q, want outer's %q to survive", got.Trigger, TriggerMCPMove)
	}
}
