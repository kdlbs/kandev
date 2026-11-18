package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/models"
)

// ErrSeatStepNotInWorkspace rejects a change for a step whose workflow belongs
// to another workspace.
var ErrSeatStepNotInWorkspace = errors.New("workflow step does not belong to this workspace")

// ErrSeatRoleNotFound rejects a change for a role the step has no
// ensure_participant_seat action for.
var ErrSeatRoleNotFound = errors.New("workflow step has no participant seat for this role")

// SeatAuthorExclusionChange turns exclude_author_provider on or off for one
// step's ensure_participant_seat action for Role.
type SeatAuthorExclusionChange struct {
	StepID  string
	Role    string
	Enabled bool
}

// SetSeatAuthorExclusion applies changes to steps of workspaceID's workflows
// and returns the updated steps. Every change is authorized and validated
// before the first write, so a rejected batch changes nothing. Callers publish
// the returned steps as workflow step updates.
func (s *Service) SetSeatAuthorExclusion(
	ctx context.Context, workspaceID string, changes []SeatAuthorExclusionChange,
) ([]*models.WorkflowStep, error) {
	pending := make([]*models.WorkflowStep, 0, len(changes))
	byID := map[string]*models.WorkflowStep{}
	for _, change := range changes {
		step, err := s.seatStepForChange(ctx, workspaceID, change.StepID, byID)
		if err != nil {
			return nil, err
		}
		events, found := models.SetExcludeAuthorProvider(step.Events, change.Role, change.Enabled)
		if !found {
			return nil, fmt.Errorf("%w: step %s, role %q", ErrSeatRoleNotFound, change.StepID, change.Role)
		}
		if _, seen := byID[step.ID]; !seen {
			pending = append(pending, step)
		}
		step.Events = events
		byID[step.ID] = step
	}
	for _, step := range pending {
		if _, err := s.UpdateStepWithStartStepIntent(ctx, step, nil); err != nil {
			return nil, fmt.Errorf("update workflow step %s: %w", step.ID, err)
		}
	}
	return pending, nil
}

// seatStepForChange loads stepID once per batch after checking that the caller
// may see it, that its workflow is in workspaceID, and that the workflow is
// mutable.
func (s *Service) seatStepForChange(
	ctx context.Context, workspaceID, stepID string, loaded map[string]*models.WorkflowStep,
) (*models.WorkflowStep, error) {
	if step, ok := loaded[stepID]; ok {
		return step, nil
	}
	if err := s.AuthorizeStep(ctx, stepID); err != nil {
		return nil, err
	}
	step, err := s.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	if s.workflowProvider == nil {
		return nil, fmt.Errorf("%w: step %s", ErrSeatStepNotInWorkspace, stepID)
	}
	wf, err := s.workflowProvider.GetWorkflow(ctx, step.WorkflowID)
	if err != nil || wf == nil || wf.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: step %s", ErrSeatStepNotInWorkspace, stepID)
	}
	if err := s.EnsureWorkflowMutable(ctx, step.WorkflowID); err != nil {
		return nil, err
	}
	return step, nil
}
