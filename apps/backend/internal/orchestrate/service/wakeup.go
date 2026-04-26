package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// Wakeup reason constants.
const (
	WakeupReasonTaskAssigned          = "task_assigned"
	WakeupReasonTaskComment           = "task_comment"
	WakeupReasonTaskBlockersResolved  = "task_blockers_resolved"
	WakeupReasonTaskChildrenCompleted = "task_children_completed"
	WakeupReasonApprovalResolved      = "approval_resolved"
	WakeupReasonRoutineTrigger        = "routine_trigger"
	WakeupReasonHeartbeat             = "heartbeat"
	WakeupReasonBudgetAlert           = "budget_alert"
	WakeupReasonAgentError            = "agent_error"
)

// Wakeup status constants.
const (
	WakeupStatusQueued   = "queued"
	WakeupStatusClaimed  = "claimed"
	WakeupStatusFinished = "finished"
	WakeupStatusFailed   = "failed"
)

// CoalesceWindowSeconds is the default coalescing window.
const CoalesceWindowSeconds = 5

// IdempotencyWindowHours is the deduplication window.
const IdempotencyWindowHours = 24

// QueueWakeup enqueues a wakeup request for an agent instance.
// It checks agent status, idempotency, and attempts coalescing before inserting.
func (s *Service) QueueWakeup(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	if err := s.guardAgentStatus(ctx, agentInstanceID); err != nil {
		return err
	}

	if idempotencyKey != "" {
		dup, err := s.repo.CheckIdempotencyKey(ctx, idempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			s.logger.Debug("wakeup skipped (idempotent)",
				zap.String("key", idempotencyKey))
			return nil
		}
	}

	coalesced, err := s.repo.CoalesceWakeup(ctx, agentInstanceID, reason, CoalesceWindowSeconds, payload)
	if err != nil {
		return fmt.Errorf("coalesce check: %w", err)
	}
	if coalesced {
		s.logger.Debug("wakeup coalesced",
			zap.String("agent", agentInstanceID),
			zap.String("reason", reason))
		return nil
	}

	var idemKeyPtr *string
	if idempotencyKey != "" {
		idemKeyPtr = &idempotencyKey
	}
	req := &models.WakeupRequest{
		ID:              uuid.New().String(),
		AgentInstanceID: agentInstanceID,
		Reason:          reason,
		Payload:         payload,
		Status:          WakeupStatusQueued,
		CoalescedCount:  1,
		IdempotencyKey:  idemKeyPtr,
		RequestedAt:     time.Now().UTC(),
	}
	if err := s.repo.CreateWakeupRequest(ctx, req); err != nil {
		return fmt.Errorf("enqueue wakeup: %w", err)
	}

	s.logger.Info("wakeup queued",
		zap.String("id", req.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", reason))
	return nil
}

// guardAgentStatus returns an error if the agent is paused or stopped.
func (s *Service) guardAgentStatus(ctx context.Context, agentInstanceID string) error {
	agent, err := s.repo.GetAgentInstance(ctx, agentInstanceID)
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
