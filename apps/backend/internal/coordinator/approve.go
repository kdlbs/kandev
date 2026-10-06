package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// staleApproveWindow is the two-minute rule an approve request uses to decide
// whether an approving proposal's claim is stale
// (docs/specs/coordinator/system-design/proposals.md#recovery). The startup
// pass uses T0 instead (task 08, recovery.go).
const staleApproveWindow = 2 * time.Minute

// ProposalConflictError reports that a proposal's status ruled out the
// requested decision; Proposal is the current row, as re-read after the
// conflict was discovered (docs/specs/coordinator/system-design/
// proposals.md#approve, #reject). Handlers map this to 409 with
// NewProposalConflictResponse(Proposal).
type ProposalConflictError struct {
	Proposal *Proposal
}

func (e *ProposalConflictError) Error() string {
	return fmt.Sprintf("coordinator: proposal %s is %s", e.Proposal.ID, e.Proposal.Status)
}

// proposalExternalID builds the reserved external id an approved proposal's
// task is created and looked up under
// (docs/specs/coordinator/system-design/proposals.md#reserved-prefix).
func proposalExternalID(proposalID string) string {
	return taskservice.ReservedExternalIDPrefixCoordinatorProposal + proposalID
}

// decidingUserID resolves the approve/reject actor recorded as decided_by:
// the request identity's user id, or "" for an internal or synthetic caller
// (auth disabled), matching internal/task/service's callerScope pattern.
func decidingUserID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return ""
	}
	return identity.UserID
}

// ApproveProposal implements POST .../proposals/:pid/approve
// (docs/specs/coordinator/system-design/proposals.md#approve). edits is the
// raw request body; an empty map approves the base spec unchanged.
func (s *Service) ApproveProposal(ctx context.Context, workspaceID, coordinatorID, proposalID string, edits ApproveProposalRequest) (*Proposal, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	proposal, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID)
	if err != nil {
		return nil, err
	}
	carriesEdits := carriesApproveEdits(edits)
	decidedBy := decidingUserID(ctx)

	switch proposal.Status {
	case ProposalStatusApproved, ProposalStatusRejected:
		return nil, &ProposalConflictError{Proposal: proposal}
	case ProposalStatusApproving:
		return s.approveApproving(ctx, workspaceID, coordinatorID, proposalID, proposal, carriesEdits)
	case ProposalStatusFailed:
		return s.approveFailed(ctx, workspaceID, coordinatorID, proposalID, decidedBy, proposal, edits, carriesEdits)
	case ProposalStatusPending:
		return s.approvePending(ctx, workspaceID, coordinatorID, proposalID, decidedBy, proposal, edits)
	default:
		return nil, fmt.Errorf("coordinator: proposal %s has unknown status %q", proposalID, proposal.Status)
	}
}

// carriesApproveEdits reports whether edits has at least one of the five
// approve-edit fields, whatever their values (proposals.md#approve step 1).
func carriesApproveEdits(edits ApproveProposalRequest) bool {
	for _, field := range [...]string{
		ApproveFieldTitle, ApproveFieldDescription, ApproveFieldWorkflowID,
		ApproveFieldStepID, ApproveFieldRepositoryID,
	} {
		if _, present := edits[field]; present {
			return true
		}
	}
	return false
}

// approveApproving handles an approve request against a currently-approving
// row: edits are always a conflict, a non-stale claim is a conflict, and a
// stale claim with no edits takes the stale re-claim.
func (s *Service) approveApproving(ctx context.Context, workspaceID, coordinatorID, proposalID string, proposal *Proposal, carriesEdits bool) (*Proposal, error) {
	if carriesEdits {
		return nil, &ProposalConflictError{Proposal: proposal}
	}
	cutoff := time.Now().UTC().Add(-staleApproveWindow)
	if proposal.ClaimedAt == nil || !proposal.ClaimedAt.Before(cutoff) {
		return nil, &ProposalConflictError{Proposal: proposal}
	}
	return s.reclaimStaleAndProceed(ctx, workspaceID, coordinatorID, proposalID, cutoff)
}

