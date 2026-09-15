package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/office/shared"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// handoffSourceTaskIDKey, handoffTaskIDKey and handoffHandedOffAtKey are the
// forward- and reverse-provenance metadata field names, extracted so they
// are written and read identically everywhere they appear.
const (
	handoffSourceTaskIDKey = "source_task_id"
	handoffTaskIDKey       = "task_id"
	handoffHandedOffAtKey  = "handed_off_at"

	handoffOutcomeCreated             = "created"
	handoffOutcomeFoundSettled        = "found_settled"
	handoffOutcomeFoundUnsettled      = "found_unsettled"
	handoffOutcomeCreatedIdentityLost = "created_identity_lost"

	handoffPermissionDeniedMessage = "the handoff action requires the can_handoff_tasks permission, granted per agent or per role"
	handoffTargetIsSourceMessage   = "target_workspace_id equals your own workspace; create same-workspace tasks through kandev task create instead"

	handoffReverseLinkUnreadableSuffix = "the source task's stored handoff_source.handed_off_at is unreadable; this handoff cannot be repaired automatically"
)

// HandoffValidationError is a caller-correctable defect in a handoff request
// (AC-5d: reported as HTTP 400 naming the field or condition).
type HandoffValidationError struct {
	msg string
}

func (e *HandoffValidationError) Error() string { return e.msg }

func newHandoffValidationError(format string, args ...interface{}) *HandoffValidationError {
	return &HandoffValidationError{msg: fmt.Sprintf(format, args...)}
}

// handoffSettlementTaskIDSuffix is F53's stable, parseable marker: the
// settlement-failure error body always ends with this literal followed by
// the delivery task id, so a caller can recover the id by splitting on it
// rather than parsing prose. Not a replacement for the JSON envelope itself
// (still `{"error": <string>}`) — just a fixed anchor within that string.
const handoffSettlementTaskIDSuffix = "; task_id="

// HandoffSettlementError reports that a delivery task was created but its
// external_id could not be settled (AC-24d/F53). The route maps this to HTTP
// 500 with the delivery task id carried in the error body as a stable
// suffix (handoffSettlementTaskIDSuffix + TaskID), since the task was NOT
// rolled back and the caller needs its id to recover manually.
type HandoffSettlementError struct {
	TaskID string
	cause  error
}

func (e *HandoffSettlementError) Error() string {
	return fmt.Sprintf("delivery task was created but could not settle its external_id: %v%s%s",
		e.cause, handoffSettlementTaskIDSuffix, e.TaskID)
}

func (e *HandoffSettlementError) Unwrap() error { return e.cause }

var errHandoffPermissionDenied = fmt.Errorf("%w: %s", shared.ErrForbidden, handoffPermissionDeniedMessage)

// HandoffWorkspaceScoper attaches the identity of the source task's owning
// user to ctx (AC-11), reusing the same per-user scoping in-session MCP
// dispatch already relies on, so a subsequent GetWorkspace on the target
// workspace is denied exactly when that workspace is not visible to the
// caller — indistinguishable from the workspace not existing at all.
type HandoffWorkspaceScoper interface {
	Scope(ctx context.Context, taskID string) (context.Context, error)
}

// HandoffTaskService is the task-service surface the handoff action needs.
type HandoffTaskService interface {
	GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error)
	GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error)
	GetExecutorProfile(ctx context.Context, id string) (*taskmodels.ExecutorProfile, error)
	GetRepository(ctx context.Context, id string) (*taskmodels.Repository, error)
	CreateTask(ctx context.Context, req *service.CreateTaskRequest) (service.CreateTaskResult, error)
	SettleExternalID(ctx context.Context, taskID, externalID string) (settled bool, survivor *taskmodels.Task, err error)
}

// HandoffWorkflowSteps resolves a workflow's steps for destination selection.
type HandoffWorkflowSteps interface {
	ListStepsByWorkflow(ctx context.Context, req workflowctrl.ListStepsRequest) (*workflowctrl.ListStepsResponse, error)
}

// HandoffAgentProfiles validates that an agent profile belongs to a workspace.
type HandoffAgentProfiles interface {
	AgentProfileBelongsToWorkspace(ctx context.Context, agentProfileID, workspaceID string) (bool, error)
}

// HandoffReverseLinkStore is the CAS-backed reverse-link store on the source
// task's handoffs metadata array (AC-25/AC-26/AC-27/AC-28).
type HandoffReverseLinkStore interface {
	GetTaskHandoffsRaw(ctx context.Context, taskID string) (string, error)
	SetTaskHandoffsIfUnchanged(ctx context.Context, taskID, expectedHandoffsJSON, newHandoffsJSON string) (stored bool, currentHandoffsJSON string, err error)
}

