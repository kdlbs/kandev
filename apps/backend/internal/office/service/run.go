package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/pause"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// Run reason constants.
const (
	RunReasonTaskAssigned          = "task_assigned"
	RunReasonTaskComment           = "task_comment"
	RunReasonTaskBlockersResolved  = "task_blockers_resolved"
	RunReasonTaskChildrenCompleted = "task_children_completed"
	RunReasonApprovalResolved      = "approval_resolved"
	RunReasonTaskReviewRequested   = "task_review_requested"
	RunReasonTaskChangesRequested  = "task_changes_requested"
	RunReasonRoutineTrigger        = "routine_trigger"
	// RunReasonHeartbeat aliases shared.RunReasonHeartbeat so this package's
	// local constant and the shared idle-skip classifier cannot drift apart
	// the way the un-aliased pair did before WO-46 (Review round 1, S2).
	RunReasonHeartbeat   = shared.RunReasonHeartbeat
	RunReasonBudgetAlert = "budget_alert"
	RunReasonAgentError  = "agent_error"
)

// These reasons were persisted by earlier workflow templates. Keep them
// readable so existing materialized workflows continue to wake the correct
// prompt after the built-in template changes.
const (
	legacyRunReasonBlockersResolved  = "blockers_resolved"
	legacyRunReasonChildrenCompleted = "children_completed"
	legacyRunReasonReviewStarted     = "review_started"
	legacyRunReasonApprovalStarted   = "approval_started"
)

// Run status constants.
const (
	RunStatusQueued    = "queued"
	RunStatusClaimed   = "claimed"
	RunStatusFinished  = "finished"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

// Run outcome constants (docs/specs/task-delivery-ledger/spec.md, "Office run
// outcome"). Written into runs.outcome alongside status='finished' at each of
// the eight terminal call sites (the original six, plus the pause gate's
// early and final checks in scheduler_integration.go); NULL on the failed
// path and on every pre-activation row. RunOutcomeProcessed is the only
// value RunCountsByDayForAgent counts as succeeded. Every other value
// buckets into skipped.
const (
	RunOutcomeProcessed          = "processed"
	RunOutcomeBudgetBlocked      = "budget_blocked"
	RunOutcomeIdleSkipped        = "idle_skipped"
	RunOutcomeAgentInactive      = "agent_inactive"
	RunOutcomeTaskTreeHeld       = "task_tree_held"
	RunOutcomeBudgetUnmeasurable = "budget_unmeasurable"
	RunOutcomeWorkspacePaused    = "workspace_paused"
)

// CoalesceWindowSeconds is the default coalescing window.
const CoalesceWindowSeconds = 5

// IdempotencyWindowHours is the deduplication window.
const IdempotencyWindowHours = 24

// QueueRun enqueues a run request for an agent instance.
// It checks agent status, idempotency, and attempts coalescing before inserting.
//
// When a runs service is wired (via SetRunsService) the insert +
// publish + scheduler signal are delegated to it so the engine and
// office paths share one queue implementation. The agent status guard
// stays here because it depends on office-specific tables.
func (s *Service) QueueRun(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	agent, err := s.guardAgentStatus(ctx, agentInstanceID)
	if err != nil {
		return err
	}
	if err := s.checkPauseGateForAgent(ctx, agent, "queue_run"); err != nil {
		return err
	}

	if s.runsService != nil {
		_, err := s.runsService.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:         reason,
			IdempotencyKey: idempotencyKey,
			Payload:        payloadWithAgent(payload, agentInstanceID),
		})
		return err
	}
	return s.queueRunInline(ctx, agentInstanceID, reason, payload, idempotencyKey)
}

// queueRunInline performs the legacy in-office insert path used when
// no runs service is wired (older tests, transitional deployments).
// Behaviour matches the pre-Phase-3 implementation.
func (s *Service) queueRunInline(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	if idempotencyKey != "" {
		dup, err := s.repo.CheckIdempotencyKey(ctx, idempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			s.logger.Debug("run skipped (idempotent)",
				zap.String("key", idempotencyKey))
			return nil
		}
	}

	coalesced, err := s.repo.CoalesceRun(ctx, agentInstanceID, reason, CoalesceWindowSeconds, payload)
	if err != nil {
		return fmt.Errorf("coalesce check: %w", err)
	}
	if coalesced {
		s.logger.Debug("run coalesced",
			zap.String("agent", agentInstanceID),
			zap.String("reason", reason))
		return nil
	}

	var idemKeyPtr *string
	if idempotencyKey != "" {
		idemKeyPtr = &idempotencyKey
	}
	req := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: agentInstanceID,
		Reason:         reason,
		Payload:        payload,
		Status:         RunStatusQueued,
		CoalescedCount: 1,
		IdempotencyKey: idemKeyPtr,
		RequestedAt:    time.Now().UTC(),
	}
	if err := s.repo.CreateRun(ctx, req); err != nil {
		return fmt.Errorf("enqueue run: %w", err)
	}

	s.logger.Info("run queued",
		zap.String("id", req.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", reason))

	s.publishRunQueued(ctx, req, idempotencyKey)
	return nil
}

// payloadWithAgent decodes the JSON payload string and adds the
// agent_profile_id field so the runs service can resolve the
// instance without a separate resolver. The runs queue's payload
// column is JSON, so re-injecting the field here keeps the row shape
// identical to the legacy office.QueueRun insert.
func payloadWithAgent(payload, agentInstanceID string) map[string]any {
	out := map[string]any{}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &out)
	}
	out["agent_profile_id"] = agentInstanceID
	return out
}

