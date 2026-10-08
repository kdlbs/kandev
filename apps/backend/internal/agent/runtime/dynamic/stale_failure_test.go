package dynamic

import (
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
)

// After a failure moved the route from candidate-a to its successor, a later
// failure that still names the old generation or the old candidate belongs to
// the attempt the route already left. It must not open the circuit of the
// candidate the route holds now, and it must not advance the route.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
func TestStaleFailureDoesNotOpenTheCurrentCandidateCircuit(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	registry := NewCircuitRegistry()
	engine := NewEngine(WithClock(func() time.Time { return now }), WithCircuitRegistry(registry))
	profile := Profile{
		ID: "dynamic", Version: 1,
		Candidates: []Candidate{
			{ID: "candidate-a", Enabled: true, BindingKey: "binding-a", Policies: routingpolicy.DefaultDocument()},
			{ID: "candidate-b", Enabled: true, BindingKey: "binding-b", Policies: routingpolicy.DefaultDocument()},
			{ID: "candidate-c", Enabled: true, BindingKey: "binding-c", Policies: routingpolicy.DefaultDocument()},
		},
	}
	rateLimited := &routingerr.Error{
		Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient,
		Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
	}
	initial, err := engine.Select("session", profile, 0, "")
	if err != nil || initial.ExecutionProfileID != "candidate-a" {
		t.Fatalf("Select = %#v, %v; want candidate-a", initial, err)
	}
	moved, err := engine.ApplyFailure("session", profile, initial.Generation, "candidate-a", rateLimited)
	if err != nil {
		t.Fatalf("ApplyFailure: %v", err)
	}
	if moved.ExecutionProfileID != "candidate-b" || moved.Generation != initial.Generation+1 {
		t.Fatalf("route after the candidate-a failure = %#v, want candidate-b at generation %d", moved, initial.Generation+1)
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
		if _, err := engine.ApplyFailure("session", profile, tc.generation, tc.candidate, rateLimited); !errors.Is(err, ErrStaleGeneration) {
			t.Errorf("%s: ApplyFailure error = %v, want %v", tc.name, err, ErrStaleGeneration)
		}
	}
	for _, candidate := range profile.Candidates[1:] {
		if registry.IsOpen(candidate.BindingKey, now) {
			t.Errorf("a stale failure opened the circuit of %s", candidate.ID)
		}
	}
	state, exists := engine.State("session")
	if !exists || state.Generation != moved.Generation || state.ExecutionProfileID != "candidate-b" {
		t.Fatalf("route state = %#v (exists=%v), want generation %d on candidate-b", state, exists, moved.Generation)
	}
}