// approvePending validates and claims a never-claimed proposal (proposals.md#approve
// step 2 and step 3, pending branch).
func (s *Service) approvePending(ctx context.Context, workspaceID, coordinatorID, proposalID, decidedBy string, proposal *Proposal, edits ApproveProposalRequest) (*Proposal, error) {
	candidate, err := s.buildCandidateSpec(ctx, workspaceID, proposal.Spec, edits)
	if err != nil {
		return nil, err
	}
	validated, err := s.validateProposalSpec(ctx, workspaceID, candidate)
	if err != nil {
		return nil, err
	}
	return s.claimAndProceed(ctx, workspaceID, coordinatorID, proposalID, decidedBy, validated, nil)
}

// approveFailed handles an approve request against a failed row
// (proposals.md#approve step 1's failed branch): a task already created by an
// earlier attempt short-circuits validation and the create call; otherwise
// this behaves like approvePending but bases edits on final_spec_json.
func (s *Service) approveFailed(ctx context.Context, workspaceID, coordinatorID, proposalID, decidedBy string, proposal *Proposal, edits ApproveProposalRequest, carriesEdits bool) (*Proposal, error) {
	foundTask, err := s.decisionTasks.GetTaskByExternalID(ctx, workspaceID, proposalExternalID(proposalID))
	switch {
	case err == nil:
		if carriesEdits {
			return nil, &ProposalConflictError{Proposal: proposal}
		}
		base := ProposalSpec{}
		if proposal.FinalSpec != nil {
			base = *proposal.FinalSpec
		}
		return s.claimAndProceed(ctx, workspaceID, coordinatorID, proposalID, decidedBy, base, foundTask)
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		base := proposal.Spec
		if proposal.FinalSpec != nil {
			base = *proposal.FinalSpec
		}
		candidate, verr := s.buildCandidateSpec(ctx, workspaceID, base, edits)
		if verr != nil {
			return nil, verr
		}
		validated, verr := s.validateProposalSpec(ctx, workspaceID, candidate)
		if verr != nil {
			return nil, verr
		}
		return s.claimAndProceed(ctx, workspaceID, coordinatorID, proposalID, decidedBy, validated, nil)
	default:
		return nil, err
	}
}

// buildCandidateSpec merges edits onto base per proposals.md#edits: absent
// fields are unchanged, an empty step_id (or a workflow_id change with
// step_id absent) resolves to the target workflow's start step, and an empty
// workflow_id or empty title fail validation downstream. Strings are trimmed
// here; validateProposalSpec re-trims title/description/rationale, which is
// harmless. An edited workflow_id has its workspace ownership checked as soon
// as it changes, before resolveStepEdit's step-graph read: that read
// (LoadStepGraph/ListStepsByWorkflow) does no authorization of its own and
// trusts the caller to have already scoped the workspace
// (apps/backend/AGENTS.md).
func (s *Service) buildCandidateSpec(ctx context.Context, workspaceID string, base ProposalSpec, edits ApproveProposalRequest) (ProposalSpec, error) {
	candidate := base

	if v, present, err := edits.StringField(ApproveFieldTitle); err != nil {
		return ProposalSpec{}, err
	} else if present {
		candidate.Title = strings.TrimSpace(*v)
	}
	if v, present, err := edits.StringField(ApproveFieldDescription); err != nil {
		return ProposalSpec{}, err
	} else if present {
		candidate.Description = strings.TrimSpace(*v)
	}
	if v, present, err := edits.StringField(ApproveFieldRepositoryID); err != nil {
		return ProposalSpec{}, err
	} else if present {
		candidate.RepositoryID = strings.TrimSpace(*v)
	}

	workflowChanged, err := applyWorkflowIDEdit(edits, &candidate)
	if err != nil {
		return ProposalSpec{}, err
	}
	if workflowChanged {
		if err := s.validateWorkflowInWorkspace(ctx, workspaceID, candidate.WorkflowID); err != nil {
			return ProposalSpec{}, err
		}
	}
	stepPresent, stepValue, err := stepIDEdit(edits)
	if err != nil {
		return ProposalSpec{}, err
	}

	stepID, err := s.resolveStepEdit(ctx, candidate, workflowChanged, stepPresent, stepValue)
	if err != nil {
		return ProposalSpec{}, err
	}
	candidate.StepID = stepID
	return candidate, nil
}

