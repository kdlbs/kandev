package dynamic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

func failureCircuitFixture(now *time.Time) (*Engine, *CircuitRegistry, Profile) {
	clock := func() time.Time { return *now }
	registry := NewCircuitRegistry(WithCircuitClock(clock))
	engine := NewEngine(WithClock(clock), WithCircuitRegistry(registry))
	profile := Profile{
		ID: "dynamic", Version: 1,
		Candidates: []Candidate{
			{ID: "candidate-a", Enabled: true, BindingKey: "binding-a", Policies: routingpolicy.DefaultDocument()},
			{ID: "candidate-b", Enabled: true, BindingKey: "binding-b", Policies: routingpolicy.DefaultDocument()},
		},
	}
	return engine, registry, profile
}

func providerUnavailable() *routingerr.Error {
	return &routingerr.Error{
		Code: routingerr.CodeProviderUnavailable, Class: routingerr.ClassTransient,
		Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
	}
}

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
func TestRecordFailureCircuitOpensWithoutRouting(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	engine, registry, profile := failureCircuitFixture(&now)
	initial, err := engine.Select("session", profile, 0, "")
	if err != nil || initial.ExecutionProfileID != "candidate-a" {
		t.Fatalf("Select = %#v, %v; want candidate-a", initial, err)
	}
	before, _ := engine.State("session")

	opened, err := engine.RecordFailureCircuit(context.Background(), "session", profile, initial.Generation, "candidate-a", providerUnavailable())
	if err != nil || !opened {
		t.Fatalf("RecordFailureCircuit = %v, %v; want opened", opened, err)
	}
	if !registry.IsOpen("binding-a", now) {
		t.Fatal("the failing candidate's circuit stayed closed")
	}
	if registry.IsOpen("binding-b", now) {
		t.Fatal("another candidate's circuit opened")
	}
	after, _ := engine.State("session")
	if after != before {
		t.Fatalf("route state changed: before %#v, after %#v", before, after)
	}
	next, err := engine.Select("other-session", profile, 0, "")
	if err != nil || next.ExecutionProfileID != "candidate-b" {
		t.Fatalf("next selection = %#v, %v; want candidate-b", next, err)
	}
}

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
func TestRecordFailureCircuitIgnoresStaleAttempts(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	engine, registry, profile := failureCircuitFixture(&now)
	initial, err := engine.Select("session", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	moved, err := engine.ApplyFailure("session", profile, initial.Generation, "candidate-a", providerUnavailable())
	if err != nil || moved.ExecutionProfileID != "candidate-b" {
		t.Fatalf("route after the first failure = %#v, %v; want candidate-b", moved, err)
	}
	stale := []struct {
		name       string
		generation int64
		candidate  string
	}{
		{name: "old generation and old candidate", generation: initial.Generation, candidate: "candidate-a"},
		{name: "current generation with the old candidate", generation: moved.Generation, candidate: "candidate-a"},
		{name: "old generation with the current candidate", generation: initial.Generation, candidate: "candidate-b"},
	}
	for _, tc := range stale {
		opened, err := engine.RecordFailureCircuit(context.Background(), "session", profile, tc.generation, tc.candidate, providerUnavailable())
		if opened || !errors.Is(err, ErrStaleGeneration) {
			t.Errorf("%s: RecordFailureCircuit = %v, %v; want %v", tc.name, opened, err, ErrStaleGeneration)
		}
	}
	if registry.IsOpen("binding-b", now) {
		t.Error("a stale failure opened the circuit of the current candidate")
	}
	state, _ := engine.State("session")
	if state.Generation != moved.Generation || state.ExecutionProfileID != "candidate-b" {
		t.Fatalf("route state = %#v, want generation %d on candidate-b", state, moved.Generation)
	}
}

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
func TestRecordFailureCircuitSkipsNonQualifyingCodes(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	engine, registry, profile := failureCircuitFixture(&now)
	initial, err := engine.Select("session", profile, 0, "")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	for _, code := range []routingerr.Code{routingerr.CodeUnknownProvider, routingerr.CodeAgentRuntime, routingerr.CodeAccountBlocked} {
		failure := &routingerr.Error{Code: code, Class: routingerr.ClassForCode(code)}
		opened, err := engine.RecordFailureCircuit(context.Background(), "session", profile, initial.Generation, "candidate-a", failure)
		if opened || err != nil {
			t.Errorf("%s: RecordFailureCircuit = %v, %v; want not opened", code, opened, err)
		}
	}
	if registry.IsOpen("binding-a", now) {
		t.Fatal("a non-qualifying failure opened the circuit")
	}
	if registry.IsHeld(accountBlockKey("candidate-a")) {
		t.Fatal("RecordFailureCircuit held the account; the caller owns the hold")
	}
}
