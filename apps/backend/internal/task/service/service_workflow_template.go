package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/stepevents"
)

const (
	workflowTemplateImproveKandev     = "improve-kandev"
	workflowTemplateReportKandevIssue = "report-kandev-issue"
	workflowTemplateStepEventSource   = "task-service"
)

var errWorkflowTemplateIDRequired = errors.New("workflow ID is required")

// EnsureWorkflowTemplateSteps repairs an empty Improve Kandev workflow using
// its persisted template binding. Populated workflows are left unchanged.
func (s *Service) EnsureWorkflowTemplateSteps(ctx context.Context, workflowID, templateID string) error {
	if workflowID == "" {
		return errWorkflowTemplateIDRequired
	}
	if err := validateImproveKandevTemplateID(templateID); err != nil {
		return err
	}

	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	if err := s.authorizeWorkflowTemplateRepair(ctx, workflow, workflowID, templateID); err != nil {
		return err
	}

	lister, err := s.workflowTemplateStepLister()
	if err != nil {
		return err
	}
	steps, err := lister.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("list workflow template steps: %w", err)
	}
	if len(steps) > 0 {
		return nil
	}
	if s.workflowStepCreator == nil {
		return fmt.Errorf("workflow step creator is not configured")
	}
	if err := s.workflowStepCreator.CreateStepsFromTemplate(ctx, workflowID, templateID); err != nil {
		return fmt.Errorf("repair workflow template steps: %w", err)
	}
	steps, err = lister.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("list repaired workflow template steps: %w", err)
	}
	if len(steps) == 0 {
		return fmt.Errorf("workflow template %q created no steps", templateID)
	}
	stepevents.NewPublisher(s.eventBus, workflowTemplateStepEventSource, s.logger).
		PublishAll(ctx, events.WorkflowStepCreated, steps)
	return nil
}

func validateImproveKandevTemplateID(templateID string) error {
	switch templateID {
	case workflowTemplateImproveKandev, workflowTemplateReportKandevIssue:
		return nil
	default:
		return fmt.Errorf("workflow template %q is not managed by Improve Kandev", templateID)
	}
}

func (s *Service) authorizeWorkflowTemplateRepair(
	ctx context.Context,
	workflow *models.Workflow,
	workflowID, templateID string,
) error {
	if workflow == nil || workflow.WorkspaceID == "" {
		return fmt.Errorf("workflow %q has no workspace", workflowID)
	}
	if err := s.AuthorizeWorkspaceScope(ctx, workflow.WorkspaceID, authz.ScopeRepositoryManage); err != nil {
		return err
	}
	if workflow.Source == models.WorkflowSourceGitHub {
		return fmt.Errorf("workflow %q is managed by GitHub sync", workflowID)
	}
	if workflow.WorkflowTemplateID == nil || *workflow.WorkflowTemplateID != templateID {
		return fmt.Errorf("workflow %q is not bound to template %q", workflowID, templateID)
	}
	return nil
}

func (s *Service) workflowTemplateStepLister() (workflowStepLister, error) {
	lister, ok := s.workflowStepGetter.(workflowStepLister)
	if !ok {
		return nil, fmt.Errorf("workflow step lister is not configured")
	}
	return lister, nil
}
