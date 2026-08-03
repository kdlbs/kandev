package cron

import (
	"context"
	"reflect"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// RoutineTicker is the surface RoutinesHandler needs from the office
// routines service. It mirrors the production
// office/routines.RoutineService.TickScheduledTriggers signature so the
// real service satisfies the interface for free; tests pass a fake.
type RoutineTicker interface {
	TickScheduledTriggers(ctx context.Context, now time.Time) error
}

// RoutinesHandler is a thin adapter that drives
// RoutineService.TickScheduledTriggers from the shared cron loop.
//
// Phase 5 deliberately does not change routine dispatch behaviour: the
// existing concurrency policy / fingerprint / cron-expression logic
// inside the routines service is what produces tasks. The only collapse
// versus pre-Phase-5 is that the newly created task's first step's
// on_enter trigger drives the assignee wakeup through the workflow
// engine — the routine itself no longer needs to emit a manual
// task_assigned run, which is what the existing office event
// subscribers handle.
type RoutinesHandler struct {
	ticker RoutineTicker
	now    func() time.Time
	log    *logger.Logger
}

// NewRoutinesHandler builds a RoutinesHandler. now defaults to
// time.Now().UTC() when nil — tests pass a controlled clock.
//
// A typed-nil collaborator (a concrete nil pointer assigned into the
// RoutineTicker interface, as happens when Office is disabled) is
// normalised to a genuinely nil interface so Tick takes the no-op branch
// instead of dereferencing a nil receiver.
func NewRoutinesHandler(ticker RoutineTicker, now func() time.Time, log *logger.Logger) *RoutinesHandler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if isNilTicker(ticker) {
		ticker = nil
	}
	return &RoutinesHandler{
		ticker: ticker,
		now:    now,
		log:    log.WithFields(zap.String("handler", "routines")),
	}
}

// isNilTicker reports whether ticker is absent, treating a typed-nil
// pointer wrapped in the interface the same as a genuinely nil interface.
func isNilTicker(ticker RoutineTicker) bool {
	if ticker == nil {
		return true
	}
	// Every RoutineTicker implementation today is a pointer receiver, so Ptr is
	// the only kind reached in practice; the remaining nil-able kinds are listed
	// defensively so a future value/func/channel implementer can never smuggle a
	// typed-nil past this guard and re-open the Office-disabled tick panic.
	v := reflect.ValueOf(ticker)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Chan, reflect.Func, reflect.Slice, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// Name implements Handler.
func (h *RoutinesHandler) Name() string { return "routines" }

// Tick implements Handler. Forwards to the routines service and lets
// it own claim / dispatch / concurrency-policy semantics. A nil ticker
// is treated as a no-op so the cron loop can be started before the
// office service is fully wired (e.g. during e2e fixtures).
func (h *RoutinesHandler) Tick(ctx context.Context) error {
	if h.ticker == nil {
		return nil
	}
	return h.ticker.TickScheduledTriggers(ctx, h.now())
}
