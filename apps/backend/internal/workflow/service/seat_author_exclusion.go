package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
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

// seatExclusionWriteAttempts bounds retries when another writer changes a step
// between the read and the guarded write.
const seatExclusionWriteAttempts = 3

// SetSeatAuthorExclusion applies changes to steps of workspaceID's workflows
// and returns the updated steps. Every change is authorized and validated
// before anything is written, and the steps are written in one transaction
// that only succeeds if none of them changed after they were read, so a
// rejected or failed batch changes nothing. When another writer got there
// first, the batch is re-read and re-applied. Callers publish the returned
// steps as workflow step updates.
func (s *Service) SetSeatAuthorExclusion(
	ctx context.Context, workspaceID string, changes []SeatAuthorExclusionChange,
) ([]*models.WorkflowStep, error) {
	var err error
	for attempt := 0; attempt < seatExclusionWriteAttempts; attempt++ {
		var steps []*models.WorkflowStep
		steps, err = s.applySeatAuthorExclusion(ctx, workspaceID, changes)
		if !errors.Is(err, models.ErrWorkflowStepChanged) {
			return steps, err
		}
	}
	return nil, err
}

func (s *Service) applySeatAuthorExclusion(
	ctx context.Context, workspaceID string, changes []SeatAuthorExclusionChange,
) ([]*models.WorkflowStep, error) {
	pending, err := s.prepareSeatAuthorExclusion(ctx, workspaceID, changes)
	if err != nil {
		return nil, err
	}
	updates := make([]repository.StepEventsUpdate, 0, len(pending))
	for _, step := range pending {
		updates = append(updates, repository.StepEventsUpdate{
			StepID: step.ID, WorkflowID: step.WorkflowID, Events: step.Events, ExpectedUpdatedAt: step.UpdatedAt,
		})
	}
	saved, err := s.repo.UpdateStepEventsIfUnchanged(ctx, updates)
	if err != nil {
		return nil, err
	}
	for i, version := range saved {
		pending[i].UpdatedAt = version.UpdatedAt
	}
	return pending, nil
}

// prepareSeatAuthorExclusion loads each changed step once, applies its
// changes in memory, and runs every check a step write needs.
func (s *Service) prepareSeatAuthorExclusion(
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
		if err := models.ValidateWorkflowStep(step); err != nil {
			return nil, err
		}
	}
	return pending, nil
}

// seatStepForChange loads stepID once per batch after checking that the caller
// may see it, that its workflow is in workspaceID, and that the workflow and
// its executor allow the change.
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
	if err := s.authorizeExecutorMutation(ctx, step.WorkflowID); err != nil {
		return nil, err
	}
	return step, nil
}
