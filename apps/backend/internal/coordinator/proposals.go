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
	// InReplyTo names the returned create_task proposal this one revises.
	InReplyTo string
	// StandingOrderIDs cites the standing orders that shaped the proposal.
	StandingOrderIDs []string
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
	if !s.phase2 {
		req.StandingOrderIDs = nil
	}
	if err := s.checkInReplyTo(ctx, found, req.InReplyTo); err != nil {
		return nil, 0, err
	}
	if err := checkOrderIDsShape(req.StandingOrderIDs); err != nil {
		return nil, 0, err
	}
	relaxed := s.phase2 && s.policyFor(found).Allows(ActionStartAgent)
	spec, startsAgent, err := s.buildProposalSpec(ctx, found.WorkspaceID, req, relaxed)
	if err != nil {
		return nil, 0, err
	}
	if s.phase2 {
		if err := s.checkProposalWatched(ctx, coordinatorID, found.WorkspaceID, spec); err != nil {
			return nil, 0, err
		}
	}

	proposal := &Proposal{
		CoordinatorID: coordinatorID,
		WorkspaceID:   found.WorkspaceID,
		Spec:          spec,
		StartsAgent:   startsAgent,
		InReplyTo:     optString(req.InReplyTo),
	}
	if s.phase2 {
		proposal.StandingOrderIDs = append([]string{}, req.StandingOrderIDs...)
	}
	pre := func(ctx context.Context, tx coordinatorExec) (*Proposal, error) {
		return nil, s.checkOrdersActive(ctx, tx, coordinatorID, req.StandingOrderIDs)
	}
	turnID := s.currentUnattendedTurn(ctx, coordinatorID)
	inTx := func(ctx context.Context, tx coordinatorExec, p *Proposal) error {
		if err := s.recordProposed(ctx, tx, p, turnID); err != nil {
			return err
		}
		return s.store.MarkApplied(ctx, tx, coordinatorID, req.StandingOrderIDs, p.CreatedAt)
	}
	if err := s.store.InsertProposalWith(ctx, proposal, s.phase2, pre, inTx); err != nil {
		return nil, 0, err
	}
	openCount, err := s.store.CountOpenProposals(ctx, coordinatorID, s.phase2)
	if err != nil {
		return nil, 0, fmt.Errorf("count open proposals: %w", err)
	}
	s.logger.Info("proposal created",
		zap.String("coordinator_id", coordinatorID), zap.String("proposal_id", proposal.ID),
		zap.String("workflow_id", spec.WorkflowID), zap.String("step_id", spec.StepID))
	return proposal, openCount, nil
}

// recordProposed appends the proposed / requires_approval activity row for a
// freshly inserted proposal, in the insert's own transaction.
func (s *Service) recordProposed(ctx context.Context, tx coordinatorExec, p *Proposal, turnID *string) error {
	id := p.ID
	return s.Record(ctx, tx, ActivityRow{
		CoordinatorID:    p.CoordinatorID,
		WorkspaceID:      p.WorkspaceID,
		ActionClass:      ActionCreateTask,
		Outcome:          ActivityProposed,
		Authorization:    AuthRequiresApproval,
		ProposalID:       &id,
		Detail:           p.Spec.Title,
		UnattendedTurnID: turnID,
	})
}

