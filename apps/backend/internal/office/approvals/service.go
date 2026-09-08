package approvals

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

// Repo is the persistence interface required by ApprovalService.
type Repo interface {
	CreateApproval(ctx context.Context, approval *Approval) error
	GetApproval(ctx context.Context, id string) (*Approval, error)
	ListApprovals(ctx context.Context, workspaceID string) ([]*Approval, error)
	ListPendingApprovals(ctx context.Context, workspaceID string) ([]*Approval, error)
	UpdateApproval(ctx context.Context, approval *Approval) error
}

// AgentWriter activates agents by updating their status.
type AgentWriter interface {
	UpdateAgentStatusFields(ctx context.Context, agentID, status, pauseReason string) error
}

// RunQueuer enqueues run requests for agent instances, attributed to a
// typed actor (AC-OFFICE-RUN-CAUSATION-001.15).
type RunQueuer interface {
	QueueRunWithActor(
		ctx context.Context,
		agentInstanceID, reason, payload, idempotencyKey string,
		actorKind models.ActorKind, actorID string,
	) error
}

// ApprovalService handles approval CRUD and decide logic.
type ApprovalService struct {
	repo        Repo
	logger      *logger.Logger
	activity    shared.ActivityLogger
	runs        RunQueuer
	agentWriter AgentWriter
}

// NewApprovalService constructs an ApprovalService.
func NewApprovalService(
	repo Repo,
	log *logger.Logger,
	activity shared.ActivityLogger,
	runs RunQueuer,
) *ApprovalService {
	return &ApprovalService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "approvals-service")),
		activity: activity,
		runs:     runs,
	}
}

// SetAgentWriter wires in an agent writer for activating agents on approval.
func (s *ApprovalService) SetAgentWriter(w AgentWriter) {
	s.agentWriter = w
}

// CreateApprovalWithActivity creates a new approval, logs the activity, and
// returns the created approval.
func (s *ApprovalService) CreateApprovalWithActivity(ctx context.Context, approval *Approval) error {
	if approval.Status == "" {
		approval.Status = StatusPending
	}
	if err := s.repo.CreateApproval(ctx, approval); err != nil {
		return fmt.Errorf("create approval: %w", err)
	}

	s.activity.LogActivity(ctx, approval.WorkspaceID, "system", approval.RequestedByAgentProfileID,
		"approval.created", "approval", approval.ID,
		fmt.Sprintf(`{"type":%q}`, approval.Type))

	s.logger.Info("approval created",
		zap.String("approval_id", approval.ID),
		zap.String("type", approval.Type))
	return nil
}

// DecideApproval resolves an approval and performs side effects based on the
// approval type: activating agents, moving tasks, creating skills, and
// queuing runs for the requesting agent. actorKind identifies who decidedBy
// actually is (an authenticated agent, or a human/UI caller) so the queued
// approval_resolved run carries a real actor instead of a hardcoded default.
func (s *ApprovalService) DecideApproval(
	ctx context.Context,
	approvalID, status, decidedBy string,
	actorKind models.ActorKind,
	note string,
) (*Approval, error) {
	if status != StatusApproved && status != StatusRejected {
		return nil, fmt.Errorf("invalid status: %s (must be approved or rejected)", status)
	}

	approval, err := s.repo.GetApproval(ctx, approvalID)
	if err != nil {
		return nil, fmt.Errorf("get approval: %w", err)
	}
	if approval.Status != StatusPending {
		return nil, fmt.Errorf("approval already decided: %s", approval.Status)
	}

	now := time.Now().UTC()
	approval.Status = models.ApprovalStatus(status)
	approval.DecisionNote = note
	approval.DecidedBy = decidedBy
	approval.DecidedAt = &now

	if err := s.repo.UpdateApproval(ctx, approval); err != nil {
		return nil, fmt.Errorf("update approval: %w", err)
	}

	if err := s.applyApprovalSideEffects(ctx, approval, actorKind); err != nil {
		s.logger.Error("approval side effects failed",
			zap.String("approval_id", approvalID),
			zap.Error(err))
		return approval, fmt.Errorf("approval committed but side effect failed: %w", err)
	}

	s.activity.LogActivity(ctx, approval.WorkspaceID, "user", decidedBy,
		"approval.resolved", "approval", approval.ID,
		fmt.Sprintf(`{"status":%q,"type":%q}`, status, approval.Type))

	s.logger.Info("approval decided",
		zap.String("approval_id", approvalID),
		zap.String("status", status))
	return approval, nil
}

