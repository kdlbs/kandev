package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"go.uber.org/zap"
)

// Proposal title and free-text length limits, in Unicode code points
// (docs/specs/coordinator/requirements/proposals.md,
// AC-COORDINATOR-PROPOSALS-001.3).
const (
	proposalTitleMinRunes = 1
	proposalTitleMaxRunes = 60
	proposalTextMaxRunes  = 10000
)

// WorkflowReader resolves a workflow by id for propose-time workspace
// validation. Satisfied by the task service.
type WorkflowReader interface {
	GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error)
}

// RepositoryReader resolves a repository by id for propose-time workspace
// validation of an optional repository. Satisfied by the task service.
type RepositoryReader interface {
	GetRepository(ctx context.Context, id string) (*taskmodels.Repository, error)
}

// SourceTaskReader resolves a task by id for propose-time workspace
// validation of an optional source task. Satisfied by the task service.
type SourceTaskReader interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
}

// WorkflowStepReader lists a workflow's full step graph, used both to
// default an omitted step to the workflow's start step and to run
// EligibleStep (docs/specs/coordinator/system-design/proposals.md#no-agent-
// starts). Satisfied by the workflow service.
type WorkflowStepReader interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error)
}

// ProposeTaskRequest is propose_task_kandev's input
// (docs/specs/coordinator/system-design/proposals.md#propose). StepID,
// RepositoryID and SourceTaskID are optional.
type ProposeTaskRequest struct {
	Title        string
	Description  string
	Rationale    string
	WorkflowID   string
	StepID       string
	RepositoryID string
	SourceTaskID string
}

// SetProposalDeps registers the readers ProposeTask needs to validate a
// proposal's referenced workflow, step, repository and source task. Required
// before any ProposeTask call once features.coordinator is enabled.
func (s *Service) SetProposalDeps(workflows WorkflowReader, repositories RepositoryReader, tasks SourceTaskReader, steps WorkflowStepReader) {
	s.proposalWorkflows = workflows
	s.proposalRepositories = repositories
	s.proposalTasks = tasks
	s.proposalSteps = steps
}

// ProposeTask validates req and inserts one pending proposal for
// coordinatorID, honoring the per-coordinator 25-open cap
// (docs/specs/coordinator/system-design/proposals.md#propose,
// AC-COORDINATOR-PROPOSALS-001.1 through .5). It returns the coordinator's
// open-proposal count after insert, for the caller to publish
// events.CoordinatorUpdated. It carries no workspace scope of its own: the
// caller has already resolved and authorized coordinatorID through the MCP
// guard (internal/mcp/handlers/coordinator_authorization.go).
func (s *Service) ProposeTask(ctx context.Context, coordinatorID string, req ProposeTaskRequest) (*Proposal, int, error) {
	found, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, 0, err
	}
	spec, err := s.buildProposalSpec(ctx, found.WorkspaceID, req)
	if err != nil {
		return nil, 0, err
	}

	proposal := &Proposal{
		CoordinatorID: coordinatorID,
		WorkspaceID:   found.WorkspaceID,
		Spec:          spec,
	}
	if err := s.store.InsertProposal(ctx, proposal); err != nil {
		return nil, 0, err
	}
	openCount, err := s.store.CountOpenProposals(ctx, coordinatorID)
	if err != nil {
		return nil, 0, fmt.Errorf("count open proposals: %w", err)
	}
	s.logger.Info("proposal created",
		zap.String("coordinator_id", coordinatorID), zap.String("proposal_id", proposal.ID),
		zap.String("workflow_id", spec.WorkflowID), zap.String("step_id", spec.StepID))
	return proposal, openCount, nil
}