// buildProposalSpec validates req's fields per
// AC-COORDINATOR-PROPOSALS-001.3, defaults an omitted step to the workflow's
// start step per .2, and runs EligibleStep on the resolved step. Every
// failure is a *FieldError naming the offending field.
func (s *Service) buildProposalSpec(ctx context.Context, workspaceID string, req ProposeTaskRequest, relaxed bool) (ProposalSpec, bool, error) {
	title := strings.TrimSpace(req.Title)
	if n := utf8.RuneCountInString(title); n < proposalTitleMinRunes || n > proposalTitleMaxRunes {
		return ProposalSpec{}, false, &FieldError{
			Field:   "title",
			Message: fmt.Sprintf("title must be %d to %d characters", proposalTitleMinRunes, proposalTitleMaxRunes),
		}
	}
	if utf8.RuneCountInString(req.Description) > proposalTextMaxRunes {
		return ProposalSpec{}, false, &FieldError{
			Field:   "description",
			Message: fmt.Sprintf("description must be at most %d characters", proposalTextMaxRunes),
		}
	}
	if utf8.RuneCountInString(req.Rationale) > proposalTextMaxRunes {
		return ProposalSpec{}, false, &FieldError{
			Field:   "rationale",
			Message: fmt.Sprintf("rationale must be at most %d characters", proposalTextMaxRunes),
		}
	}

	workflow, err := s.proposalWorkflows.GetWorkflow(ctx, req.WorkflowID)
	if _, err := resolveWorkspaceScopedRef(
		"workflow", workflow, err, repoerrors.ErrWorkflowNotFound,
		func(w *taskmodels.Workflow) string { return w.WorkspaceID }, workspaceID,
		&FieldError{Field: "workflow_id", Message: "workflow not found in this workspace"},
	); err != nil {
		return ProposalSpec{}, false, err
	}

	if err := s.validateProposalSourceTask(ctx, workspaceID, req.SourceTaskID); err != nil {
		return ProposalSpec{}, false, err
	}
	if err := s.validateProposalRepository(ctx, workspaceID, req.RepositoryID); err != nil {
		return ProposalSpec{}, false, err
	}

	stepID, startsAgent, err := s.resolveProposalStep(ctx, req.WorkflowID, req.StepID, relaxed)
	if err != nil {
		return ProposalSpec{}, false, err
	}

	return ProposalSpec{
		Title:        title,
		Description:  req.Description,
		Rationale:    req.Rationale,
		WorkflowID:   req.WorkflowID,
		StepID:       stepID,
		RepositoryID: req.RepositoryID,
		SourceTaskID: req.SourceTaskID,
	}, startsAgent, nil
}

// checkProposalWatched refuses a proposal whose workflow, whose source task,
// or whose repository is outside the coordinator's Watches. A source task
// without a workflow is never watched.
func (s *Service) checkProposalWatched(ctx context.Context, coordinatorID, workspaceID string, spec ProposalSpec) error {
	set, err := s.EffectiveWatchSet(ctx, coordinatorID)
	if err != nil {
		return fmt.Errorf("read watch set: %w", err)
	}
	g, err := s.newWatchGate(ctx, set, workspaceID)
	if err != nil {
		return fmt.Errorf("read projects: %w", err)
	}
	if !set.Contains(spec.WorkflowID) {
		return &FieldError{Field: "workflow_id", Message: "workflow is outside this coordinator's watches"}
	}
	if g.projects.Selected() && !g.projects.InProjects(repoIDsOf(spec.RepositoryID)) {
		return &FieldError{Field: ApproveFieldRepositoryID, Message: "repository is outside this coordinator's projects"}
	}
	return s.checkSourceTaskWatched(ctx, g, spec)
}

func repoIDsOf(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}

// sourceTaskUnavailableMessage is the one refusal for a source task that is
// missing, in another workspace, or outside the watches, so the response does
// not reveal which.
const sourceTaskUnavailableMessage = "source task not found in this workspace"

func (s *Service) checkSourceTaskWatched(ctx context.Context, g *watchGate, spec ProposalSpec) error {
	if spec.SourceTaskID == "" {
		return nil
	}
	task, err := s.proposalTasks.GetTask(ctx, spec.SourceTaskID)
	if err != nil {
		return fmt.Errorf("get source task: %w", err)
	}
	outside := &FieldError{Field: fieldSourceTaskID, Message: sourceTaskUnavailableMessage}
	if task == nil {
		return outside
	}
	repos, err := s.taskRepositoryIDs(ctx, g, []string{task.ID})
	if err != nil {
		s.logger.Warn("coordinator: source task repository read failed; treating task as outside the projects",
			zap.String("task_id", task.ID), zap.Error(err))
		return outside
	}
	if !g.task(task.WorkflowID, repos[task.ID]) {
		return outside
	}
	return nil
}

func (s *Service) validateProposalSourceTask(ctx context.Context, workspaceID, sourceTaskID string) error {
	if sourceTaskID == "" {
		return nil
	}
	task, err := s.proposalTasks.GetTask(ctx, sourceTaskID)
	_, err = resolveWorkspaceScopedRef(
		"source task", task, err, repoerrors.ErrTaskNotFound,
		func(t *taskmodels.Task) string { return t.WorkspaceID }, workspaceID,
		&FieldError{Field: fieldSourceTaskID, Message: sourceTaskUnavailableMessage},
	)
	return err
}

