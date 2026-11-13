package dynamic

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

func engineBackoffFixture(clock *backoffClock) (*Engine, *CircuitRegistry, Profile) {
	registry := newBackoffRegistry(clock)
	engine := NewEngine(WithCircuitRegistry(registry), WithClock(clock.Now))
	profile := Profile{
		ID: "dynamic", Version: 1,
		Candidates: []Candidate{
			{ID: "candidate-a", Enabled: true, BindingKey: "binding-a", Policies: routingpolicy.DefaultDocument()},
			{ID: "candidate-b", Enabled: true, BindingKey: "binding-b", Policies: routingpolicy.DefaultDocument()},
		},
	}
	return engine, registry, profile
}

// midTurnCycle selects candidate-a for a new session (a probe once its
// circuit expired), reports a successful launch, then records a mid-turn
// provider failure. It returns the backoff that failure opened.
func midTurnCycle(t *testing.T, engine *Engine, registry *CircuitRegistry, profile Profile, clock *backoffClock, sessionID string) time.Duration {
	t.Helper()
	decision, err := engine.Select(sessionID, profile, 0, "")
	if err != nil || decision.ExecutionProfileID != "candidate-a" {
		t.Fatalf("select for %s = %#v, %v; want candidate-a", sessionID, decision, err)
	}
	engine.ReleaseProbe(decision, true)
	if _, err := engine.RecordFailureCircuit(context.Background(), sessionID, profile, decision.Generation, "candidate-a", &routingerr.Error{
		Code: routingerr.CodeProviderUnavailable, Class: routingerr.ClassTransient, FallbackAllowed: true,
	}); err != nil {
		t.Fatalf("RecordFailureCircuit: %v", err)
	}
	return circuitUntil(t, registry, "binding-a").Sub(clock.now)
}

// A candidate whose launch succeeds and whose turn then fails mid-turn
// escalates across cycles; a successful launch does not reset the count and
// only a clean turn completion of the current generation resets the ladder.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestEngineMidTurnFailuresEscalateAcrossSuccessfulLaunches(t *testing.T) {
	clock := &backoffClock{now: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	engine, registry, profile := engineBackoffFixture(clock)
	sessions := []string{"session-a", "session-b", "session-c"}
	for index, want := range []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute} {
		if got := midTurnCycle(t, engine, registry, profile, clock, sessions[index]); got != want {
			t.Fatalf("cycle %d backoff = %v, want %v", index+1, got, want)
		}
		clock.now = clock.now.Add(want + time.Second)
	}

	// A later turn on the candidate completes without a provider error.
	clean, err := engine.Select("session-clean", profile, 0, "")
	if err != nil || clean.ExecutionProfileID != "candidate-a" {
		t.Fatalf("clean select = %#v, %v", clean, err)
	}
	engine.ReleaseProbe(clean, true)
	if _, err := engine.ResetCircuitFailures(context.Background(), "session-clean", profile, clean.Generation+1, "candidate-a"); err == nil {
		t.Fatal("a stale completion was accepted")
	}
	if got := circuitFailures(registry, "binding-a"); got != 3 {
		t.Fatalf("a stale completion changed the failure count to %d", got)
	}
	if _, err := engine.ResetCircuitFailures(context.Background(), "session-clean", profile, clean.Generation, "candidate-a"); err != nil {
		t.Fatalf("ResetCircuitFailures: %v", err)
	}
	if got := circuitFailures(registry, "binding-a"); got != 0 {
		t.Fatalf("failures after a clean turn = %d, want 0", got)
	}
	if got := midTurnCycle(t, engine, registry, profile, clock, "session-after-clean"); got != time.Minute {
		t.Fatalf("backoff after a clean turn = %v, want 1m", got)
	}
}

// A failed probe launch advances the ladder through the engine.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestEngineFailedProbeBackoffFollowsTheLadder(t *testing.T) {
	clock := &backoffClock{now: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	engine, registry, profile := engineBackoffFixture(clock)
	midTurnCycle(t, engine, registry, profile, clock, "session-1")
	clock.now = clock.now.Add(time.Minute + time.Second)
	probe, err := engine.Select("session-2", profile, 0, "")
	if err != nil || probe.ExecutionProfileID != "candidate-a" {
		t.Fatalf("probe select = %#v, %v; want the probe on candidate-a", probe, err)
	}
	engine.ReleaseProbe(probe, false)
	if got, want := circuitUntil(t, registry, "binding-a"), clock.now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("failed probe backoff = %v, want %v", got.Sub(clock.now), 5*time.Minute)
	}
}