// HandoffLaunchRequest is a launcher-agnostic mirror of the fields the
// handoff action needs to start a session, kept independent of
// internal/orchestrator to avoid an import cycle (internal/office/dashboard
// already imports internal/office/runtime, and internal/orchestrator imports
// internal/office/dashboard).
type HandoffLaunchRequest struct {
	TaskID            string
	AgentProfileID    string
	ExecutorID        string
	ExecutorProfileID string
	WorkflowStepID    string
	Prompt            string
}

// HandoffLauncher dispatches a synchronous session launch (AC-32).
type HandoffLauncher interface {
	LaunchSession(ctx context.Context, req HandoffLaunchRequest) error
}

// HandoffActivityLogger records handoff activity entries (AC-19). Never
// consulted for run resolution — the run id is read directly off RunContext,
// per AC-19a.
type HandoffActivityLogger interface {
	LogActivityWithRun(ctx context.Context, workspaceID, actorType, actorID, action, targetType, targetID, details, runID, sessionID string)
}

// HandoffDependencies groups the dependencies the handoff action needs,
// beyond the narrower ActionDependencies interfaces the rest of the runtime
// syscall surface uses.
type HandoffDependencies struct {
	Workspaces    HandoffWorkspaceScoper
	Tasks         HandoffTaskService
	Workflows     HandoffWorkflowSteps
	AgentProfiles HandoffAgentProfiles
	ReverseLinks  HandoffReverseLinkStore
	Launcher      HandoffLauncher
	Activity      HandoffActivityLogger
}

// HandoffRequest is the wire and business-validation shape for the handoff
// action (AC-2a/AC-5). Optional strings are pointers so a present-but-blank
// value (AC-5a) is distinguishable from an omitted one.
type HandoffRequest struct {
	TargetWorkspaceID string  `json:"target_workspace_id"`
	WorkflowID        string  `json:"workflow_id"`
	Title             string  `json:"title"`
	Prompt            string  `json:"prompt"`
	AgentProfileID    string  `json:"agent_profile_id"`
	ExecutorProfileID string  `json:"executor_profile_id"`
	RepositoryID      *string `json:"repository_id"`
	BaseBranch        *string `json:"base_branch"`
	StartAgent        *bool   `json:"start_agent"`
	ExternalID        *string `json:"external_id"`
}

// HandoffResult is the handoff action's response shape (AC-33). Message is an
// addition (not a replacement for outcome) carrying AC-24b/AC-24c's caller
// guidance on the two outcomes that need it.
type HandoffResult struct {
	TaskID              string `json:"task_id"`
	WorkspaceID         string `json:"workspace_id"`
	WorkflowID          string `json:"workflow_id"`
	WorkflowStepID      string `json:"workflow_step_id"`
	Outcome             string `json:"outcome"`
	CreationComplete    bool   `json:"creation_complete"`
	Started             bool   `json:"started"`
	ReverseLinkRecorded bool   `json:"reverse_link_recorded"`
	HandedOffAt         string `json:"handed_off_at"`
	ReverseLinkError    string `json:"reverse_link_error,omitempty"`
	StartError          string `json:"start_error,omitempty"`
	Message             string `json:"message,omitempty"`
}

type handoffResolvedResources struct {
	WorkflowStepID string
	ExecutorID     string
	Repositories   []service.TaskRepositoryInput
}

// Handoff creates a delivery task in a workspace other than the run's own,
// implementing D3a's seven-step total evaluation order and D3b's post-create
// sequence.
func (a *Actions) Handoff(ctx context.Context, runCtx RunContext, req HandoffRequest) (*HandoffResult, error) {
	trimmed, verr := validateHandoffShape(req)
	if verr != nil {
		return nil, verr
	}
	if trimmed.TargetWorkspaceID == runCtx.WorkspaceID {
		return nil, newHandoffValidationError(handoffTargetIsSourceMessage)
	}
	if !runCtx.Capabilities.Allows(CapabilityHandoffTask) {
		return nil, errHandoffPermissionDenied
	}

	scopedCtx, workspace, err := a.authorizeHandoffTargetWorkspace(ctx, runCtx.TaskID, trimmed.TargetWorkspaceID)
	if err != nil {
		return nil, err
	}

	resolved, err := a.resolveHandoffTargetResources(scopedCtx, trimmed, workspace)
	if err != nil {
		return nil, err
	}

	return a.executeHandoff(scopedCtx, runCtx, trimmed, resolved)
}

