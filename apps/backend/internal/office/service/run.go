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
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// Run reason constants.
const (
	RunReasonTaskAssigned          = "task_assigned"
	RunReasonTaskComment           = "task_comment"
	RunReasonTaskBlockersResolved  = "task_blockers_resolved"
	RunReasonTaskChildrenCompleted = "task_children_completed"
	RunReasonApprovalResolved      = "approval_resolved"
	RunReasonRoutineTrigger        = "routine_trigger"
	RunReasonHeartbeat             = "heartbeat"
	RunReasonBudgetAlert           = "budget_alert"
	RunReasonAgentError            = "agent_error"
)

// Run status constants.
const (
	RunStatusQueued    = "queued"
	RunStatusClaimed   = "claimed"
	RunStatusFinished  = "finished"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
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
	if err := s.guardAgentStatus(ctx, agentInstanceID); err != nil {
		return err
	}

	if s.runsService != nil {
		return s.runsService.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:         reason,
			IdempotencyKey: idempotencyKey,
			Payload:        payloadWithAgent(payload, agentInstanceID),
		})
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
	parsed := ParseRunPayload(req.Payload)
	data := map[string]interface{}{
		"run_id":           req.ID,
		"agent_profile_id": req.AgentProfileID,
		"reason":           req.Reason,
		"task_id":          parsed["task_id"],
		"comment_id":       parsed["comment_id"],
		"idempotency_key":  idempotencyKey,
	}
	event := bus.NewEvent(events.OfficeRunQueued, "office-service", data)
	if err := s.eb.Publish(ctx, events.OfficeRunQueued, event); err != nil {
		s.logger.Debug("publish run queued event failed",
			zap.String("run_id", req.ID),
			zap.Error(err))
	}
}

// guardAgentStatus returns an error if the agent is paused or stopped.
func (s *Service) guardAgentStatus(ctx context.Context, agentInstanceID string) error {
	agent, err := s.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return fmt.Errorf("get agent instance: %w", err)
	}
	switch agent.Status {
	case models.AgentStatusPaused:
		return fmt.Errorf("agent %s is paused", agentInstanceID)
	case models.AgentStatusStopped:
		return fmt.Errorf("agent %s is stopped", agentInstanceID)
	case models.AgentStatusPendingApproval:
		return fmt.Errorf("agent %s is pending approval", agentInstanceID)
	}
	return nil
}