// publishRunQueued emits an OfficeRunQueued bus event so the WS
// gateway can fan it out to subscribed clients. Defensive: skips when
// the bus is not configured. Publish errors are logged at debug and
// swallowed — the queue write already succeeded by the time we get
// here.
func (s *Service) publishRunQueued(ctx context.Context, req *models.Run, idempotencyKey string) {
	if s.eb == nil {
		return
	}
	taskID, commentID := commentkeys.IdentityFromPayload(req.Payload)
	data := map[string]interface{}{
		"run_id":           req.ID,
		"agent_profile_id": req.AgentProfileID,
		"reason":           req.Reason,
		"task_id":          taskID,
		"comment_id":       commentID,
		"idempotency_key":  idempotencyKey,
	}
	event := bus.NewEvent(events.OfficeRunQueued, "office-service", data)
	if err := s.eb.Publish(ctx, events.OfficeRunQueued, event); err != nil {
		s.logger.Debug("publish run queued event failed",
			zap.String("run_id", req.ID),
			zap.Error(err))
	}
}

// guardAgentStatus returns an error if the agent is paused or stopped,
// and otherwise the resolved agent — callers that also need the pause
// gate's workspace scope (checkPauseGateForAgent) reuse this fetch instead
// of looking the agent up a second time.
func (s *Service) guardAgentStatus(ctx context.Context, agentInstanceID string) (*models.AgentInstance, error) {
	agent, err := s.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return nil, fmt.Errorf("get agent instance: %w", err)
	}
	switch agent.Status {
	case models.AgentStatusPaused:
		return nil, fmt.Errorf("agent %s is paused", agentInstanceID)
	case models.AgentStatusStopped:
		return nil, fmt.Errorf("agent %s is stopped", agentInstanceID)
	case models.AgentStatusPendingApproval:
		return nil, fmt.Errorf("agent %s is pending approval", agentInstanceID)
	}
	return agent, nil
}

// pauseGateState reads the workspace-pause gate directly (by workspace
// id, not agent id — used by scheduler_integration.go's run-processing
// gates, which already have the agent and its WorkspaceID in hand).
// The returned bool is true only for a confirmed pause; err is non-nil
// only on a gate-read failure. When s.pauseGate is nil (not wired) it
// always reports (false, nil) so dispatch proceeds ungated.
func (s *Service) pauseGateState(ctx context.Context, workspaceID string) (bool, error) {
	if s.pauseGate == nil {
		return false, nil
	}
	active, err := s.pauseGate.PauseState(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return active != nil, nil
}

// checkPauseGateForAgent blocks queuing when agent's workspace is paused
// (the operator kill switch). Takes the already-resolved agent — usually
// guardAgentStatus's return value — rather than re-resolving it, so the
// two checks can never see two different snapshots of the agent's
// workspace. Fails closed on a gate-read error
// (shared.ErrPauseGateUnavailable) — this write hasn't happened yet, so
// there is nothing to leave in a retryable state beyond simply not writing
// it; the caller's own retry (or the next event) tries again.
func (s *Service) checkPauseGateForAgent(ctx context.Context, agent *models.AgentInstance, gateName string) error {
	if s.pauseGate == nil {
		return nil
	}
	active, err := s.pauseGate.PauseState(ctx, agent.WorkspaceID)
	if err != nil {
		pause.RecordGateError(gateName)
		s.logger.Warn("queue run: pause gate read failed",
			zap.String("agent", agent.ID), zap.Error(err))
		return shared.ErrPauseGateUnavailable
	}
	if active != nil {
		pause.RecordBlocked(gateName)
		return shared.ErrWorkspacePaused
	}
	return nil
}
