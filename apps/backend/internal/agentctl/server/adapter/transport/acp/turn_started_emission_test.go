package acp

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestSendPrompt_RecordsTurnStartBeforeDispatch verifies D3/AC-41b's central
// ordering guarantee for the human-prompt path: both the turn-start stamp
// (cfg.RecordTurnStart) and the turn_started event are performed inside
// sendPrompt's syncNotifQueueThen barrier callback, and are therefore
// guaranteed to have happened before conn.Prompt is ever dispatched to the
// agent. The fake agent's Prompt handler signals `entered` as the very first
// thing it does, so observing that signal proves conn.Prompt was reached —
// and by then both writes must already have landed.
func TestSendPrompt_RecordsTurnStartBeforeDispatch(t *testing.T) {
	a, fa := setupConcurrencyFakeAgent(t)

	var stamped atomic.Bool
	a.cfg.RecordTurnStart = func(time.Time) { stamped.Store(true) }

	if err := a.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if _, err := a.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("new session: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- a.Prompt(context.Background(), "hello", nil, 1) }()

	select {
	case <-fa.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt never reached the fake agent")
	}

	if !stamped.Load() {
		t.Fatal("expected RecordTurnStart to be called before conn.Prompt dispatched")
	}

	events := drainEvents(a)
	var sawTurnStarted bool
	for _, ev := range events {
		if ev.Type == streams.EventTypeTurnStarted {
			sawTurnStarted = true
			if ev.PromptGeneration != 1 {
				t.Errorf("turn_started prompt_generation = %d, want 1", ev.PromptGeneration)
			}
		}
	}
	if !sawTurnStarted {
		t.Fatal("expected turn_started event to have been emitted before conn.Prompt dispatched")
	}

	close(fa.release)
	if err := <-done; err != nil {
		t.Fatalf("prompt returned error: %v", err)
	}
}

// TestFireWakeup_RecordsTurnStartWithGenerationZero verifies AC-41's second
// GIVEN and AC-41a's generation-0 clause: the synthetic ScheduleWakeup path
// also stamps the turn start and emits turn_started, carrying
// prompt_generation: 0 — there is no non-zero generation in existence to
// carry on this path, and that is correct, not a defect to work around.
func TestFireWakeup_RecordsTurnStartWithGenerationZero(t *testing.T) {
	a, fa := setupConcurrencyFakeAgent(t)

	var stamped atomic.Bool
	a.cfg.RecordTurnStart = func(time.Time) { stamped.Store(true) }

	if err := a.Initialize(context.Background()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	sessionID, err := a.NewSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	a.fireWakeup(sessionID, "synthetic wakeup prompt")

	select {
	case <-fa.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("synthetic wakeup prompt never reached the fake agent")
	}

	if !stamped.Load() {
		t.Fatal("expected RecordTurnStart to be called on the synthetic wakeup path")
	}

	events := drainEvents(a)
	var sawTurnStarted bool
	for _, ev := range events {
		if ev.Type == streams.EventTypeTurnStarted {
			sawTurnStarted = true
			if ev.PromptGeneration != 0 {
				t.Errorf("synthetic turn_started prompt_generation = %d, want 0", ev.PromptGeneration)
			}
		}
	}
	if !sawTurnStarted {
		t.Fatal("expected turn_started event on the synthetic wakeup path")
	}

	close(fa.release)
}
