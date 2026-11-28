package service

import (
	"context"
	"slices"

	"github.com/kandev/kandev/internal/events"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepevents"
)

// SetPromptSourceEventPublishers connects direct reconciliation to the same
// lifecycle publishers used by ordinary workflow and step mutations.
func (s *Service) SetPromptSourceEventPublishers(workflows func(context.Context, string, *taskmodels.Workflow), steps *stepevents.Publisher) {
	s.promptSourceWorkflowPublisher = workflows
	s.promptSourceStepPublisher = steps
}

func (s *Service) publishPromptSourceChanges(ctx context.Context, plans []models.PromptSourcePlan) {
	for _, plan := range plans {
		if len(plan.ChangedFields) == 0 {
			continue
		}
		if s.promptSourceWorkflowPublisher != nil {
			if plan.ExpectedWorkflow == nil {
				s.promptSourceWorkflowPublisher(ctx, events.WorkflowCreated, plan.Workflow)
			} else if slices.Contains(plan.ChangedFields, "workflow.definition") || slices.Contains(plan.ChangedFields, "workflow.source") {
				s.promptSourceWorkflowPublisher(ctx, events.WorkflowUpdated, plan.Workflow)
			}
		}
		s.publishPromptSourceSteps(ctx, plan)
	}
}

// Publish from the committed plan, including removed owners that cannot be
// reloaded. Retire old start selections before announcing their replacements.
func (s *Service) publishPromptSourceSteps(ctx context.Context, plan models.PromptSourcePlan) {
	oldSteps := make(map[string]*models.WorkflowStep, len(plan.ExistingSteps))
	for _, step := range plan.ExistingSteps {
		oldSteps[step.ID] = step
	}
	var demoted, updated, created []*models.WorkflowStep
	for _, step := range plan.Steps {
		if !slices.Contains(plan.ChangedFields, "steps."+step.Name) {
			continue
		}
		old := oldSteps[step.ID]
		switch {
		case old == nil:
			created = append(created, step)
		case old.IsStartStep && !step.IsStartStep:
			demoted = append(demoted, step)
		default:
			updated = append(updated, step)
		}
	}
	s.promptSourceStepPublisher.PublishAll(ctx, events.WorkflowStepDeleted, plan.Removed)
	s.promptSourceStepPublisher.PublishAll(ctx, events.WorkflowStepUpdated, demoted)
	s.promptSourceStepPublisher.PublishAll(ctx, events.WorkflowStepUpdated, updated)
	s.promptSourceStepPublisher.PublishAll(ctx, events.WorkflowStepCreated, created)
}
