package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	runsmodels "github.com/kandev/kandev/internal/runs/models"
	"go.uber.org/zap"
)

// CeilingDeferredLaunchReplayed settles the Office run whose deferred launch
// the session ceiling just admitted. The ceiling owns the launch; this only
// binds the resulting session to the run and releases the deferral park so the
// ordinary lifecycle events can finish it. It never launches or retries.
func (s *Service) CeilingDeferredLaunchReplayed(
	ctx context.Context, taskID, causingRunID, sessionID, turnID string,
) error {
	return s.settleCeilingDeferredRun(ctx, taskID, causingRunID, sessionID, turnID, "")
}

// settleCeilingDeferredRun is the shared exact-row settlement. taskID always
// scopes the update to the run's own task; agentProfileID adds the lifecycle
// event's agent identity when the ready-event recovery path supplies it.
func (s *Service) settleCeilingDeferredRun(
	ctx context.Context, taskID, causingRunID, sessionID, turnID, agentProfileID string,
) error {
	if causingRunID == "" {
		return nil
	}
	wrote, err := s.repo.SettleCeilingDeferredRun(ctx, causingRunID, sessionID, taskID, agentProfileID)
	if err != nil {
		return fmt.Errorf("settle ceiling-deferred run %s: %w", causingRunID, err)
	}
	if !wrote {
		s.logger.Debug("ceiling replay: causing run is no longer parked",
			zap.String("run_id", causingRunID),
			zap.String("task_id", taskID))
		return nil
	}
	s.AppendRunEvent(ctx, causingRunID, "adapter.invoke", "info", map[string]interface{}{
		"phase":      "launch_reconciled",
		"reason":     "session_ceiling_replay",
		"task_id":    taskID,
		"session_id": sessionID,
		"turn_id":    turnID,
	})
	s.logger.Info("office run settled from ceiling replay",
		zap.String("run_id", causingRunID),
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
	return nil
}

func lifecycleCausingRunID(data AgentLifecycleData) string {
	if data.TerminalCausingRunID != "" {
		return data.TerminalCausingRunID
	}
	return data.CausingRunID
}

func lifecycleEventRunID(data AgentLifecycleData) string {
	if data.RunID != "" {
		return data.RunID
	}
	return lifecycleCausingRunID(data)
}

// resolveTerminalLifecycleRun uses the launch's retained identity before the
// legacy task-and-agent lookup. A delayed terminal event must not settle a
// successor run on the same task and agent.
func (s *Service) resolveTerminalLifecycleRun(
	ctx context.Context, data AgentLifecycleData,
) (*runsmodels.Run, error) {
	if data.RunID != "" {
		return s.resolveLifecycleRun(ctx, data)
	}
	causingRunID := lifecycleCausingRunID(data)
	if causingRunID == "" {
		return s.resolveLifecycleRun(ctx, data)
	}
	data.RunID = causingRunID
	return s.resolveLifecycleRun(ctx, data)
}

// recoverCeilingDeferredRunOnFailure restores a replayed run to the normal
// claimed-run failure path when the launch fails before its ready event can
// settle the ceiling deferral.
func (s *Service) recoverCeilingDeferredRunOnFailure(
	ctx context.Context, data AgentLifecycleData,
) (*runsmodels.Run, error) {
	runID := lifecycleCausingRunID(data)
	if runID == "" || data.TaskID == "" || data.AgentProfileID == "" {
		return nil, sql.ErrNoRows
	}
	deferred, err := s.isCeilingDeferredRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if !deferred {
		return nil, sql.ErrNoRows
	}
	if err := s.settleCeilingDeferredRun(
		ctx, data.TaskID, runID, data.SessionID, data.TurnID, data.AgentProfileID,
	); err != nil {
		return nil, err
	}
	data.RunID = runID
	return s.resolveLifecycleRun(ctx, data)
}

// settleCeilingDeferredLaunchOnReady is the event-driven recovery path for a
// ceiling replay whose synchronous settlement write failed. The replayed
// launch's ready event carries the same Office causing run id, so the parked
// row can be settled without a second launch. A normal task-owned launch is
// already claimed and this is a no-op.
func (s *Service) settleCeilingDeferredLaunchOnReady(
	ctx context.Context, data *AgentLifecycleData,
) error {
	if data == nil || data.TaskID == "" || data.CausingRunID == "" || data.SessionID == "" {
		return nil
	}
	deferred, err := s.isCeilingDeferredRun(ctx, data.CausingRunID)
	if err != nil {
		return err
	}
	if !deferred {
		return nil
	}
	return s.settleCeilingDeferredRun(
		ctx, data.TaskID, data.CausingRunID, data.SessionID, data.TurnID, data.AgentProfileID,
	)
}

// finishCeilingDeferredRunOnCompletion handles the ordering where the
// replayed launch's completion event arrives before the ready recovery has
// claimed the run. It finishes the exact parked row atomically; if the ready
// recovery won the race, it returns false so the caller retries the normal
// claimed-run path instead of leaving a claimed run with no later event.
func (s *Service) finishCeilingDeferredRunOnCompletion(
	ctx context.Context, data *AgentLifecycleData,
) (bool, error) {
	if data == nil || data.TaskID == "" || data.AgentProfileID == "" || data.SessionID == "" || data.ErrorMessage != "" {
		return false, nil
	}
	runID := lifecycleCausingRunID(*data)
	if runID == "" {
		return false, nil
	}
	run, err := s.repo.GetRunByID(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if run.AgentProfileID != data.AgentProfileID || taskIDFromRunPayload(run.Payload) != data.TaskID {
		return false, nil
	}
	deferred, err := s.isCeilingDeferredRun(ctx, run.ID)
	if err != nil {
		return false, err
	}
	if !deferred {
		return false, nil
	}
	finished, err := s.repo.FinishCeilingDeferredRunOnCompletion(
		ctx, run.ID, data.SessionID, data.TaskID, data.AgentProfileID, RunOutcomeProcessed,
	)
	if err != nil {
		return false, err
	}
	if finished == nil {
		return false, nil
	}
	outcome := RunOutcomeProcessed
	s.AppendRunEvent(ctx, finished.ID, "complete", "info", map[string]interface{}{
		"task_id":    data.TaskID,
		"session_id": data.SessionID,
	})
	s.recordTerminalShape(ctx, finished, RunStatusFinished, &outcome)
	s.publishRunProcessed(ctx, finished.ID, RunStatusFinished, finished)
	s.markRoutingSuccess(ctx, finished)
	s.recordRunOutputSummary(ctx, finished, *data)
	s.warnIfReviewDecisionMissing(ctx, finished)
	s.releaseTaskCheckoutForRun(ctx, finished)
	s.stampRunFinished(ctx, finished)
	s.logger.Info("office run finished from ceiling replay completion",
		zap.String("run_id", finished.ID),
		zap.String("task_id", data.TaskID),
		zap.String("session_id", data.SessionID))
	return true, nil
}

// isCeilingDeferredRun reports whether the run's latest event is the durable
// ceiling-deferral marker. The ready recovery accepts only this marker, so a
// normal launch's ready event cannot release a run parked for another reason.
func (s *Service) isCeilingDeferredRun(ctx context.Context, runID string) (bool, error) {
	run, err := s.repo.GetRunByID(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if string(run.Status) != RunStatusQueued || run.RoutingBlockedStatus == nil ||
		*run.RoutingBlockedStatus != runsmodels.RoutingBlockedActionRequired {
		return false, nil
	}
	events, err := s.repo.ListRunEvents(ctx, runID, -1, 0)
	if err != nil {
		return false, err
	}
	if len(events) == 0 {
		return false, nil
	}
	latest := events[len(events)-1]
	if string(latest.EventType) != "adapter.invoke" {
		return false, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(latest.Payload), &payload); err != nil {
		return false, nil
	}
	return payload["phase"] == "deferred" && payload["reason"] == "session_ceiling", nil
}

// CeilingDeferredLaunchDropped fails the Office run whose deferred launch the
// ceiling terminally dropped, since no session will ever be created for it.
// A run another path already settled is left untouched.
func (s *Service) CeilingDeferredLaunchDropped(
	ctx context.Context, taskID, causingRunID, reasonCode, detail string,
) error {
	if causingRunID == "" {
		return nil
	}
	message := fmt.Sprintf("the queued launch was dropped: %s", detail)
	run, err := s.repo.FailCeilingDeferredRun(ctx, causingRunID, message, taskID)
	if err != nil {
		return fmt.Errorf("fail ceiling-deferred run %s: %w", causingRunID, err)
	}
	if run == nil {
		s.logger.Debug("ceiling drop: causing run is no longer parked",
			zap.String("run_id", causingRunID),
			zap.String("task_id", taskID),
			zap.String("reason_code", reasonCode))
		return nil
	}
	s.AppendRunEvent(ctx, run.ID, "error", "error", map[string]interface{}{
		runEventFieldErrorMessage: message,
		"reason_code":             reasonCode,
		"task_id":                 taskID,
	})
	s.recordTerminalShape(ctx, run, RunStatusFailed, nil)
	s.publishRunProcessed(ctx, run.ID, RunStatusFailed, run)
	s.logger.Info("office run failed from ceiling drop",
		zap.String("run_id", run.ID),
		zap.String("task_id", taskID),
		zap.String("reason_code", reasonCode))
	return nil
}