// validateHandoffShape is D3a step 2. Checks run in the R1 table's
// top-to-bottom order.
func validateHandoffShape(req HandoffRequest) (HandoffRequest, *HandoffValidationError) {
	out := HandoffRequest{
		TargetWorkspaceID: strings.TrimSpace(req.TargetWorkspaceID),
		WorkflowID:        strings.TrimSpace(req.WorkflowID),
		Title:             req.Title,
		Prompt:            req.Prompt,
		AgentProfileID:    strings.TrimSpace(req.AgentProfileID),
		ExecutorProfileID: strings.TrimSpace(req.ExecutorProfileID),
		StartAgent:        req.StartAgent,
	}
	if out.TargetWorkspaceID == "" {
		return out, newHandoffValidationError("target_workspace_id is required")
	}
	if out.WorkflowID == "" {
		return out, newHandoffValidationError("workflow_id is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return out, newHandoffValidationError("title is required")
	}
	if titleLen := len([]rune(req.Title)); titleLen > service.TaskTitleMaxLength {
		return out, newHandoffValidationError("title must be %d characters or fewer (got %d)", service.TaskTitleMaxLength, titleLen)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return out, newHandoffValidationError("prompt is required")
	}
	if out.AgentProfileID == "" {
		return out, newHandoffValidationError("agent_profile_id is required")
	}
	if out.ExecutorProfileID == "" {
		return out, newHandoffValidationError("executor_profile_id is required")
	}
	if req.RepositoryID != nil {
		v := strings.TrimSpace(*req.RepositoryID)
		if v == "" {
			return out, newHandoffValidationError("repository_id must not be blank when supplied")
		}
		out.RepositoryID = &v
	}
	if req.BaseBranch != nil {
		v := strings.TrimSpace(*req.BaseBranch)
		if v == "" {
			return out, newHandoffValidationError("base_branch must not be blank when supplied")
		}
		out.BaseBranch = &v
	}
	if req.ExternalID != nil {
		v := strings.TrimSpace(*req.ExternalID)
		if v == "" {
			return out, newHandoffValidationError("external_id must not be blank when supplied")
		}
		out.ExternalID = &v
	}
	return out, nil
}

// authorizeHandoffTargetWorkspace is D3a step 5 (AC-11): the existing
// per-user workspace scoping, indistinguishable from not-found on denial.
func (a *Actions) authorizeHandoffTargetWorkspace(
	ctx context.Context, sourceTaskID, targetWorkspaceID string,
) (context.Context, *taskmodels.Workspace, error) {
	if a.deps.Handoff.Workspaces == nil || a.deps.Handoff.Tasks == nil {
		return nil, nil, fmt.Errorf("%w: handoff", ErrRuntimeDependencyMissing)
	}
	scopedCtx, err := a.deps.Handoff.Workspaces.Scope(ctx, sourceTaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve caller identity: %w", err)
	}
	workspace, err := a.deps.Handoff.Tasks.GetWorkspace(scopedCtx, targetWorkspaceID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return nil, nil, newHandoffValidationError("target_workspace_id %q was not found", targetWorkspaceID)
		}
		return nil, nil, fmt.Errorf("failed to validate target_workspace_id: %w", err)
	}
	return scopedCtx, workspace, nil
}

// resolveHandoffTargetResources is D3a step 6, in its stated order:
// workflow_id, destination step, agent_profile_id, executor_profile_id,
// repository_id, base_branch.
func (a *Actions) resolveHandoffTargetResources(
	ctx context.Context, req HandoffRequest, workspace *taskmodels.Workspace,
) (*handoffResolvedResources, error) {
	if err := a.validateHandoffWorkflow(ctx, req.WorkflowID, workspace); err != nil {
		return nil, err
	}
	startAgent := req.StartAgent != nil && *req.StartAgent
	stepID, err := a.resolveHandoffDestinationStep(ctx, req.WorkflowID, startAgent)
	if err != nil {
		return nil, err
	}
	if err := a.validateHandoffAgentProfile(ctx, req.AgentProfileID, workspace.ID); err != nil {
		return nil, err
	}
	executorID, err := a.validateHandoffExecutorProfile(ctx, req.ExecutorProfileID)
	if err != nil {
		return nil, err
	}
	repositoryID := ""
	if req.RepositoryID != nil {
		repositoryID = *req.RepositoryID
	}
	baseBranch := ""
	if req.BaseBranch != nil {
		baseBranch = *req.BaseBranch
	}
	repos, err := a.validateHandoffRepository(ctx, repositoryID, baseBranch, workspace.ID)
	if err != nil {
		return nil, err
	}
	return &handoffResolvedResources{WorkflowStepID: stepID, ExecutorID: executorID, Repositories: repos}, nil
}