// applyWorkflowIDEdit applies an edited workflow_id (if present) onto
// candidate and reports whether it actually changed the workflow, which
// resolveStepEdit needs to decide whether an absent step_id should reset to
// the new workflow's start step (proposals.md#edits).
func applyWorkflowIDEdit(edits ApproveProposalRequest, candidate *ProposalSpec) (bool, error) {
	v, present, err := edits.StringField(ApproveFieldWorkflowID)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return false, &FieldError{Field: ApproveFieldWorkflowID, Message: "workflow_id is required"}
	}
	changed := trimmed != candidate.WorkflowID
	candidate.WorkflowID = trimmed
	return changed, nil
}

// stepIDEdit reports whether edits carries a step_id and its trimmed value.
func stepIDEdit(edits ApproveProposalRequest) (present bool, value string, err error) {
	v, present, err := edits.StringField(ApproveFieldStepID)
	if err != nil || !present {
		return false, "", err
	}
	return true, strings.TrimSpace(*v), nil
}

// resolveStepEdit implements proposals.md#edits' step_id resolution: an
// explicit step_id (even "") always wins; otherwise a workflow_id change
// resets to "" so it falls through to the new workflow's start step; an
// unresolved "" at that point resolves via the step graph.
func (s *Service) resolveStepEdit(ctx context.Context, candidate ProposalSpec, workflowChanged, stepPresent bool, stepValue string) (string, error) {
	stepID := candidate.StepID
	switch {
	case stepPresent:
		stepID = stepValue
	case workflowChanged:
		stepID = ""
	}
	if stepID != "" {
		return stepID, nil
	}
	nodes, err := LoadStepGraph(ctx, s.decisionSteps, candidate.WorkflowID)
	if err != nil {
		return "", err
	}
	return StartStepID(nodes), nil
}

// claimAndProceed claims a pending or failed proposal with spec as the frozen
// spec, then continues to the pre-create eligibility check and create call
// (proposals.md#approve steps 3-6). A nil foundTask means the original
// claimer path; a non-nil foundTask means the failed-row-with-existing-task
// short circuit, which skips eligibility and create entirely.
func (s *Service) claimAndProceed(ctx context.Context, workspaceID, coordinatorID, proposalID, decidedBy string, spec ProposalSpec, foundTask *taskmodels.Task) (*Proposal, error) {
	token := uuid.New().String()
	now := time.Now().UTC()
	matched, err := s.store.ClaimProposal(ctx, proposalID, token, spec, decidedBy, now)
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.claimRaceResult(ctx, workspaceID, coordinatorID, proposalID)
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	s.logger.Info("proposal claimed",
		zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID), zap.String("workspace_id", workspaceID))
	return s.completeClaimedApproval(ctx, workspaceID, coordinatorID, proposalID, token, spec, foundTask)
}

// reclaimStaleAndProceed re-issues an approving row's claim (with a fresh
// token, cutoff-fenced) and continues exactly as claimAndProceed does,
// reusing the frozen spec the original claim recorded. Shared by an approve
// request's inline stale re-claim (cutoff = now - 2 minutes) and the startup
// recovery pass (cutoff = T0, task 08).
func (s *Service) reclaimStaleAndProceed(ctx context.Context, workspaceID, coordinatorID, proposalID string, cutoff time.Time) (*Proposal, error) {
	token := uuid.New().String()
	now := time.Now().UTC()
	matched, err := s.store.ReclaimStale(ctx, proposalID, token, now, cutoff)
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.claimRaceResult(ctx, workspaceID, coordinatorID, proposalID)
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	s.logger.Info("proposal re-claimed",
		zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID), zap.String("workspace_id", workspaceID))

	current, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID)
	if err != nil {
		return nil, err
	}
	if current.FinalSpec == nil {
		return nil, fmt.Errorf("coordinator: reclaimed proposal %s has no frozen spec", proposalID)
	}
	frozen := *current.FinalSpec

	foundTask, err := s.decisionTasks.GetTaskByExternalID(ctx, workspaceID, proposalExternalID(proposalID))
	switch {
	case err == nil:
		return s.completeClaimedApproval(ctx, workspaceID, coordinatorID, proposalID, token, frozen, foundTask)
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		return s.completeClaimedApproval(ctx, workspaceID, coordinatorID, proposalID, token, frozen, nil)
	default:
		s.logger.Warn("stale re-claim lookup failed", zap.String("proposal_id", proposalID), zap.Error(err))
		return nil, err
	}
}

