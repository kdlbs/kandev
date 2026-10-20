package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// OfficeApprovalGuard reports the approvers whose decision still blocks an
// Office task from completing. Implemented in backendapp over the Office
// dashboard so this package does not import it.
type OfficeApprovalGuard interface {
	PendingApproversForCompletion(ctx context.Context, taskID string) ([]PendingApprover, error)
}

// PendingApprover names one approver without a current approval.
type PendingApprover struct {
	AgentProfileID string
	Name           string
}

// ApprovalsPendingMoveError refuses a human move that would complete an
// Office task while approvers are pending. Nothing has been written.
type ApprovalsPendingMoveError struct {
	Pending []PendingApprover
}

func (e *ApprovalsPendingMoveError) Error() string {
	names := make([]string, 0, len(e.Pending))
	for _, p := range e.Pending {
		if p.Name != "" {
			names = append(names, p.Name)
		} else {
			names = append(names, p.AgentProfileID)
		}
	}
	return fmt.Sprintf("task approvals pending: %s", strings.Join(names, ", "))
}

// SetOfficeApprovalGuard wires the Office approver read consulted before a
// human move completes an Office task. Optional; when unset, moves complete
// without consulting approvers.
func (s *Service) SetOfficeApprovalGuard(guard OfficeApprovalGuard) {
	s.officeApprovalGuard = guard
}

// guardOfficeHumanCompletion refuses a human move that would complete an
// Office task while any approver is pending. It runs before the move writes
// anything, so a refusal leaves the task untouched.
func (s *Service) guardOfficeHumanCompletion(
	ctx context.Context, task *models.Task, nextState v1.TaskState, opts MoveTaskOptions,
) error {
	if s.officeApprovalGuard == nil || !task.IsFromOffice ||
		opts.StepHistoryActor != wfmodels.StepTransitionActorHuman ||
		nextState != v1.TaskStateCompleted || task.State == v1.TaskStateCompleted {
		return nil
	}
	pending, err := s.officeApprovalGuard.PendingApproversForCompletion(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("resolve pending approvers: %w", err)
	}
	if len(pending) > 0 {
		return &ApprovalsPendingMoveError{Pending: pending}
	}
	return nil
}
