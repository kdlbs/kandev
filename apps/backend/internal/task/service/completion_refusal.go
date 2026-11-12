package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// PendingApprover names one approver without a current approval.
type PendingApprover struct {
	AgentProfileID string
	Name           string
}

// ApproverNameResolver adds display names to stable approver IDs returned by
// a completion policy.
type ApproverNameResolver interface {
	ResolvePendingApproverNames(ctx context.Context, ids []string) []PendingApprover
}

type finalStepCompletionRepository interface {
	CompleteTaskOnFinalStep(ctx context.Context, id string, admission *completion.Admission) (*models.Task, v1.TaskState, bool, error)
}

// ApprovalsPendingMoveError describes the approvers that refused a human
// completion. The policy itself returns only stable IDs.
type ApprovalsPendingMoveError struct {
	Pending []PendingApprover
}

func (e *ApprovalsPendingMoveError) Error() string {
	names := make([]string, 0, len(e.Pending))
	for _, approver := range e.Pending {
		if approver.Name != "" {
			names = append(names, approver.Name)
		} else {
			names = append(names, approver.AgentProfileID)
		}
	}
	return fmt.Sprintf("task approvals pending: %s", strings.Join(names, ", "))
}

func (s *Service) SetApproverNameResolver(resolver ApproverNameResolver) {
	s.approverNameResolver = resolver
}

func (s *Service) translateCompletionError(ctx context.Context, err error) error {
	var refused *completion.RefusedError
	if !errors.As(err, &refused) || refused.Code != completion.CodeApprovalsPending {
		return err
	}
	pending := make([]PendingApprover, 0, len(refused.Subjects))
	for _, id := range refused.Subjects {
		pending = append(pending, PendingApprover{AgentProfileID: id, Name: id})
	}
	if s.approverNameResolver != nil {
		pending = s.approverNameResolver.ResolvePendingApproverNames(ctx, refused.Subjects)
	}
	return &ApprovalsPendingMoveError{Pending: pending}
}

// UpdateTaskAsHuman binds the caller's authorized placement to a field mutation.
func (s *Service) UpdateTaskAsHuman(ctx context.Context, id string, req *UpdateTaskRequest) (*models.Task, error) {
	task, err := s.officeHumanCompletionTaskSnapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	kind := completion.Fields
	if req.State != nil || req.WorkflowStepID != nil {
		kind = completion.SetState
	}
	updated, err := s.updateTask(ctx, id, req, humanCompletionAdmission(task, kind))
	return updated, s.translateCompletionError(ctx, err)
}

// CompleteOfficeTaskAsHuman applies the Office Done action to the current
// final step through the same transaction that evaluates completion policies.
func (s *Service) CompleteOfficeTaskAsHuman(ctx context.Context, id, expectedStepID string) (*models.Task, error) {
	task, err := s.officeHumanCompletionTaskSnapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.WorkflowStepID != expectedStepID {
		return nil, repoerrors.ErrWorkflowChangeConflict
	}
	repo, ok := s.tasks.(finalStepCompletionRepository)
	if !ok {
		return nil, fmt.Errorf("final-step completion repository not configured")
	}
	updated, previousState, changed, err := repo.CompleteTaskOnFinalStep(ctx, id, humanCompletionAdmission(task, completion.FinalStepDone))
	if err != nil {
		return nil, s.translateCompletionError(ctx, err)
	}
	if changed {
		s.PublishTaskStateChanged(ctx, updated, previousState)
	}
	return updated, nil
}

// officeHumanCompletionTaskSnapshot binds the existing authorization decision
// to the same task scope that the mutation transaction will fence.
func (s *Service) officeHumanCompletionTaskSnapshot(ctx context.Context, id string) (*models.Task, error) {
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, repoerrors.ErrTaskNotFound
	}
	_, err = s.requireWorkspaceScope(ctx, task.WorkspaceID, authz.ScopeTaskWrite, repoerrors.ErrTaskNotFound)
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		if s.collaborationApplies(ctx) {
			return nil, repoerrors.ErrTaskNotFound
		}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}
