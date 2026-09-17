package reachability

import (
	"context"
	"time"

	"go.uber.org/zap"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// Repository is the narrow slice of repository.ExecutorRepository this
// package depends on. The poller and the off-cycle-probe primitive share it,
// so a fake test double satisfies both without importing the full
// task/repository interface.
type Repository interface {
	ListSSHExecutorsForReachability(ctx context.Context) ([]*models.Executor, error)
	GetExecutorReachability(ctx context.Context, executorID string) (*models.ExecutorReachability, error)
	UpsertExecutorReachability(ctx context.Context, obs models.ExecutorReachabilityObservation) error
	ResetExecutorReachability(ctx context.Context, executorID, host string) error
}

// observeResult reports what Observe actually did, for the poller's logging
// and metrics and as the change signal a future consumer (the HTTP/event
// surface) reads off the write path rather than re-deriving. Changed is
// broader than StateChanged: the destination state can hold steady
// (unreachable stays unreachable) while the stored reason moves between two
// different failure causes, and that is still worth telling a client about.
type observeResult struct {
	Applied      bool
	StateChanged bool
	Changed      bool
	Previous     models.ExecutorReachabilityState
	Current      models.ExecutorReachabilityState
	// After is the full record as written, for a caller (the HTTP immediate-
	// probe route, the change-event publisher) that needs more than the bare
	// state enum without a second read-back.
	After *models.ExecutorReachability
}

// store owns the write path's hysteresis. The consecutive-failure counter
// and the destination state for an existing row are derived entirely inside
// UpsertExecutorReachability's own SQL (see the Persistence section of the
// design) — store's job is narrower: pick InitialState/InitialFailures for a
// never-before-seen row (the insert-only path), and determine whether the
// write actually landed and whether the resulting state changed by reading
// the record before and after the call.
type store struct {
	repo Repository
	log  *logger.Logger
}

// Observe records one probe outcome for executor. checkedAt is the probe's
// own completion timestamp, supplied by the caller so a test can control it
// precisely (see the design's last-write-wins contract).
func (s *store) Observe(ctx context.Context, executor *models.Executor, outcome agentruntime.SSHProbeOutcome, checkedAt time.Time) observeResult {
	before, _ := s.repo.GetExecutorReachability(ctx, executor.ID)

	obs := buildObservation(executor, outcome, checkedAt)
	recordProbeOutcome(outcome.Success, string(obs.Reason))
	if err := s.repo.UpsertExecutorReachability(ctx, obs); err != nil {
		s.log.Warn("executor ssh reachability: write failed",
			zap.String("executor_id", executor.ID), zap.Error(err))
		return observeResult{}
	}

	after, err := s.repo.GetExecutorReachability(ctx, executor.ID)
	if err != nil {
		s.log.Warn("executor ssh reachability: read-back after write failed",
			zap.String("executor_id", executor.ID), zap.Error(err))
		return observeResult{}
	}

	// The upsert's own WHERE (eligibility, last-write-wins on checked_at) may
	// have silently dropped this write. Comparing the stored checked_at back
	// against the one this observation carried is how we find out — the SQL
	// itself never reports it.
	if after.CheckedAt == nil || !after.CheckedAt.Equal(checkedAt) {
		writeRefusedTotal.Add(1)
		s.log.Debug("executor ssh reachability: write refused",
			zap.String("executor_id", executor.ID))
		return observeResult{Current: after.State}
	}

	previous := models.ExecutorReachabilityStateUnknown
	var previousReason models.ExecutorReachabilityReason
	if before != nil {
		previous = before.State
		previousReason = before.Reason
	}
	stateChanged := previous != after.State
	if stateChanged {
		s.logTransition(executor, previous, after.State, obs.Reason, after.ConsecutiveFailures)
		recordStateTransition(string(after.State))
	}
	changed := stateChanged || previousReason != after.Reason
	return observeResult{
		Applied: true, StateChanged: stateChanged, Changed: changed,
		Previous: previous, Current: after.State, After: after,
	}
}

// logTransition logs at Warn going into unreachable and Info coming out of
// it, per the design's Observability section. Unchanged results never reach
// here — Observe only calls this when the state actually changed.
func (s *store) logTransition(executor *models.Executor, previous, current models.ExecutorReachabilityState, reason models.ExecutorReachabilityReason, failures int) {
	fields := []zap.Field{
		zap.String("executor_id", executor.ID),
		zap.String("host", executor.Config["ssh_host"]),
		zap.String("from", string(previous)),
		zap.String("to", string(current)),
		zap.String("reason", string(reason)),
		zap.Int("consecutive_failures", failures),
	}
	if current == models.ExecutorReachabilityStateUnreachable {
		s.log.Warn("executor ssh reachability state transition", fields...)
		return
	}
	s.log.Info("executor ssh reachability state transition", fields...)
}

// buildObservation projects a probe outcome into the upsert's input. An
// empty Reason means success. InitialState/InitialFailures matter only on
// the insert path (no prior row); every later write derives both in SQL
// from the stored row instead:
//   - success: reachable, zero counter.
//   - a sticky reason (config, host_key): unreachable immediately, whatever
//     the counter — both are deterministic and would repeat, so waiting out
//     the threshold buys no confidence.
//   - any other failure: the counter starts at 1, which is below
//     failureThreshold, so the state stays unknown — a record that doesn't
//     exist yet is never promoted by a single below-threshold failure, the
//     same rule the SQL enforces for an existing row.
func buildObservation(executor *models.Executor, outcome agentruntime.SSHProbeOutcome, checkedAt time.Time) models.ExecutorReachabilityObservation {
	reason := models.ExecutorReachabilityReason(outcome.Reason)
	initialState := models.ExecutorReachabilityStateUnknown
	initialFailures := 0
	switch {
	case outcome.Success:
		initialState = models.ExecutorReachabilityStateReachable
	case isStickyReason(reason):
		initialState = models.ExecutorReachabilityStateUnreachable
		initialFailures = 1
	default:
		initialFailures = 1
		if initialFailures >= failureThreshold {
			initialState = models.ExecutorReachabilityStateUnreachable
		}
	}
	return models.ExecutorReachabilityObservation{
		ExecutorID:       executor.ID,
		SeenUpdatedAt:    executor.UpdatedAt,
		InitialState:     initialState,
		InitialFailures:  initialFailures,
		Reason:           reason,
		Message:          outcome.Message,
		Host:             outcome.Host,
		CheckedAt:        checkedAt,
		FailureThreshold: failureThreshold,
	}
}

func isStickyReason(reason models.ExecutorReachabilityReason) bool {
	return reason == models.ExecutorReachabilityReasonConfig || reason == models.ExecutorReachabilityReasonHostKey
}
