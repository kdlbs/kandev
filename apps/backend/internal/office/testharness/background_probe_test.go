package testharness

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
)

func TestScriptedBackgroundProbeUnscriptedSessionReturnsUnknown(t *testing.T) {
	p := NewScriptedBackgroundProbe()
	result, err := p.Probe(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != executor.ProbeResultUnknown {
		t.Fatalf("expected ProbeResultUnknown for an unscripted session, got %q", result)
	}
}

func TestScriptedBackgroundProbeReplaysSequenceInOrder(t *testing.T) {
	p := NewScriptedBackgroundProbe()
	p.Script("session-1", []executor.ProbeResult{
		executor.ProbeResultLive,
		executor.ProbeResultLive,
		executor.ProbeResultSettled,
	})

	want := []executor.ProbeResult{
		executor.ProbeResultLive,
		executor.ProbeResultLive,
		executor.ProbeResultSettled,
	}
	for i, expected := range want {
		got, err := p.Probe(context.Background(), "session-1")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if got != expected {
			t.Fatalf("call %d: expected %q, got %q", i, expected, got)
		}
	}
}

func TestScriptedBackgroundProbeHoldsAtLastValueOnceExhausted(t *testing.T) {
	p := NewScriptedBackgroundProbe()
	p.Script("session-1", []executor.ProbeResult{executor.ProbeResultLive, executor.ProbeResultSettled})

	for i := 0; i < 2; i++ {
		if _, err := p.Probe(context.Background(), "session-1"); err != nil {
			t.Fatalf("priming call %d: unexpected error: %v", i, err)
		}
	}

	for i := 0; i < 3; i++ {
		got, err := p.Probe(context.Background(), "session-1")
		if err != nil {
			t.Fatalf("post-exhaustion call %d: unexpected error: %v", i, err)
		}
		if got != executor.ProbeResultSettled {
			t.Fatalf("post-exhaustion call %d: expected the scripted sequence to hold at its last value %q, got %q",
				i, executor.ProbeResultSettled, got)
		}
	}
}

func TestScriptedBackgroundProbeIsolatesSessionsIndependently(t *testing.T) {
	p := NewScriptedBackgroundProbe()
	p.Script("session-live", []executor.ProbeResult{executor.ProbeResultLive})
	p.Script("session-settled", []executor.ProbeResult{executor.ProbeResultSettled})

	liveResult, err := p.Probe(context.Background(), "session-live")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if liveResult != executor.ProbeResultLive {
		t.Fatalf("expected session-live to probe live, got %q", liveResult)
	}

	settledResult, err := p.Probe(context.Background(), "session-settled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settledResult != executor.ProbeResultSettled {
		t.Fatalf("expected session-settled to probe settled, got %q", settledResult)
	}

	// session-live's own sequence must not have advanced because
	// session-settled was probed in between (per-session call counters).
	liveResultAgain, err := p.Probe(context.Background(), "session-live")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if liveResultAgain != executor.ProbeResultLive {
		t.Fatalf("expected session-live to still hold at live, got %q", liveResultAgain)
	}

	unscripted, err := p.Probe(context.Background(), "session-unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unscripted != executor.ProbeResultUnknown {
		t.Fatalf("expected an unrelated unscripted session to still read Unknown, got %q", unscripted)
	}
}

func TestScriptedBackgroundProbeRescriptResetsCallCounter(t *testing.T) {
	p := NewScriptedBackgroundProbe()
	p.Script("session-1", []executor.ProbeResult{executor.ProbeResultLive, executor.ProbeResultLive})
	if _, err := p.Probe(context.Background(), "session-1"); err != nil {
		t.Fatalf("priming call: unexpected error: %v", err)
	}

	p.Script("session-1", []executor.ProbeResult{executor.ProbeResultSettled})
	got, err := p.Probe(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != executor.ProbeResultSettled {
		t.Fatalf("expected re-scripting to reset the call counter and start the new sequence at index 0, got %q", got)
	}
}
