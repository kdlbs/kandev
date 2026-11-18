package backendapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/routing"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

// exclusionSetupEvaluator is the routing.Provider subset the import advisor
// needs.
type exclusionSetupEvaluator interface {
	ExclusionSetup(ctx context.Context, workspaceID string, decls []routing.ExclusionDeclaration) ([]routing.SetupWarning, error)
}

// importExclusionAdvisorAdapter satisfies workflowservice.ImportExclusionAdvisor
// over Office routing, so workflow import and sync can warn about
// exclude_author_provider setups without the workflow system importing Office.
type importExclusionAdvisorAdapter struct {
	setup exclusionSetupEvaluator
}

// AdviseExclusionSetup implements workflowservice.ImportExclusionAdvisor.
func (a *importExclusionAdvisorAdapter) AdviseExclusionSetup(
	ctx context.Context, workspaceID string, decls []workflowmodels.AuthorExclusionDeclaration,
) ([]workflowservice.ImportWarning, error) {
	routingDecls := make([]routing.ExclusionDeclaration, 0, len(decls))
	for _, d := range decls {
		routingDecls = append(routingDecls, routing.ExclusionDeclaration{
			WorkflowName: d.WorkflowName, StepName: d.StepName, Role: d.Role,
		})
	}
	warnings, err := a.setup.ExclusionSetup(ctx, workspaceID, routingDecls)
	if err != nil {
		return nil, err
	}
	out := make([]workflowservice.ImportWarning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, workflowservice.ImportWarning{
			Code:         string(w.Code),
			WorkflowName: w.Declaration.WorkflowName,
			StepName:     w.Declaration.StepName,
			Role:         w.Declaration.Role,
			ProviderID:   string(w.ProviderID),
			AgentName:    w.AgentName,
		})
	}
	return out, nil
}

// seatExclusionSetter is the workflow service write used by the routing
// settings page's Review on a different provider switches.
type seatExclusionSetter interface {
	SetSeatAuthorExclusion(ctx context.Context, workspaceID string,
		changes []workflowservice.SeatAuthorExclusionChange) ([]*workflowmodels.WorkflowStep, error)
}

// stepUpdatePublisher publishes workflow step lifecycle events.
type stepUpdatePublisher interface {
	PublishAll(ctx context.Context, eventType string, steps []*workflowmodels.WorkflowStep)
}

// seatExclusionWriterAdapter satisfies dashboard.SeatAuthorExclusionWriter
// over the workflow service, so the Office dashboard writes steps through the
// same authorization and mutability checks as every other step mutation, and
// publishes the same workflow_step.updated events. A failed batch writes
// nothing, so there is nothing to publish.
type seatExclusionWriterAdapter struct {
	workflow  seatExclusionSetter
	publisher stepUpdatePublisher
}

// SetSeatAuthorExclusion implements dashboard.SeatAuthorExclusionWriter.
func (a *seatExclusionWriterAdapter) SetSeatAuthorExclusion(
	ctx context.Context, workspaceID string, changes []dashboard.SeatAuthorExclusionChange,
) error {
	wfChanges := make([]workflowservice.SeatAuthorExclusionChange, 0, len(changes))
	for _, ch := range changes {
		wfChanges = append(wfChanges, workflowservice.SeatAuthorExclusionChange{
			StepID: ch.StepID, Role: ch.Role, Enabled: ch.Enabled,
		})
	}
	steps, err := a.workflow.SetSeatAuthorExclusion(ctx, workspaceID, wfChanges)
	if err != nil {
		return seatChangeError(err)
	}
	a.publisher.PublishAll(ctx, events.WorkflowStepUpdated, steps)
	return nil
}

// seatChangeError wraps a workflow service error in the dashboard sentinel
// that selects its HTTP status. A step in another workspace reads exactly
// like one the caller cannot see.
func seatChangeError(err error) error {
	switch {
	case errors.Is(err, workflowservice.ErrSeatRoleNotFound):
		return fmt.Errorf("%w: %w", dashboard.ErrSeatChangeInvalid, err)
	case errors.Is(err, workflowservice.ErrSeatStepNotInWorkspace), errors.Is(err, workflowservice.ErrNotVisible):
		return fmt.Errorf("%w: %w", dashboard.ErrSeatChangeNotFound, err)
	case errors.Is(err, workflowservice.ErrWorkflowReadOnly), errors.Is(err, workflowservice.ErrWorkflowWorkspaceReadOnly):
		return fmt.Errorf("%w: %w", dashboard.ErrSeatChangeReadOnly, err)
	case errors.Is(err, workflowmodels.ErrWorkflowStepChanged):
		return fmt.Errorf("%w: %w", dashboard.ErrSeatChangeConflict, err)
	default:
		return err
	}
}
