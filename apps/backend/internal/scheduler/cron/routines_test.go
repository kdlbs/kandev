package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

type fakeRoutineTicker struct {
	calls   int
	lastNow time.Time
	err     error
}

func (f *fakeRoutineTicker) TickScheduledTriggers(_ context.Context, now time.Time) error {
	f.calls++
	f.lastNow = now
	return f.err
}

func TestRoutinesHandler_ForwardsToTicker(t *testing.T) {
	tk := &fakeRoutineTicker{}
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	h := NewRoutinesHandler(tk, func() time.Time { return now }, logger.Default())

	if h.Name() != "routines" {
		t.Errorf("name = %q", h.Name())
	}
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if tk.calls != 1 {
		t.Errorf("ticker calls = %d, want 1", tk.calls)
	}
	if !tk.lastNow.Equal(now) {
		t.Errorf("lastNow = %v, want %v", tk.lastNow, now)
	}
}

func TestRoutinesHandler_PropagatesError(t *testing.T) {
	tk := &fakeRoutineTicker{err: errors.New("boom")}
	h := NewRoutinesHandler(tk, nil, logger.Default())
	if err := h.Tick(context.Background()); err == nil {
		t.Fatal("expected error to surface")
	}
}

func TestRoutinesHandler_NilTickerIsNoOp(t *testing.T) {
	h := NewRoutinesHandler(nil, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

// TestRoutinesHandler_TypedNilTickerIsNoOp reproduces the Office-disabled
// regression: a concrete nil pointer assigned into the RoutineTicker
// interface is a non-nil interface value, so a naive h.ticker == nil guard
// is bypassed and Tick dereferences the nil receiver. A handler built from a
// typed-nil collaborator by any wiring path must no-op instead of panicking.
func TestRoutinesHandler_TypedNilTickerIsNoOp(t *testing.T) {
	var typedNil *fakeRoutineTicker
	var ticker RoutineTicker = typedNil // non-nil interface wrapping a nil pointer

	h := NewRoutinesHandler(ticker, nil, logger.Default())
	if err := h.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}
