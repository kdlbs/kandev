package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/engine"
	"go.uber.org/zap"
)

// decisionSeatDispatcher is the additive capability HoldsDecisionSeat needs
// from s.engineDispatcher. Named locally and reached via a type assertion
// rather than widening shared.WorkflowEngineDispatcher, mirroring
// dashboard.roleResolvingDispatcher. HoldsDecisionSeat only observes seat
// occupancy — it neither evaluates a guard nor records a decision — so it
// asserts on the read-only variant rather than ResolveParticipantRole:
// the recording variant would emit the unresolved-agent quorum counter and
// warning log, which are scoped to guard evaluations. A capability grant
// runs once per run launch, not per guard evaluation, so it must use the
// read-only variant to avoid inflating those metrics at an unrelated
// cadence.
type decisionSeatDispatcher interface {
	ResolveParticipantRoleReadOnly(ctx context.Context, taskID, stepID, agentProfileID string) (role, participantID string, err error)
}

// HoldsDecisionSeat reports whether agentProfileID currently holds a
// decision seat (reviewer or approver) at the task's current workflow step.
// It resolves the seat the same way RecordAgentDecision authorizes a real
// record_step_decision_kandev call, so runtime.ContextBuilder can grant the
// matching capability without over- or under-stating who can actually
// decide.
func (s *Service) HoldsDecisionSeat(ctx context.Context, taskID, agentProfileID string) (bool, error) {
	if taskID == "" || agentProfileID == "" {
		return false, nil
	}
	dispatcher, ok := s.engineDispatcher.(decisionSeatDispatcher)
	if !ok {
		return false, nil
	}
	stepID, err := s.repo.GetTaskWorkflowStepID(ctx, taskID)
	if err != nil {
		s.logger.Warn("resolve task workflow_step_id failed, denying decision seat",
			zap.String("task_id", taskID), zap.String("agent_profile_id", agentProfileID), zap.Error(err))
		return false, fmt.Errorf("resolve task workflow_step_id: %w", err)
	}
	if stepID == "" {
		return false, nil
	}
	_, _, err = dispatcher.ResolveParticipantRoleReadOnly(ctx, taskID, stepID, agentProfileID)
	if err != nil {
		if errors.Is(err, engine.ErrParticipantNotFound) {
			return false, nil
		}
		s.logger.Warn("resolve participant role failed, denying decision seat",
			zap.String("task_id", taskID), zap.String("agent_profile_id", agentProfileID), zap.Error(err))
		return false, fmt.Errorf("resolve participant role: %w", err)
	}
	return true, nil
}
