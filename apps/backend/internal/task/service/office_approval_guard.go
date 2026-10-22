package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// OfficeApprovalGuard reports the approvers whose decision still blocks an
// Office task from completing on targetStepID. Implemented in backendapp over
// the Office dashboard so this package does not import it.
type OfficeApprovalGuard interface {
	PendingApproversForCompletion(ctx context.Context, taskID, targetStepID string) ([]PendingApprover, error)
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
	ctx context.Context, task *models.Task, targetStepID string, nextState v1.TaskState, opts MoveTaskOptions,
) error {
	if s.atomicOfficeCompletionConfigured() || s.officeApprovalGuard == nil || !task.IsFromOffice ||
		opts.StepHistoryActor != wfmodels.StepTransitionActorHuman ||
		nextState != v1.TaskStateCompleted || task.State == v1.TaskStateCompleted {
		return nil
	}
	pending, err := s.officeApprovalGuard.PendingApproversForCompletion(ctx, task.ID, targetStepID)
	if err != nil {
		return fmt.Errorf("resolve pending approvers: %w", err)
	}
	if len(pending) > 0 {
		return &ApprovalsPendingMoveError{Pending: pending}
	}
	return nil
}

func (s *Service) atomicOfficeCompletionConfigured() bool {
	r, ok := s.tasks.(interface{ OfficeHumanCompletionGuardConfigured() bool })
	return ok && r.OfficeHumanCompletionGuardConfigured()
}
func (s *Service) translateOfficeCompletionError(ctx context.Context, err error) error {
	var pending *repoerrors.OfficeApprovalsPendingError
	if !errors.As(err, &pending) {
		return err
	}
	result := make([]PendingApprover, 0, len(pending.AgentProfileIDs))
	for _, id := range pending.AgentProfileIDs {
		result = append(result, PendingApprover{AgentProfileID: id, Name: id})
	}
	if resolver, ok := s.officeApprovalGuard.(interface {
		ResolvePendingApproverNames(context.Context, []string) []PendingApprover
	}); ok {
		result = resolver.ResolvePendingApproverNames(ctx, pending.AgentProfileIDs)
	}
	return &ApprovalsPendingMoveError{Pending: result}
}

// UpdateTaskAsHuman binds the caller's authorized placement to a field mutation.
func (s *Service) UpdateTaskAsHuman(ctx context.Context, id string, req *UpdateTaskRequest) (*models.Task, error) {
	task, err := s.officeHumanCompletionTaskSnapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	kind := "fields"
	if req.State != nil || req.WorkflowStepID != nil {
		kind = "set_state"
	}
	ctx = models.WithOfficeHumanCompletionIntent(ctx, models.OfficeHumanCompletionIntent{TaskID: id, ExpectedWorkspaceID: task.WorkspaceID, ExpectedWorkflowID: task.WorkflowID, ExpectedStepID: task.WorkflowStepID, Kind: kind})
	updated, err := s.UpdateTask(ctx, id, req)
	return updated, s.translateOfficeCompletionError(ctx, err)
}

// CompleteOfficeTaskAsHuman preserves Office's explicit scalar completion policy.
func (s *Service) CompleteOfficeTaskAsHuman(ctx context.Context, id, expectedStepID string) (*models.Task, error) {
	task, err := s.officeHumanCompletionTaskSnapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.WorkflowStepID != expectedStepID {
		return nil, repoerrors.ErrWorkflowChangeConflict
	}
	repo, ok := s.tasks.(interface {
		CompleteOfficeTaskAsHuman(context.Context, string, string, string, string) (*models.Task, error)
	})
	if !ok {
		return nil, fmt.Errorf("office human completion repository not configured")
	}
	updated, err := repo.CompleteOfficeTaskAsHuman(ctx, id, task.WorkspaceID, task.WorkflowID, expectedStepID)
	return updated, s.translateOfficeCompletionError(ctx, err)
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
	// Match authorizeTaskScope's existing durable-cleanup workspace grace.
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

// persistHumanTaskState keeps optional legacy stores usable without allowing a
// configured production guard to fall back to an unguarded write.
func (s *Service) persistHumanTaskState(ctx context.Context, task *models.Task, state v1.TaskState) error {
	if err := s.guardOfficeHumanCompletion(ctx, task, task.WorkflowStepID, state,
		MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorHuman}); err != nil {
		return err
	}
	if repo, ok := s.tasks.(interface {
		UpdateTaskStateAsHuman(context.Context, string, v1.TaskState) (*models.Task, error)
	}); ok {
		_, err := repo.UpdateTaskStateAsHuman(ctx, task.ID, state)
		return err
	}
	if s.atomicOfficeCompletionConfigured() {
		return fmt.Errorf("human task state repository not configured")
	}
	return s.tasks.UpdateTaskState(ctx, task.ID, state)
}