// completeClaimedApproval runs the create sequence for a just-(re)claimed
// row: a found task short-circuits straight to completion; otherwise it
// re-checks step eligibility immediately before creating
// (proposals.md#no-agent-starts), then branches on the create outcome.
func (s *Service) completeClaimedApproval(ctx context.Context, workspaceID, coordinatorID, proposalID, token string, spec ProposalSpec, foundTask *taskmodels.Task) (*Proposal, error) {
	if foundTask != nil {
		return s.completeApproval(ctx, workspaceID, coordinatorID, proposalID, token, foundTask.ID)
	}

	eligible, err := s.stepStillEligible(ctx, spec)
	if err != nil {
		s.logger.Warn("step-graph read failed before proposal create",
			zap.String("proposal_id", proposalID), zap.Error(err))
		return nil, err
	}
	if !eligible {
		return s.failApproval(ctx, workspaceID, coordinatorID, proposalID, token, "the target step is no longer eligible")
	}

	result, err := s.createApprovedTask(ctx, workspaceID, proposalID, spec)
	if err != nil {
		return s.failApproval(ctx, workspaceID, coordinatorID, proposalID, token, err.Error())
	}
	return s.completeCreateOutcome(ctx, workspaceID, coordinatorID, proposalID, token, result)
}

// stepStillEligible loads the frozen spec's workflow step graph and runs
// EligibleStep against it, for the pre-create check every create call repeats
// (proposals.md#no-agent-starts).
func (s *Service) stepStillEligible(ctx context.Context, spec ProposalSpec) (bool, error) {
	nodes, err := LoadStepGraph(ctx, s.decisionSteps, spec.WorkflowID)
	if err != nil {
		return false, err
	}
	return EligibleStep(nodes, spec.StepID), nil
}

// createApprovedTask builds the task-service create request for an approved
// proposal: the frozen spec's fields, the reserved external id, the regular
// board origin, and no session/auto-start intent
// (proposals.md#no-agent-starts).
func (s *Service) createApprovedTask(ctx context.Context, workspaceID, proposalID string, spec ProposalSpec) (taskservice.CreateTaskResult, error) {
	req := &taskservice.CreateTaskRequest{
		WorkspaceID:             workspaceID,
		WorkflowID:              spec.WorkflowID,
		WorkflowStepID:          spec.StepID,
		Title:                   spec.Title,
		Description:             spec.Description,
		Origin:                  taskmodels.TaskOriginManual,
		ExternalID:              proposalExternalID(proposalID),
		AllowReservedExternalID: true,
	}
	if spec.RepositoryID != "" {
		req.Repositories = []taskservice.TaskRepositoryInput{{RepositoryID: spec.RepositoryID}}
	}
	return s.decisionTasks.CreateTask(ctx, req)
}

// completeCreateOutcome branches on CreateTask's outcome
// (proposals.md#approve step 4's create-outcome list).
func (s *Service) completeCreateOutcome(ctx context.Context, workspaceID, coordinatorID, proposalID, token string, result taskservice.CreateTaskResult) (*Proposal, error) {
	switch result.Outcome {
	case taskservice.CreateTaskOutcomeFoundUnsettled:
		s.logger.Info("approval found unsettled task from an earlier attempt",
			zap.String("proposal_id", proposalID), zap.String("task_id", result.Task.ID))
		return s.completeApproval(ctx, workspaceID, coordinatorID, proposalID, token, result.Task.ID)
	case taskservice.CreateTaskOutcomeFoundSettled:
		return s.completeApproval(ctx, workspaceID, coordinatorID, proposalID, token, result.Task.ID)
	default:
		return s.completeCreatedOutcome(ctx, workspaceID, coordinatorID, proposalID, token, result.Task.ID)
	}
}

