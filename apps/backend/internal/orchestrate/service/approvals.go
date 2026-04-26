package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
)

// CreateApprovalWithActivity creates a new approval, logs the activity, and
// returns the created approval.
func (s *Service) CreateApprovalWithActivity(ctx context.Context, approval *models.Approval) error {
	if approval.Status == "" {
		approval.Status = "pending"
	}
	if err := s.repo.CreateApproval(ctx, approval); err != nil {
		return fmt.Errorf("create approval: %w", err)
	}

	s.LogActivity(ctx, approval.WorkspaceID, "system", approval.RequestedByAgentInstanceID,
		"approval.created", "approval", approval.ID,
		fmt.Sprintf(`{"type":%q}`, approval.Type))

	s.logger.Info("approval created",
		zap.String("approval_id", approval.ID),
		zap.String("type", approval.Type))
	return nil
}

// DecideApproval resolves an approval and performs side effects based on the
// approval type: activating agents, moving tasks, creating skills, and
// queuing wakeups for the requesting agent.
func (s *Service) DecideApproval(
	ctx context.Context,
	approvalID, status, decidedBy, note string,
) (*models.Approval, error) {
	if status != "approved" && status != "rejected" {
		return nil, fmt.Errorf("invalid status: %s (must be approved or rejected)", status)
	}

	approval, err := s.repo.GetApproval(ctx, approvalID)
	if err != nil {
		return nil, fmt.Errorf("get approval: %w", err)
	}
	if approval.Status != "pending" {
		return nil, fmt.Errorf("approval already decided: %s", approval.Status)
	}

	now := time.Now().UTC()
	approval.Status = status
	approval.DecisionNote = note
	approval.DecidedBy = decidedBy
	approval.DecidedAt = &now

	if err := s.repo.UpdateApproval(ctx, approval); err != nil {
		return nil, fmt.Errorf("update approval: %w", err)
	}

	if err := s.applyApprovalSideEffects(ctx, approval); err != nil {
		s.logger.Error("approval side effects failed",
			zap.String("approval_id", approvalID),
			zap.Error(err))
	}

	s.LogActivity(ctx, approval.WorkspaceID, "user", decidedBy,
		"approval.resolved", "approval", approval.ID,
		fmt.Sprintf(`{"status":%q,"type":%q}`, status, approval.Type))

	s.logger.Info("approval decided",
		zap.String("approval_id", approvalID),
		zap.String("status", status))
	return approval, nil
}

// applyApprovalSideEffects handles type-specific logic after an approval is
// decided. Each case is intentionally simple; complex orchestration belongs
// in the scheduler/wakeup layer.
func (s *Service) applyApprovalSideEffects(
	ctx context.Context, approval *models.Approval,
) error {
	if approval.Status == "rejected" {
		return s.queueApprovalWakeup(ctx, approval)
	}

	switch approval.Type {
	case models.ApprovalTypeHireAgent:
		return s.onHireAgentApproved(ctx, approval)
	case models.ApprovalTypeTaskReview:
		// Task state transitions are handled by the orchestrator via wakeups.
		return s.queueApprovalWakeup(ctx, approval)
	case models.ApprovalTypeSkillCreation:
		// Skill creation from approval payload is handled via wakeup.
		return s.queueApprovalWakeup(ctx, approval)
	default:
		return s.queueApprovalWakeup(ctx, approval)
	}
}

func (s *Service) onHireAgentApproved(ctx context.Context, approval *models.Approval) error {
	if approval.RequestedByAgentInstanceID == "" {
		return s.queueApprovalWakeup(ctx, approval)
	}
	// The actual agent instance creation happens via the wakeup; here we just
	// queue the wakeup so the requesting agent can proceed.
	return s.queueApprovalWakeup(ctx, approval)
}

func (s *Service) queueApprovalWakeup(ctx context.Context, approval *models.Approval) error {
	if approval.RequestedByAgentInstanceID == "" {
		return nil
	}
	payload := fmt.Sprintf(
		`{"approval_id":%q,"type":%q,"status":%q,"note":%q}`,
		approval.ID, approval.Type, approval.Status, approval.DecisionNote,
	)
	idempotencyKey := "approval:" + approval.ID
	return s.QueueWakeup(ctx, approval.RequestedByAgentInstanceID,
		"approval_resolved", payload, idempotencyKey)
}

// GetPendingApprovals returns all pending approvals for a workspace.
func (s *Service) GetPendingApprovals(ctx context.Context, wsID string) ([]*models.Approval, error) {
	return s.repo.ListPendingApprovals(ctx, wsID)
}