// validateHandoffWorkflow is AC-12/AC-12a/AC-12b/AC-12c. It deliberately does
// not disclose the workflow's actual owning workspace (AC-12a).
func (a *Actions) validateHandoffWorkflow(ctx context.Context, workflowID string, workspace *taskmodels.Workspace) error {
	const notAMemberMessage = "workflow_id is not a workflow of the target workspace"
	workflow, err := a.deps.Handoff.Tasks.GetWorkflow(ctx, workflowID)
	if err != nil {
		if isHandoffNotFoundError(err) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return newHandoffValidationError(notAMemberMessage)
		}
		return fmt.Errorf("failed to validate workflow_id: %w", err)
	}
	if workflow.WorkspaceID != workspace.ID {
		return newHandoffValidationError(notAMemberMessage)
	}
	if workspace.OfficeWorkflowID != "" && workflowID == workspace.OfficeWorkflowID {
		return newHandoffValidationError("workflow_id must be a delivery workflow, not the target workspace's office workflow")
	}
	return nil
}

func isHandoffNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}

// resolveHandoffDestinationStep is AC-15a/AC-15b/AC-15c: a deterministic,
// (position, id)-ordered selection, distinguishing configuration-no-match
// from lookup failure.
func (a *Actions) resolveHandoffDestinationStep(ctx context.Context, workflowID string, startAgent bool) (string, error) {
	if a.deps.Handoff.Workflows == nil {
		return "", fmt.Errorf("%w: workflow steps", ErrRuntimeDependencyMissing)
	}
	resp, err := a.deps.Handoff.Workflows.ListStepsByWorkflow(ctx, workflowctrl.ListStepsRequest{WorkflowID: workflowID})
	if err != nil {
		return "", fmt.Errorf("failed to resolve destination step: %w", err)
	}
	var steps []*workflowmodels.WorkflowStep
	if resp != nil {
		steps = resp.Steps
	}
	step := selectHandoffDestinationStep(steps, startAgent)
	if step == nil {
		return "", newHandoffValidationError("workflow_id has no resolvable destination step")
	}
	return step.ID, nil
}

func selectHandoffDestinationStep(steps []*workflowmodels.WorkflowStep, startAgent bool) *workflowmodels.WorkflowStep {
	ordered := make([]*workflowmodels.WorkflowStep, 0, len(steps))
	for _, step := range steps {
		if step != nil {
			ordered = append(ordered, step)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Position != ordered[j].Position {
			return ordered[i].Position < ordered[j].Position
		}
		return ordered[i].ID < ordered[j].ID
	})
	if startAgent {
		for _, step := range ordered {
			if step.HasOnEnterAction(workflowmodels.OnEnterAutoStartAgent) {
				return step
			}
		}
	}
	for _, step := range ordered {
		if step.IsStartStep {
			return step
		}
	}
	if len(ordered) > 0 {
		return ordered[0]
	}
	return nil
}

// validateHandoffAgentProfile is AC-14b's agent_profile_id predicate.
func (a *Actions) validateHandoffAgentProfile(ctx context.Context, agentProfileID, targetWorkspaceID string) error {
	if a.deps.Handoff.AgentProfiles == nil {
		return fmt.Errorf("%w: agent profiles", ErrRuntimeDependencyMissing)
	}
	belongs, err := a.deps.Handoff.AgentProfiles.AgentProfileBelongsToWorkspace(ctx, agentProfileID, targetWorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to validate agent_profile_id: %w", err)
	}
	if !belongs {
		return newHandoffValidationError("agent_profile_id does not resolve in target_workspace_id")
	}
	return nil
}

// validateHandoffExecutorProfile is AC-14b's executor_profile_id predicate:
// existence only, since ExecutorProfile carries no workspace field.
func (a *Actions) validateHandoffExecutorProfile(ctx context.Context, executorProfileID string) (string, error) {
	profile, err := a.deps.Handoff.Tasks.GetExecutorProfile(ctx, executorProfileID)
	if err != nil {
		if isHandoffNotFoundError(err) {
			return "", newHandoffValidationError("executor_profile_id does not resolve")
		}
		return "", fmt.Errorf("failed to validate executor_profile_id: %w", err)
	}
	if profile == nil {
		return "", newHandoffValidationError("executor_profile_id does not resolve")
	}
	return profile.ExecutorID, nil
}

