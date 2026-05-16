package cron

import (
	"context"
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
func NewRoutinesHandler(ticker RoutineTicker, now func() time.Time, log *logger.Logger) *RoutinesHandler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &RoutinesHandler{
		ticker: ticker,
		now:    now,
		log:    log.WithFields(zap.String("handler", "routines")),
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
