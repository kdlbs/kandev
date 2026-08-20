package service

import (
	"context"
	"database/sql"
	"errors"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/runs/commentkeys"
)

// ClaimNextRun atomically claims the next eligible run from the queue.
// Returns nil, nil if no run is available.
func (s *Service) ClaimNextRun(ctx context.Context) (*models.Run, error) {
	req, err := s.repo.ClaimNextEligibleRun(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.logger.Info("run claimed",
		zap.String("id", req.ID),
		zap.String("agent", req.AgentProfileID),
		zap.String("reason", req.Reason))
	return req, nil
}

// FinishRun marks a claimed run as finished and publishes an
// OfficeRunProcessed bus event. The run row is fetched first so the
// published payload carries enough context (agent, task, comment,
// reason) for downstream WS consumers to scope updates.
func (s *Service) FinishRun(ctx context.Context, id string) error {
	return s.transitionRunTerminal(ctx, id, RunStatusFinished)
}

// FailRun marks a claimed run as failed and publishes an
// OfficeRunProcessed event. See FinishRun for the lifecycle contract.
func (s *Service) FailRun(ctx context.Context, id string) error {
	return s.transitionRunTerminal(ctx, id, RunStatusFailed)
}

// transitionRunTerminal updates the run row to the given terminal
// status and emits OfficeRunProcessed. Pre-fetching the run keeps the
// published payload self-contained even when the caller doesn't hold a
// reference to the model. Publish errors are logged at debug and
// swallowed; persistence errors are returned to the caller.
//
// Deliberately does NOT release the task checkout: transitionRunTerminal
// is reached by every terminal run, including ones that never held the
// checkout in the first place (the "agent not active" / idle-skip /
// tree-gated / checkout-contended-error branches in processRun, all of
// which finish a run before it ever reaches checkoutTask). Owner-scoping
// releaseTaskCheckoutForRun by run.AgentProfileID guards a DIFFERENT
// agent's live lock, but not the case where a second, pre-checkout run
// for the SAME agent + task races a first run that is genuinely still
// executing and holds the checkout — that second run's release matches
// the first run's own checkout_agent_id and steals its own live lock out
// from under it (Review round 3, BLOCKING FINDING 1). Callers that KNOW
// their run actually held the checkout call releaseTaskCheckoutForRun
// explicitly instead: handleAgentCompleted / handleTasklessAgentCompleted
// (event_subscribers.go) for the launched-run completion path, and
// HandleAgentFailure (failure.go) for the launched-run failure path.
func (s *Service) transitionRunTerminal(ctx context.Context, id, status string) error {
	run, getErr := s.repo.GetRunByID(ctx, id)
	if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
		s.logger.Debug("get run for terminal transition failed",
			zap.String("run_id", id),
			zap.String("status", status),
			zap.Error(getErr))
	}
	if err := s.repo.FinishRun(ctx, id, status); err != nil {
		return err
	}
	s.publishRunProcessed(ctx, id, status, run)
	return nil
}

// releaseTaskCheckoutForRun releases the run's task checkout. Call this
// only when the caller knows run genuinely held the checkout (a launched
// run that reached checkoutTask) — see transitionRunTerminal's doc for why
// it is not called unconditionally on every terminal transition. Safe to
// call redundantly alongside SchedulerIntegration's own synchronous
// finishRun/checkBudget call sites (releaseCheckoutIfNeeded): idempotent,
// owner-scoped SET NULL.
//
// Owner-scoped by run ID and run.AgentProfileID: a run that never
// held the checkout (the loser of a checkout-contention race escalating via
// escalateFailure, or a run for an agent that turned out to be
// inactive/idle and never reached the checkout attempt in processRun) must
// not clear a different, currently-active agent's lock out from under it —
// that inverted the invariant this whole release path exists to protect.
func (s *Service) releaseTaskCheckoutForRun(ctx context.Context, run *models.Run) {
	if run == nil {
		return
	}
	taskID := taskIDFromRunPayload(run.Payload)
	if taskID == "" {
		return
	}
	if err := s.repo.ReleaseTaskCheckoutForRun(ctx, taskID, run.AgentProfileID, run.ID); err != nil {
		s.logger.Error("failed to release task checkout on terminal transition",
			zap.String("run_id", run.ID),
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

// publishRunProcessed emits an OfficeRunProcessed bus event with the
// per-run context the WS gateway needs to fan the update out.
// Best-effort: skips silently when no bus is configured.
func (s *Service) publishRunProcessed(
	ctx context.Context, id, status string, run *models.Run,
) {
	if s.eb == nil {
		return
	}
	data := map[string]interface{}{
		"run_id": id,
		"status": status,
	}
	if run != nil {
		taskID, commentID := commentkeys.IdentityFromPayload(run.Payload)
		data["agent_profile_id"] = run.AgentProfileID
		data["reason"] = run.Reason
		data["task_id"] = taskID
		data["comment_id"] = commentID
		if run.ErrorMessage != "" {
			data["error_message"] = run.ErrorMessage
		}
	}
	event := bus.NewEvent(events.OfficeRunProcessed, "office-service", data)
	if err := s.eb.Publish(ctx, events.OfficeRunProcessed, event); err != nil {
		s.logger.Debug("publish run processed event failed",
			zap.String("run_id", id),
			zap.Error(err))
	}
}

// ProcessRunGuard checks if the agent is still eligible to be woken.
// Returns true if the run should proceed, false if it should be skipped.
func (s *Service) ProcessRunGuard(ctx context.Context, run *models.Run) (bool, error) {
	agent, err := s.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		return false, err
	}
	switch agent.Status {
	case models.AgentStatusPaused, models.AgentStatusStopped, models.AgentStatusPendingApproval:
		s.logger.Info("run skipped (agent not active)",
			zap.String("run_id", run.ID),
			zap.String("agent_status", string(agent.Status)))
		return false, nil
	}
	return true, nil
}