// validateHandoffRepository is AC-5b.
func (a *Actions) validateHandoffRepository(
	ctx context.Context, repositoryID, baseBranch, targetWorkspaceID string,
) ([]service.TaskRepositoryInput, error) {
	if repositoryID == "" {
		if baseBranch != "" {
			return nil, newHandoffValidationError("base_branch requires repository_id")
		}
		return nil, nil
	}
	repo, err := a.deps.Handoff.Tasks.GetRepository(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrRepositoryNotFound) {
			return nil, newHandoffValidationError("repository_id does not exist in target_workspace_id")
		}
		return nil, fmt.Errorf("failed to validate repository_id: %w", err)
	}
	if repo == nil || repo.WorkspaceID != targetWorkspaceID {
		return nil, newHandoffValidationError("repository_id does not exist in target_workspace_id")
	}
	resolvedBaseBranch := baseBranch
	if resolvedBaseBranch == "" {
		resolvedBaseBranch = repo.DefaultBranch
	}
	return []service.TaskRepositoryInput{{RepositoryID: repositoryID, BaseBranch: resolvedBaseBranch}}, nil
}

// executeHandoff is D3a step 7 (idempotency resolution and the create
// itself), branching into D3b's created-path and Found-path sequences.
func (a *Actions) executeHandoff(
	ctx context.Context, runCtx RunContext, req HandoffRequest, resolved *handoffResolvedResources,
) (*HandoffResult, error) {
	handedOffAtStr := formatHandoffTimestamp(time.Now().UTC())

	metadata := map[string]interface{}{
		taskmodels.MetaKeyAgentProfileID:    req.AgentProfileID,
		taskmodels.MetaKeyExecutorProfileID: req.ExecutorProfileID,
		taskmodels.MetaKeyHandoffSource: map[string]interface{}{
			handoffSourceTaskIDKey:    runCtx.TaskID,
			"source_workspace_id":     runCtx.WorkspaceID,
			"source_session_id":       runCtx.SessionID,
			"source_agent_profile_id": runCtx.AgentID,
			handoffHandedOffAtKey:     handedOffAtStr,
		},
	}
	if resolved.ExecutorID != "" {
		metadata[taskmodels.MetaKeyExecutorID] = resolved.ExecutorID
	}

	externalID := ""
	if req.ExternalID != nil {
		externalID = *req.ExternalID
	}
	startAgent := req.StartAgent != nil && *req.StartAgent

	result, err := a.deps.Handoff.Tasks.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:            req.TargetWorkspaceID,
		WorkflowID:             req.WorkflowID,
		WorkflowStepID:         resolved.WorkflowStepID,
		Title:                  req.Title,
		Description:            req.Prompt,
		Repositories:           resolved.Repositories,
		Metadata:               metadata,
		TrustedHandoffMetadata: true,
		StartAgent:             startAgent,
		ExternalID:             externalID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create delivery task: %w", err)
	}

	if result.Outcome != service.CreateTaskOutcomeCreated {
		return a.handleHandoffFoundOutcome(ctx, runCtx, req, result)
	}
	return a.handleHandoffCreatedOutcome(ctx, runCtx, req, resolved, result.Task, handedOffAtStr)
}

// handleHandoffCreatedOutcome is D3b steps 2-5 on the created path: settle
// the external id (AC-24d), write the reverse link (AC-17/AC-27), log
// activity (AC-19), then dispatch the launch (AC-32).
func (a *Actions) handleHandoffCreatedOutcome(
	ctx context.Context, runCtx RunContext, req HandoffRequest,
	resolved *handoffResolvedResources, task *taskmodels.Task, handedOffAtStr string,
) (*HandoffResult, error) {
	settled, survivor, settleErr := a.deps.Handoff.Tasks.SettleExternalID(ctx, task.ID, task.ExternalID)
	if settleErr != nil {
		// R-F39/F53: a settlement error halts here. No reverse link, no
		// activity, no launch. The task is not deleted.
		return nil, &HandoffSettlementError{TaskID: task.ID, cause: settleErr}
	}

	// AC-24d: the created row when settled, the survivor SettleExternalID
	// returns when not — those are different rows in general, even though
	// today's SettleExternalID implementation happens to re-fetch by the same
	// task ID.
	outcome := handoffOutcomeCreated
	settledTask := task
	if !settled {
		outcome = handoffOutcomeCreatedIdentityLost
		if survivor != nil {
			settledTask = survivor
		}
	}

	externalID := ""
	if req.ExternalID != nil {
		externalID = *req.ExternalID
	}

	reverseLink := a.ensureHandoffReverseLink(ctx, runCtx.TaskID, task.ID, req.TargetWorkspaceID, handedOffAtStr, false)
	// The activity log records task.ID (not settledTask), matching the
	// reverse link and the response's task_id above: on the identity-lost
	// path the caller is told to use task.ID, so the audit trail must name
	// the same task rather than the survivor that kept the external_id.
	a.logHandoffActivity(ctx, runCtx, task, req.TargetWorkspaceID, outcome)

	started := false
	var startError string
	startAgent := req.StartAgent != nil && *req.StartAgent
	if outcome == handoffOutcomeCreated && startAgent {
		if launchErr := a.dispatchHandoffLaunch(ctx, task, req.AgentProfileID, resolved.ExecutorID, req.ExecutorProfileID); launchErr != nil {
			startError = launchErr.Error()
		} else {
			started = true
		}
	}

	response := &HandoffResult{
		TaskID:              task.ID,
		WorkspaceID:         req.TargetWorkspaceID,
		WorkflowID:          settledTask.WorkflowID,
		WorkflowStepID:      settledTask.WorkflowStepID,
		Outcome:             outcome,
		CreationComplete:    true,
		Started:             started,
		ReverseLinkRecorded: reverseLink.recorded,
		HandedOffAt:         handedOffAtStr,
		StartError:          startError,
	}
	if !reverseLink.recorded {
		response.ReverseLinkError = reverseLink.errorMessage(externalID != "")
	}
	if outcome == handoffOutcomeCreatedIdentityLost {
		response.Message = fmt.Sprintf(
			"the delivery task exists but no longer holds external_id %q; record task_id rather than replaying, which would create a second task",
			externalID)
	}
	return response, nil
}