func (s *Service) validateProposalRepository(ctx context.Context, workspaceID, repositoryID string) error {
	if repositoryID == "" {
		return nil
	}
	repository, err := s.proposalRepositories.GetRepository(ctx, repositoryID)
	_, err = resolveWorkspaceScopedRef(
		"repository", repository, err, repoerrors.ErrRepositoryNotFound,
		func(r *taskmodels.Repository) string { return r.WorkspaceID }, workspaceID,
		&FieldError{Field: ApproveFieldRepositoryID, Message: "repository not found in this workspace"},
	)
	return err
}

// resolveWorkspaceScopedRef fetches a proposal-referenced entity and
// collapses every outcome that means "does not exist for this coordinator's
// workspace" — a raw notFoundErr, a nil result with no error, or a result
// belonging to a different workspace — into fieldErr. It is the single path
// workflow_id, repository_id and source_task_id all resolve through, so a fix
// to what counts as "not found" is applied once rather than needing to be
// copied into each field's lookup and risking being missed on one of them,
// which is exactly what happened to workflow_id alone across SEC-002, R2-A
// and R3-A.
//
// Any other error from the lookup is a genuine failure (a transient database
// error, for example), not an authorization outcome, and is propagated
// rather than folded into fieldErr: collapsing it too would hide a real
// backend problem behind what looks like ordinary input validation.
func resolveWorkspaceScopedRef[T any](
	label string,
	entity *T, err error, notFoundErr error,
	workspaceOf func(*T) string, workspaceID string,
	fieldErr *FieldError,
) (*T, error) {
	if err != nil && !errors.Is(err, notFoundErr) {
		return nil, fmt.Errorf("get %s: %w", label, err)
	}
	if entity == nil || workspaceOf(entity) != workspaceID {
		return nil, fieldErr
	}
	return entity, nil
}

// resolveProposalStep defaults an empty stepID to workflowID's start step,
// otherwise checks stepID belongs to workflowID, then runs EligibleStep
// against the workflow's full step graph either way.
func (s *Service) resolveProposalStep(ctx context.Context, workflowID, stepID string, relaxed bool) (string, bool, error) {
	steps, err := s.proposalSteps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return "", false, fmt.Errorf("list workflow steps: %w", err)
	}

	trimmed := strings.TrimSpace(stepID)
	if trimmed == "" {
		startID, ok := proposalStartStepID(steps)
		if !ok {
			return "", false, &FieldError{Field: ApproveFieldStepID, Message: "workflow has no start step"}
		}
		trimmed = startID
	} else if !proposalStepBelongsToWorkflow(steps, trimmed) {
		return "", false, &FieldError{Field: ApproveFieldStepID, Message: "step does not belong to this workflow"}
	}

	nodes := proposalStepNodes(steps)
	if relaxed {
		if !EligibleStartingStep(nodes, trimmed) {
			return "", false, &FieldError{Field: ApproveFieldStepID, Message: "step is not an eligible placement for a proposed task"}
		}
		return trimmed, StartsAgentOnEnter(nodes, trimmed), nil
	}
	if !EligibleStep(nodes, trimmed) {
		return "", false, &FieldError{Field: ApproveFieldStepID, Message: "step is not an eligible placement for a proposed task"}
	}
	return trimmed, false, nil
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
			CompletesOnEnter: step.CompleteTaskOnEnter,
			PullFromStepID:   step.PullFromStepID,
		}
	}
	return nodes
}

// checkInReplyTo refuses an in_reply_to that does not name a returned
// create_task proposal of this coordinator, and any non-empty value while
// phase 3 is off. Several proposals may name one returned row.
func (s *Service) checkInReplyTo(ctx context.Context, c *Coordinator, inReplyTo string) error {
	if inReplyTo == "" {
		return nil
	}
	refused := &FieldError{Field: "in_reply_to", Message: "in_reply_to must name a returned proposal of this coordinator"}
	if !s.phase3 {
		return refused
	}
	target, err := s.store.GetProposal(ctx, c.WorkspaceID, c.ID, inReplyTo, s.phase2)
	if errors.Is(err, ErrNotFound) {
		return refused
	}
	if err != nil {
		return fmt.Errorf("read in_reply_to proposal: %w", err)
	}
	if target.Status != ProposalStatusReturned || replyKind(target) != ProposalKindCreateTask {
		return refused
	}
	return nil
}
