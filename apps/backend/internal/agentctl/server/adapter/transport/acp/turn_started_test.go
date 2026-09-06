package acp

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// D3: a human prompt dispatch emits turn_started before the turn completes.
func TestSendPrompt_HumanPathEmitsTurnStarted(t *testing.T) {
	a, fa := setupConcurrencyFakeAgent(t)
	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- a.Prompt(ctx, "user message", nil, 1)
	}()

	started := waitForEventType(t, a, streams.EventTypeTurnStarted)
	if started.SessionID != "sess-concurrency-test" {
		t.Errorf("turn_started SessionID = %q, want sess-concurrency-test", started.SessionID)
	}

	<-fa.entered
	close(fa.release)
	if err := <-done; err != nil {
		t.Fatalf("Prompt: %v", err)
	}
}

// D3, §N: a synthetic ScheduleWakeup self-resume dispatches session/prompt
// exactly like a human prompt and must emit turn_started too — sendPrompt
// distinguishes the two paths via humanPrompt for other purposes, but this
// event must not be gated on that distinction.
func TestFireWakeup_EmitsTurnStarted(t *testing.T) {
	a, fa := setupConcurrencyFakeAgent(t)
	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	a.fireWakeup("sess-concurrency-test", "wakeup prompt")

	started := waitForEventType(t, a, streams.EventTypeTurnStarted)
	if started.SessionID != "sess-concurrency-test" {
		t.Errorf("turn_started SessionID = %q, want sess-concurrency-test", started.SessionID)
	}

	<-fa.entered
	close(fa.release)
}

// A wakeup pinned to a session that has since changed is dropped before
// beginPromptTurn runs, so it must not emit turn_started for the stale
// session — the turn it names never actually starts.
func TestFireWakeup_SessionChangedEmitsNoTurnStarted(t *testing.T) {
	a, _ := setupConcurrencyFakeAgent(t)
	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	_ = drainEvents(a)

	a.fireWakeup("a-session-that-is-not-current", "stale wakeup")

	time.Sleep(50 * time.Millisecond)
	assertNoAdapterEvent(t, a, "after a wakeup pinned to a stale session")
}

func TestBeginPromptTurn_RecordsTurnStart(t *testing.T) {
	a := newTestAdapter()
	defer func() { _ = a.Close() }()

	if _, ok := a.RecordedTurnStart("s1"); ok {
		t.Fatalf("expected no recorded turn start before any prompt")
	}

	before := time.Now()
	a.beginPromptTurn("s1")
	_ = waitForEventType(t, a, streams.EventTypeTurnStarted)
	after := time.Now()

	got, ok := a.RecordedTurnStart("s1")
	if !ok {
		t.Fatalf("expected a recorded turn start after beginPromptTurn")
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("recorded turn start %v not within [%v, %v]", got, before, after)
	}
}

// D3 / §"agentctl clears [the recorded turn start] on session teardown".
func TestNewSession_ClearsRecordedTurnStart(t *testing.T) {
	a, _ := setupConcurrencyFakeAgent(t)
	ctx := context.Background()
	if err := a.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	a.beginPromptTurn("old-session")
	_ = waitForEventType(t, a, streams.EventTypeTurnStarted)
	if _, ok := a.RecordedTurnStart("old-session"); !ok {
		t.Fatalf("expected a recorded turn start before NewSession")
	}

	if _, err := a.NewSession(ctx, nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if _, ok := a.RecordedTurnStart("old-session"); ok {
		t.Errorf("expected the old session's recorded turn start to be cleared by NewSession")
	}
}