// handleHandoffFoundOutcome is D3b on either Found outcome: steps 2 and 5
// are skipped (AC-24a); step 3 repairs the reverse link (AC-25/AC-25a/AC-25b)
// and step 4 still logs both activity entries (AC-19).
func (a *Actions) handleHandoffFoundOutcome(
	ctx context.Context, runCtx RunContext, req HandoffRequest, result service.CreateTaskResult,
) (*HandoffResult, error) {
	found := result.Task
	outcome := handoffOutcomeFoundSettled
	if result.Outcome == service.CreateTaskOutcomeFoundUnsettled {
		outcome = handoffOutcomeFoundUnsettled
	}

	handedOffAtRaw, present, matches := handoffSourceMatches(found, runCtx.TaskID)
	if !present || !matches {
		// AC-25a: a hard refusal, not the AC-29 partial-failure shape. No
		// reverse link, no activity, no task id disclosed.
		return nil, newHandoffValidationError("external_id is already held by a task this source did not hand off")
	}

	externalID := ""
	if req.ExternalID != nil {
		externalID = *req.ExternalID
	}

	reverseLink := a.ensureHandoffReverseLink(ctx, runCtx.TaskID, found.ID, req.TargetWorkspaceID, handedOffAtRaw, true)
	a.logHandoffActivity(ctx, runCtx, found, req.TargetWorkspaceID, outcome)

	response := &HandoffResult{
		TaskID:              found.ID,
		WorkspaceID:         req.TargetWorkspaceID,
		WorkflowID:          found.WorkflowID,
		WorkflowStepID:      found.WorkflowStepID,
		Outcome:             outcome,
		CreationComplete:    result.Outcome == service.CreateTaskOutcomeFoundSettled,
		Started:             false,
		ReverseLinkRecorded: reverseLink.recorded,
		HandedOffAt:         reverseLink.handedOffAt,
	}
	if !reverseLink.recorded {
		response.ReverseLinkError = reverseLink.errorMessage(externalID != "")
	}
	if outcome == handoffOutcomeFoundUnsettled {
		response.Message = "the delivery task exists but another create may still be finishing it; " +
			"do not release external_id — proceed with the returned task_id or escalate to a human"
	}
	return response, nil
}

// handoffSourceMatches reads a found task's handoff_source metadata (AC-16)
// and reports whether it names sourceTaskID as AC-25a requires, returning
// the raw stored handed_off_at for AC-25's repair.
func handoffSourceMatches(task *taskmodels.Task, sourceTaskID string) (handedOffAtRaw string, present, matches bool) {
	src, ok := task.Metadata[taskmodels.MetaKeyHandoffSource].(map[string]interface{})
	if !ok {
		return "", false, false
	}
	storedSourceTaskID, _ := src[handoffSourceTaskIDKey].(string)
	if storedSourceTaskID != sourceTaskID {
		return "", true, false
	}
	handedOffAtRaw, _ = src[handoffHandedOffAtKey].(string)
	return handedOffAtRaw, true, true
}

// reverseLinkOutcome is the result of ensureHandoffReverseLink.
type reverseLinkOutcome struct {
	recorded    bool
	errMsg      string
	handedOffAt string
}

func (rl reverseLinkOutcome) errorMessage(externalIDSupplied bool) string {
	if externalIDSupplied {
		return rl.errMsg + "; retry an identical call with the same external_id to repair the reverse link"
	}
	return rl.errMsg + "; external_id was not supplied, so a replay would create a second delivery task — record this task_id instead"
}