// GetApproval returns an approval by ID.
func (s *ApprovalService) GetApproval(ctx context.Context, id string) (*Approval, error) {
	return s.repo.GetApproval(ctx, id)
}

// GetPendingApprovals returns all pending approvals for a workspace.
func (s *ApprovalService) GetPendingApprovals(ctx context.Context, wsID string) ([]*Approval, error) {
	return s.repo.ListPendingApprovals(ctx, wsID)
}

// ListApprovals returns all approvals for a workspace.
func (s *ApprovalService) ListApprovals(ctx context.Context, wsID string) ([]*Approval, error) {
	return s.repo.ListApprovals(ctx, wsID)
}

// applyApprovalSideEffects handles type-specific logic after an approval is
// decided. Each case is intentionally simple; complex orchestration belongs
// in the scheduler/run layer.
func (s *ApprovalService) applyApprovalSideEffects(
	ctx context.Context, approval *Approval, actorKind models.ActorKind,
) error {
	if approval.Status == StatusRejected {
		if approval.Type == ApprovalTypeHireAgent {
			return s.onHireAgentRejected(ctx, approval, actorKind)
		}
		return s.queueApprovalRun(ctx, approval, actorKind)
	}

	switch approval.Type {
	case ApprovalTypeHireAgent:
		return s.onHireAgentApproved(ctx, approval, actorKind)
	case ApprovalTypeTaskReview:
		// Task state transitions are handled by the orchestrator via runs.
		return s.queueApprovalRun(ctx, approval, actorKind)
	case ApprovalTypeSkillCreation:
		// Skill creation from approval payload is handled via run.
		return s.queueApprovalRun(ctx, approval, actorKind)
	default:
		return s.queueApprovalRun(ctx, approval, actorKind)
	}
}

func (s *ApprovalService) onHireAgentApproved(
	ctx context.Context, approval *Approval, actorKind models.ActorKind,
) error {
	agentID := extractAgentProfileID(approval.Payload)
	if agentID != "" && s.agentWriter != nil {
		if err := s.agentWriter.UpdateAgentStatusFields(ctx, agentID, "idle", ""); err != nil {
			s.logger.Warn("failed to activate agent on hire approval",
				zap.String("agent_id", agentID),
				zap.Error(err))
		}
	}
	return s.queueApprovalRun(ctx, approval, actorKind)
}

func (s *ApprovalService) onHireAgentRejected(
	ctx context.Context, approval *Approval, actorKind models.ActorKind,
) error {
	agentID := extractAgentProfileID(approval.Payload)
	if agentID != "" && s.agentWriter != nil {
		if err := s.agentWriter.UpdateAgentStatusFields(ctx, agentID, "stopped", "hire rejected"); err != nil {
			s.logger.Warn("failed to stop agent on hire rejection",
				zap.String("agent_id", agentID),
				zap.Error(err))
		}
	}
	return s.queueApprovalRun(ctx, approval, actorKind)
}

// extractAgentProfileID attempts to parse an agent_profile_id from the approval payload JSON.
func extractAgentProfileID(payload string) string {
	if payload == "" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return ""
	}
	id, _ := m["agent_profile_id"].(string)
	return id
}

func (s *ApprovalService) queueApprovalRun(
	ctx context.Context, approval *Approval, actorKind models.ActorKind,
) error {
	if approval.RequestedByAgentProfileID == "" {
		return nil
	}
	payload := fmt.Sprintf(
		`{"approval_id":%q,"type":%q,"status":%q,"note":%q}`,
		approval.ID, approval.Type, approval.Status, approval.DecisionNote,
	)
	idempotencyKey := "approval:" + approval.ID
	return s.runs.QueueRunWithActor(ctx, approval.RequestedByAgentProfileID,
		"approval_resolved", payload, idempotencyKey, actorKind, approval.DecidedBy)
}
