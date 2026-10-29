package orchestrator

import (
	"expvar"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Child-turn stalled signal metrics. Labels are closed sets (outcome, cause,
// reason); task, session, and turn identifiers are never labels.
var (
	childStallOutcomes      = expvar.NewMap("task_child_stall_outcome_total")
	childStallCauses        = expvar.NewMap("task_child_stall_cause_total")
	childStallReasons       = expvar.NewMap("task_child_stall_suppressed_total")
	childStallPending       = expvar.NewInt("task_child_stall_pending")
	childStallOldestPending = expvar.NewInt("task_child_stall_oldest_pending_seconds")
)

func recordChildStallOutcome(outcome string) {
	if outcome != "" {
		childStallOutcomes.Add(outcome, 1)
	}
}

func recordChildStallCause(cause string) {
	if cause != "" {
		childStallCauses.Add(cause, 1)
	}
}

func recordChildStallReason(reason string) {
	if reason != "" {
		childStallReasons.Add(reason, 1)
	}
}

// childStallPassStats aggregates unresolved candidates seen in one pass.
type childStallPassStats struct {
	pending     int64
	oldestSince time.Time
}

func (s *childStallPassStats) observePending(turn *models.Turn) {
	s.pending++
	if turn.CompletedAt != nil && (s.oldestSince.IsZero() || turn.CompletedAt.Before(s.oldestSince)) {
		s.oldestSince = *turn.CompletedAt
	}
}

func (s *childStallPassStats) publish(now time.Time) {
	childStallPending.Set(s.pending)
	if s.oldestSince.IsZero() {
		childStallOldestPending.Set(0)
		return
	}
	childStallOldestPending.Set(int64(now.Sub(s.oldestSince) / time.Second))
}