// ensureHandoffReverseLink is AC-17's write (fresh, requireReadable=false)
// and AC-25's repair (requireReadable=true) in one function, since both
// reduce to "ensure an entry for deliveryTaskID exists in sourceTaskID's
// handoffs array". It performs AC-27's key-scoped compare-and-set append,
// bounded at 5 attempts, honouring AC-26's at-most-once idempotency and
// AC-25b's presence-before-readability ordering.
func (a *Actions) ensureHandoffReverseLink(
	ctx context.Context, sourceTaskID, deliveryTaskID, targetWorkspaceID, handedOffAtRaw string, requireReadable bool,
) reverseLinkOutcome {
	const maxAttempts = 5
	if a.deps.Handoff.ReverseLinks == nil {
		return reverseLinkOutcome{errMsg: "reverse-link storage is not configured"}
	}
	readable := isReadableHandoffTimestamp(handedOffAtRaw)
	respondValue := ""
	if readable {
		respondValue = handedOffAtRaw
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		currentRaw, err := a.deps.Handoff.ReverseLinks.GetTaskHandoffsRaw(ctx, sourceTaskID)
		if err != nil {
			return reverseLinkOutcome{errMsg: handoffReverseLinkReadFailureMessage(err), handedOffAt: respondValue}
		}
		entries, alreadyPresent, corrupt := parseHandoffEntries(currentRaw, deliveryTaskID)
		if corrupt != "" {
			return reverseLinkOutcome{errMsg: corrupt, handedOffAt: respondValue}
		}
		if alreadyPresent {
			return reverseLinkOutcome{recorded: true, handedOffAt: respondValue}
		}
		if requireReadable && !readable {
			return reverseLinkOutcome{errMsg: handoffReverseLinkUnreadableSuffix, handedOffAt: ""}
		}

		newEntryRaw, marshalErr := json.Marshal(map[string]interface{}{
			handoffTaskIDKey:      deliveryTaskID,
			"target_workspace_id": targetWorkspaceID,
			handoffHandedOffAtKey: handedOffAtRaw,
		})
		if marshalErr != nil {
			return reverseLinkOutcome{errMsg: "failed to encode the source task's reverse-link metadata", handedOffAt: respondValue}
		}
		entries = append(entries, handoffEntryRecord{raw: newEntryRaw, taskID: deliveryTaskID, handedOffAt: handedOffAtRaw})
		sortHandoffEntries(entries)
		rawEntries := make([]json.RawMessage, len(entries))
		for i, e := range entries {
			rawEntries[i] = e.raw
		}
		newRaw, marshalErr := json.Marshal(rawEntries)
		if marshalErr != nil {
			return reverseLinkOutcome{errMsg: "failed to encode the source task's reverse-link metadata", handedOffAt: respondValue}
		}
		stored, _, casErr := a.deps.Handoff.ReverseLinks.SetTaskHandoffsIfUnchanged(ctx, sourceTaskID, currentRaw, string(newRaw))
		if casErr != nil {
			return reverseLinkOutcome{errMsg: handoffReverseLinkWriteFailureMessage(casErr), handedOffAt: respondValue}
		}
		if stored {
			return reverseLinkOutcome{recorded: true, handedOffAt: respondValue}
		}
		// Stale value: retry from a fresh read.
	}
	return reverseLinkOutcome{
		errMsg:      "reverse-link write conflicted repeatedly across 5 attempts",
		handedOffAt: respondValue,
	}
}

func isReadableHandoffTimestamp(raw string) bool {
	if raw == "" {
		return false
	}
	_, err := time.Parse(time.RFC3339, raw)
	return err == nil
}

func handoffReverseLinkReadFailureMessage(err error) string {
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return "source task no longer exists; the delivery task was created but its reverse link could not be written"
	}
	return "failed to read the source task's reverse-link metadata"
}

func handoffReverseLinkWriteFailureMessage(err error) string {
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return "source task no longer exists; the delivery task was created but its reverse link could not be written"
	}
	return "failed to write the source task's reverse-link metadata"
}

// handoffEntryRecord pairs a handoffs-array entry's raw JSON bytes with the
// two keys needed to validate and sort it. Keeping `raw` untouched (rather
// than decoding into map[string]interface{} and re-marshaling) is what makes
// unknown fields survive an append byte-for-byte, including additive numeric
// fields outside float64's exact integer range (AC-27) that a decode/re-encode
// round trip through map[string]interface{} would silently corrupt.
type handoffEntryRecord struct {
	raw         json.RawMessage
	taskID      string
	handedOffAt string
}