// buildProposalSpec validates req's fields per
// AC-COORDINATOR-PROPOSALS-001.3, defaults an omitted step to the workflow's
// start step per .2, and runs EligibleStep on the resolved step. Every
// failure is a *FieldError naming the offending field.
func (s *Service) buildProposalSpec(ctx context.Context, workspaceID string, req ProposeTaskRequest) (ProposalSpec, error) {
	title := strings.TrimSpace(req.Title)
	if n := utf8.RuneCountInString(title); n < proposalTitleMinRunes || n > proposalTitleMaxRunes {
		return ProposalSpec{}, &FieldError{
			Field:   "title",
			Message: fmt.Sprintf("title must be %d to %d characters", proposalTitleMinRunes, proposalTitleMaxRunes),
		}
	}
	if utf8.RuneCountInString(req.Description) > proposalTextMaxRunes {
		return ProposalSpec{}, &FieldError{
			Field:   "description",
			Message: fmt.Sprintf("description must be at most %d characters", proposalTextMaxRunes),
		}
	}
	if utf8.RuneCountInString(req.Rationale) > proposalTextMaxRunes {
		return ProposalSpec{}, &FieldError{
			Field:   "rationale",
			Message: fmt.Sprintf("rationale must be at most %d characters", proposalTextMaxRunes),
		}
	}

	workflow, err := s.proposalWorkflows.GetWorkflow(ctx, req.WorkflowID)
	if err != nil && !errors.Is(err, repoerrors.ErrWorkflowNotFound) {
		return ProposalSpec{}, fmt.Errorf("get workflow: %w", err)
	}
	if workflow == nil || workflow.WorkspaceID != workspaceID {
		return ProposalSpec{}, &FieldError{Field: "workflow_id", Message: "workflow not found in this workspace"}
	}

	if err := s.validateProposalSourceTask(ctx, workspaceID, req.SourceTaskID); err != nil {
		return ProposalSpec{}, err
	}
	if err := s.validateProposalRepository(ctx, workspaceID, req.RepositoryID); err != nil {
		return ProposalSpec{}, err
	}

	stepID, err := s.resolveProposalStep(ctx, req.WorkflowID, req.StepID)
	if err != nil {
		return ProposalSpec{}, err
	}

	return ProposalSpec{
		Title:        title,
		Description:  req.Description,
		Rationale:    req.Rationale,
		WorkflowID:   req.WorkflowID,
		StepID:       stepID,
		RepositoryID: req.RepositoryID,
		SourceTaskID: req.SourceTaskID,
	}, nil
}

func (s *Service) validateProposalSourceTask(ctx context.Context, workspaceID, sourceTaskID string) error {
	if sourceTaskID == "" {
		return nil
	}
	task, err := s.proposalTasks.GetTask(ctx, sourceTaskID)
	if err != nil && !errors.Is(err, repoerrors.ErrTaskNotFound) {
		return fmt.Errorf("get source task: %w", err)
	}
	if task == nil || task.WorkspaceID != workspaceID {
		return &FieldError{Field: "source_task_id", Message: "source task not found in this workspace"}
	}
	return nil
}

func (s *Service) validateProposalRepository(ctx context.Context, workspaceID, repositoryID string) error {
	if repositoryID == "" {
		return nil
	}
	repository, err := s.proposalRepositories.GetRepository(ctx, repositoryID)
	if err != nil && !errors.Is(err, repoerrors.ErrRepositoryNotFound) {
		return fmt.Errorf("get repository: %w", err)
	}
	if repository == nil || repository.WorkspaceID != workspaceID {
		return &FieldError{Field: "repository_id", Message: "repository not found in this workspace"}
	}
	return nil
}

// resolveProposalStep defaults an empty stepID to workflowID's start step,
// otherwise checks stepID belongs to workflowID, then runs EligibleStep
// against the workflow's full step graph either way.
func (s *Service) resolveProposalStep(ctx context.Context, workflowID, stepID string) (string, error) {
	steps, err := s.proposalSteps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return "", fmt.Errorf("list workflow steps: %w", err)
	}

	trimmed := strings.TrimSpace(stepID)
	if trimmed == "" {
		startID, ok := proposalStartStepID(steps)
		if !ok {
			return "", &FieldError{Field: ApproveFieldStepID, Message: "workflow has no start step"}
		}
		trimmed = startID
	} else if !proposalStepBelongsToWorkflow(steps, trimmed) {
		return "", &FieldError{Field: ApproveFieldStepID, Message: "step does not belong to this workflow"}
	}

	if !EligibleStep(proposalStepNodes(steps), trimmed) {
		return "", &FieldError{Field: ApproveFieldStepID, Message: "step is not an eligible placement for a proposed task"}
	}
	return trimmed, nil
}

func proposalStartStepID(steps []*wfmodels.WorkflowStep) (string, bool) {
	for _, step := range steps {
		if step.IsStartStep {
			return step.ID, true
		}
	}
	return "", false
}

func proposalStepBelongsToWorkflow(steps []*wfmodels.WorkflowStep, stepID string) bool {
	for _, step := range steps {
		if step.ID == stepID {
			return true
		}
	}
	return false
}

func proposalStepNodes(steps []*wfmodels.WorkflowStep) []StepNode {
	nodes := make([]StepNode, len(steps))
	for i, step := range steps {
		nodes[i] = StepNode{
			ID:               step.ID,
			IsStart:          step.IsStartStep,
			AllowManualMove:  step.AllowManualMove,
			AutoStartOnEnter: step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent),
			PullFromStepID:   step.PullFromStepID,
		}
	}
	return nodes
}
