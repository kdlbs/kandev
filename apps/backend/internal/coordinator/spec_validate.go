package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Proposal spec field length limits, in Unicode code points after trimming
// leading and trailing Unicode whitespace
// (docs/specs/coordinator/requirements/proposals.md, AC-COORDINATOR-
// PROPOSALS-001.3).
const (
	proposalTitleMaxRunes       = 60
	proposalDescriptionMaxRunes = 10000
	proposalRationaleMaxRunes   = 10000
)

// validateProposalSpec implements AC-COORDINATOR-PROPOSALS-001.3: trims
// title, description and rationale and checks their lengths; confirms
// workflow_id, repository_id (when set) and source_task_id (when set)
// resolve to a row in workspaceID; and confirms step_id belongs to
// workflow_id and is an eligible step
// (docs/specs/coordinator/system-design/proposals.md#no-agent-starts).
// Returns the trimmed spec on success. The caller (propose, or approve's
// edit merge) is responsible for resolving an omitted/cleared step_id to the
// workflow's start step before calling this — an empty step_id here is
// treated as invalid, not defaulted.
//
// s.decisionTasks and s.decisionSteps must be wired via SetDecisionDeps
// before this is called.
func (s *Service) validateProposalSpec(ctx context.Context, workspaceID string, spec ProposalSpec) (ProposalSpec, error) {
	title := strings.TrimSpace(spec.Title)
	if n := utf8.RuneCountInString(title); n == 0 || n > proposalTitleMaxRunes {
		return ProposalSpec{}, &FieldError{
			Field:   "title",
			Message: fmt.Sprintf("title must be 1 to %d characters", proposalTitleMaxRunes),
		}
	}
	description := strings.TrimSpace(spec.Description)
	if utf8.RuneCountInString(description) > proposalDescriptionMaxRunes {
		return ProposalSpec{}, &FieldError{
			Field:   "description",
			Message: fmt.Sprintf("description must be at most %d characters", proposalDescriptionMaxRunes),
		}
	}
	rationale := strings.TrimSpace(spec.Rationale)
	if utf8.RuneCountInString(rationale) > proposalRationaleMaxRunes {
		return ProposalSpec{}, &FieldError{
			Field:   "rationale",
			Message: fmt.Sprintf("rationale must be at most %d characters", proposalRationaleMaxRunes),
		}
	}

	if spec.WorkflowID == "" {
		return ProposalSpec{}, &FieldError{Field: ApproveFieldWorkflowID, Message: "workflow_id is required"}
	}
	if err := s.validateWorkflowInWorkspace(ctx, workspaceID, spec.WorkflowID); err != nil {
		return ProposalSpec{}, err
	}
	if err := s.validateStepEligible(ctx, spec.WorkflowID, spec.StepID); err != nil {
		return ProposalSpec{}, err
	}
	if spec.RepositoryID != "" {
		if err := s.validateRepositoryInWorkspace(ctx, workspaceID, spec.RepositoryID); err != nil {
			return ProposalSpec{}, err
		}
	}
	if spec.SourceTaskID != "" {
		if err := s.validateSourceTaskInWorkspace(ctx, workspaceID, spec.SourceTaskID); err != nil {
			return ProposalSpec{}, err
		}
	}

	return ProposalSpec{
		Title:        title,
		Description:  description,
		Rationale:    rationale,
		WorkflowID:   spec.WorkflowID,
		StepID:       spec.StepID,
		RepositoryID: spec.RepositoryID,
		SourceTaskID: spec.SourceTaskID,
	}, nil
}

// validateWorkflowInWorkspace confirms workflowID exists and belongs to
// workspaceID. GetWorkflow's own authorization is identity-scoped, not
// workspaceID-scoped, so a caller with access to a different workspace's
// workflow would otherwise pass; the explicit WorkspaceID comparison closes
// that gap.
func (s *Service) validateWorkflowInWorkspace(ctx context.Context, workspaceID, workflowID string) error {
	workflow, err := s.decisionTasks.GetWorkflow(ctx, workflowID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return &FieldError{Field: ApproveFieldWorkflowID, Message: "workflow not found"}
		}
		return err
	}
	if workflow == nil || workflow.WorkspaceID != workspaceID {
		return &FieldError{Field: ApproveFieldWorkflowID, Message: "workflow not found"}
	}
	return nil
}

// validateRepositoryInWorkspace confirms repositoryID exists and belongs to
// workspaceID, for the same reason validateWorkflowInWorkspace does its own
// comparison.
func (s *Service) validateRepositoryInWorkspace(ctx context.Context, workspaceID, repositoryID string) error {
	repo, err := s.decisionTasks.GetRepository(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrRepositoryNotFound) {
			return &FieldError{Field: "repository_id", Message: "repository not found"}
		}
		return err
	}
	if repo == nil || repo.WorkspaceID != workspaceID {
		return &FieldError{Field: "repository_id", Message: "repository not found"}
	}
	return nil
}

// validateSourceTaskInWorkspace confirms taskID exists and belongs to
// workspaceID, for the same reason validateWorkflowInWorkspace does its own
// comparison.
func (s *Service) validateSourceTaskInWorkspace(ctx context.Context, workspaceID, taskID string) error {
	task, err := s.decisionTasks.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return &FieldError{Field: "source_task_id", Message: "source task not found"}
		}
		return err
	}
	if task == nil || task.WorkspaceID != workspaceID {
		return &FieldError{Field: "source_task_id", Message: "source task not found"}
	}
	return nil
}

// validateStepEligible loads workflowID's step graph and confirms stepID
// belongs to it and passes EligibleStep
// (docs/specs/coordinator/system-design/proposals.md#no-agent-starts).
// Called only after validateWorkflowInWorkspace has confirmed the workflow
// exists, so a step-graph read error here is a genuine read failure, not a
// missing-workflow case (a deleted workflow yields an empty graph, which
// this reports as "the step does not belong to the workflow").
func (s *Service) validateStepEligible(ctx context.Context, workflowID, stepID string) error {
	if stepID == "" {
		return &FieldError{Field: ApproveFieldStepID, Message: "step_id is required"}
	}
	nodes, err := LoadStepGraph(ctx, s.decisionSteps, workflowID)
	if err != nil {
		return err
	}
	belongs := false
	for _, n := range nodes {
		if n.ID == stepID {
			belongs = true
			break
		}
	}
	if !belongs {
		return &FieldError{Field: ApproveFieldStepID, Message: "the step does not belong to the workflow"}
	}
	if !EligibleStep(nodes, stepID) {
		return &FieldError{Field: ApproveFieldStepID, Message: "the step is not an eligible step"}
	}
	return nil
}