// parseHandoffEntries reads the source task's raw handoffs JSON, applying
// AC-27's exhaustive corruption rules. An entry carrying additional unknown
// fields is well-formed and preserved unchanged (returned as its original
// raw bytes, untouched by decoding).
func parseHandoffEntries(raw, deliveryTaskID string) (entries []handoffEntryRecord, alreadyPresent bool, corruptMsg string) {
	const corruptMessage = "the source task's handoffs metadata is corrupt and cannot be safely appended to"
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false, ""
	}
	if trimmed == "null" {
		// A genuinely absent handoffs key is reported by the store as "", so a
		// literal "null" here means the key is *present* with an explicit null
		// value — one of AC-27's exhaustive corrupt shapes ("present but not an
		// array"), not an empty array.
		return nil, false, corruptMessage
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &rawEntries); err != nil {
		return nil, false, corruptMessage
	}
	for _, re := range rawEntries {
		var keys struct {
			TaskID      string `json:"task_id"`
			HandedOffAt string `json:"handed_off_at"`
		}
		if err := json.Unmarshal(re, &keys); err != nil {
			return nil, false, corruptMessage
		}
		if keys.TaskID == "" {
			return nil, false, corruptMessage
		}
		if _, err := time.Parse(time.RFC3339, keys.HandedOffAt); err != nil {
			return nil, false, corruptMessage
		}
		if keys.TaskID == deliveryTaskID {
			alreadyPresent = true
		}
		entries = append(entries, handoffEntryRecord{raw: re, taskID: keys.TaskID, handedOffAt: keys.HandedOffAt})
	}
	return entries, alreadyPresent, ""
}

// sortHandoffEntries is AC-28: sorted by handed_off_at as a parsed instant,
// task_id lexicographic as the tiebreak. Safe unconditionally because every
// entry here already passed parseHandoffEntries.
func sortHandoffEntries(entries []handoffEntryRecord) {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, entries[i].handedOffAt)
		tj, _ := time.Parse(time.RFC3339, entries[j].handedOffAt)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return entries[i].taskID < entries[j].taskID
	})
}

// formatHandoffTimestamp is D8's canonical write form: RFC 3339 UTC, "Z",
// millisecond precision.
func formatHandoffTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// logHandoffActivity is AC-19/AC-19a: one entry in the source workspace
// (task.handed_off) and one in the target workspace (task.handoff_received).
// Never fails the call (D6). Identity fields are read directly off RunContext
// — no run lookup, so an empty run id is written as empty rather than
// resolved or skipped (AC-19a).
func (a *Actions) logHandoffActivity(
	ctx context.Context, runCtx RunContext, deliveryTask *taskmodels.Task, targetWorkspaceID, outcome string,
) {
	if a.deps.Handoff.Activity == nil {
		return
	}
	actorID := runCtx.AgentID

	sourceDetails, _ := json.Marshal(map[string]interface{}{
		"counterpart_task_id":      deliveryTask.ID,
		"counterpart_workspace_id": targetWorkspaceID,
		"outcome":                  outcome,
	})
	a.deps.Handoff.Activity.LogActivityWithRun(ctx, runCtx.WorkspaceID, "agent", actorID, "task.handed_off",
		"task", runCtx.TaskID, string(sourceDetails), runCtx.RunID, runCtx.SessionID)

	targetDetails, _ := json.Marshal(map[string]interface{}{
		"counterpart_task_id":      runCtx.TaskID,
		"counterpart_workspace_id": runCtx.WorkspaceID,
		"outcome":                  outcome,
	})
	a.deps.Handoff.Activity.LogActivityWithRun(ctx, targetWorkspaceID, "agent", actorID, "task.handoff_received",
		"task", deliveryTask.ID, string(targetDetails), runCtx.RunID, runCtx.SessionID)
}

// dispatchHandoffLaunch is AC-32: the launch is dispatched synchronously and
// its error observed, bounded by constants.AgentLaunchTimeout — deliberately
// not a fire-and-forget path. ProfileExplicit is set per AC-14a:
// agent_profile_id is used exactly as supplied to this action, with no
// inheritance or defaulting from the destination workflow step or workflow
// default.
func (a *Actions) dispatchHandoffLaunch(ctx context.Context, task *taskmodels.Task, agentProfileID, executorID, executorProfileID string) error {
	if a.deps.Handoff.Launcher == nil {
		return errors.New("no session launcher is configured")
	}
	launchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constants.AgentLaunchTimeout)
	defer cancel()
	return a.deps.Handoff.Launcher.LaunchSession(launchCtx, HandoffLaunchRequest{
		TaskID:            task.ID,
		AgentProfileID:    agentProfileID,
		ExecutorID:        executorID,
		ExecutorProfileID: executorProfileID,
		WorkflowStepID:    task.WorkflowStepID,
		Prompt:            task.Description,
	})
}