// completeCreatedOutcome settles the external id after a fresh create and
// completes with the settled task, or the survivor when the identity was
// lost, or fails the row when the created task was deleted mid-create
// (proposals.md#approve step 4's Created branch).
func (s *Service) completeCreatedOutcome(ctx context.Context, workspaceID, coordinatorID, proposalID, token, taskID string) (*Proposal, error) {
	settled, survivor, err := s.decisionTasks.SettleExternalID(ctx, taskID, proposalExternalID(proposalID))
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return s.failApproval(ctx, workspaceID, coordinatorID, proposalID, token, "The created task was deleted before approval completed")
		}
		s.logger.Warn("settle external id failed after proposal task create",
			zap.String("proposal_id", proposalID), zap.String("task_id", taskID), zap.Error(err))
		return nil, err
	}
	if !settled {
		s.logger.Warn("external id identity lost after proposal task create; completing with survivor",
			zap.String("proposal_id", proposalID), zap.String("task_id", survivor.ID))
		return s.completeApproval(ctx, workspaceID, coordinatorID, proposalID, token, survivor.ID)
	}
	return s.completeApproval(ctx, workspaceID, coordinatorID, proposalID, token, taskID)
}

// completeApproval runs step 5's success fence and its zero-row race
// handling (proposals.md#approve step 5).
func (s *Service) completeApproval(ctx context.Context, workspaceID, coordinatorID, proposalID, token, taskID string) (*Proposal, error) {
	matched, err := s.store.CompleteProposal(ctx, proposalID, token, taskID, time.Now())
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.settleWriteRace(ctx, workspaceID, coordinatorID, proposalID, taskID)
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	s.logger.Info("proposal approved",
		zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID),
		zap.String("workspace_id", workspaceID), zap.String("task_id", taskID))
	return s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID)
}

// failApproval runs step 5's failure fence (the same fence completeApproval
// uses) and its zero-row race handling.
func (s *Service) failApproval(ctx context.Context, workspaceID, coordinatorID, proposalID, token, errMsg string) (*Proposal, error) {
	matched, err := s.store.FailProposal(ctx, proposalID, token, errMsg, time.Now())
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.settleWriteRace(ctx, workspaceID, coordinatorID, proposalID, "")
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	s.logger.Info("proposal approval failed",
		zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID),
		zap.String("workspace_id", workspaceID), zap.String("error", errMsg))
	return s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID)
}

// claimRaceResult handles a zero-row race on a claim, re-claim or reject
// UPDATE: a row that is gone is 404, any other row is a 409 conflict with
// that row (proposals.md#approve step 3, #stale-re-claim, #reject).
func (s *Service) claimRaceResult(ctx context.Context, workspaceID, coordinatorID, proposalID string) (*Proposal, error) {
	current, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return nil, &ProposalConflictError{Proposal: current}
}

// settleWriteRace handles a zero-row race on step 5's complete/fail UPDATE: a
// row that is gone is 404 (task stays on its board); any other row is
// returned as-is with 200, writing nothing further
// (proposals.md#approve step 5). createdTaskID is this request's own create
// outcome, when any, logged at warn to flag the task it leaves orphaned on
// its board; empty for a fail-fence race, logged at info.
func (s *Service) settleWriteRace(ctx context.Context, workspaceID, coordinatorID, proposalID, createdTaskID string) (*Proposal, error) {
	current, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID)
	if errors.Is(err, ErrNotFound) {
		s.logger.Info("proposal deleted during approval completion", zap.String("proposal_id", proposalID))
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if createdTaskID != "" {
		s.logger.Warn("approval completion raced with another decision; created task left on its board",
			zap.String("proposal_id", proposalID), zap.String("task_id", createdTaskID))
	} else {
		s.logger.Info("approval completion raced with another decision", zap.String("proposal_id", proposalID))
	}
	return current, nil
}
