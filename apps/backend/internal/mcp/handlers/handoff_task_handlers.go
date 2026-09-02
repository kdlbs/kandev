package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/constants"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// handoff_task_kandev implements docs/specs/cross-workspace-task-handoff/spec.md:
// an Office-only MCP tool that creates a kanban delivery task in a workspace
// other than the caller's own. See handoff_task_tool.go (registration) and
// this file (business logic, dispatched via ws.ActionMCPHandoffTask).

const (
	handoffOutcomeCreated              = "created"
	handoffOutcomeFoundSettled         = "found_settled"
	handoffOutcomeFoundUnsettled       = "found_unsettled"
	handoffOutcomeCreatedIdentityLost  = "created_identity_lost"
	handoffForbiddenMessage            = "handoff_task_kandev requires a valid session and agent profile"
	handoffPermissionDeniedMessage     = "handoff_task_kandev requires the can_handoff_tasks permission, granted per agent or per role"
	handoffTargetIsSourceMessage       = "target_workspace_id equals your own workspace; create same-workspace tasks through POST /runtime/tasks instead"
	handoffReverseLinkUnreadableSuffix = "the source task's stored handoff_source.handed_off_at is unreadable; this handoff cannot be repaired automatically"

	// handoffSourceTaskIDKey and handoffHandedOffAtKey are the AC-16/AC-17
	// metadata field names, extracted so goconst doesn't flag the repeated
	// literals across the handoff_source write and the handoffs entry
	// read/write/sort paths.
	handoffSourceTaskIDKey = "source_task_id"
	handoffHandedOffAtKey  = "handed_off_at"
)

// handoffError carries a WS error code alongside its message so validation
// helpers below can report the exact code D3a/D3c require without every
// caller re-deriving it.
type handoffError struct {
	code    string
	message string
}

func newHandoffError(code, message string) *handoffError {
	return &handoffError{code: code, message: message}
}

// handoffTaskResult is handoff_task_kandev's response shape (AC-33). "at
// least" these fields per AC-33; message is an addition, not a replacement
// for outcome, and carries AC-24b/AC-24c's caller guidance on the two
// outcomes that need it.
type handoffTaskResult struct {
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

type handoffTaskArgs struct {
	TargetWorkspaceID string
	WorkflowID        string
	Title             string
	Prompt            string
	AgentProfileID    string
	ExecutorProfileID string
	RepositoryID      string
	BaseBranch        string
	StartAgent        bool
	ExternalID        string
}

// handoffTaskPayload is the wire shape decoded with DisallowUnknownFields so
// AC-5's unknown-argument rejection is enforced regardless of the transport.
// Optional strings are pointers so a present-but-blank value (AC-5a) can be
// told apart from an omitted one.
type handoffTaskPayload struct {
	TaskID            string  `json:"task_id"`
	SessionID         string  `json:"session_id"`
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

type handoffResolvedResources struct {
	WorkflowStepID string
	ExecutorID     string
	Repositories   []service.TaskRepositoryInput
}

// handleHandoffTask dispatches ws.ActionMCPHandoffTask. It implements D3a's
// six-step total evaluation order and D3b's five-step post-create sequence.
func (h *Handlers) handleHandoffTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if h.taskSvc == nil || h.agentSettingsCtrl == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "handoff_task_kandev is not configured", nil)
	}

	args, herr := parseHandoffTaskArgs(msg.Payload)
	if herr != nil {
		return ws.NewError(msg.ID, msg.Action, herr.code, herr.message, nil)
	}

	principal, session, herr := h.authorizeHandoffCall(ctx, args)
	if herr != nil {
		return ws.NewError(msg.ID, msg.Action, herr.code, herr.message, nil)
	}

	targetWorkspace, herr := h.authorizeHandoffTargetWorkspace(ctx, args.TargetWorkspaceID)
	if herr != nil {
		return ws.NewError(msg.ID, msg.Action, herr.code, herr.message, nil)
	}

	resolved, herr := h.resolveHandoffTargetResources(ctx, args, targetWorkspace)
	if herr != nil {
		return ws.NewError(msg.ID, msg.Action, herr.code, herr.message, nil)
	}

	return h.executeHandoff(ctx, msg, args, principal, session, resolved)
}

// parseHandoffTaskArgs is D3a step 1: argument shape, naming no target
// resource. Checks run in the R1 table's top-to-bottom order.
func parseHandoffTaskArgs(raw json.RawMessage) (*handoffTaskArgs, *handoffError) {
	var payload handoffTaskPayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		if field, ok := unknownHandoffFieldName(err); ok {
			return nil, newHandoffError(ws.ErrorCodeValidation, fmt.Sprintf("unknown argument %q", field))
		}
		return nil, newHandoffError(ws.ErrorCodeValidation, "Invalid payload: "+err.Error())
	}
	if herr := validateHandoffArgShape(&payload); herr != nil {
		return nil, herr
	}

	args := &handoffTaskArgs{
		TargetWorkspaceID: strings.TrimSpace(payload.TargetWorkspaceID),
		WorkflowID:        strings.TrimSpace(payload.WorkflowID),
		Title:             payload.Title,
		Prompt:            payload.Prompt,
		AgentProfileID:    strings.TrimSpace(payload.AgentProfileID),
		ExecutorProfileID: strings.TrimSpace(payload.ExecutorProfileID),
		StartAgent:        payload.StartAgent != nil && *payload.StartAgent,
	}
	if payload.RepositoryID != nil {
		args.RepositoryID = strings.TrimSpace(*payload.RepositoryID)
	}
	if payload.BaseBranch != nil {
		args.BaseBranch = strings.TrimSpace(*payload.BaseBranch)
	}
	if payload.ExternalID != nil {
		args.ExternalID = strings.TrimSpace(*payload.ExternalID)
	}
	return args, nil
}

func validateHandoffArgShape(payload *handoffTaskPayload) *handoffError {
	if strings.TrimSpace(payload.TargetWorkspaceID) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "target_workspace_id is required")
	}
	if strings.TrimSpace(payload.WorkflowID) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "workflow_id is required")
	}
	if strings.TrimSpace(payload.Title) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "title is required")
	}
	if titleLen := len([]rune(payload.Title)); titleLen > service.TaskTitleMaxLength {
		return newHandoffError(ws.ErrorCodeValidation,
			fmt.Sprintf("title must be %d characters or fewer (got %d)", service.TaskTitleMaxLength, titleLen))
	}
	if strings.TrimSpace(payload.Prompt) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "prompt is required")
	}
	if strings.TrimSpace(payload.AgentProfileID) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "agent_profile_id is required")
	}
	if strings.TrimSpace(payload.ExecutorProfileID) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "executor_profile_id is required")
	}
	if payload.RepositoryID != nil && strings.TrimSpace(*payload.RepositoryID) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "repository_id must not be blank when supplied")
	}
	if payload.BaseBranch != nil && strings.TrimSpace(*payload.BaseBranch) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "base_branch must not be blank when supplied")
	}
	if payload.ExternalID != nil && strings.TrimSpace(*payload.ExternalID) == "" {
		return newHandoffError(ws.ErrorCodeValidation, "external_id must not be blank when supplied")
	}
	return nil
}

func unknownHandoffFieldName(err error) (string, bool) {
	const marker = "unknown field "
	msg := err.Error()
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return "", false
	}
	field := strings.Trim(msg[idx+len(marker):], `"`)
	return field, field != ""
}

// authorizeHandoffCall is D3a steps 2-3: same-workspace refusal (AC-22) then
// the handoff permission re-derived from the trusted principal (AC-9).
func (h *Handlers) authorizeHandoffCall(ctx context.Context, args *handoffTaskArgs) (mcpscope.Principal, *models.TaskSession, *handoffError) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok {
		return mcpscope.Principal{}, nil, newHandoffError(ws.ErrorCodeForbidden, handoffForbiddenMessage)
	}
	if args.TargetWorkspaceID == principal.WorkspaceID {
		return mcpscope.Principal{}, nil, newHandoffError(ws.ErrorCodeValidation, handoffTargetIsSourceMessage)
	}
	if principal.CallerTaskID == "" || principal.CallerSessionID == "" {
		return mcpscope.Principal{}, nil, newHandoffError(ws.ErrorCodeForbidden, handoffForbiddenMessage)
	}

	session, err := h.sessionRepo.GetTaskSession(ctx, principal.CallerSessionID)
	if err != nil || session == nil || session.TaskID != principal.CallerTaskID {
		return mcpscope.Principal{}, nil, newHandoffError(ws.ErrorCodeForbidden, handoffForbiddenMessage)
	}
	granted, err := h.agentSettingsCtrl.AgentHasHandoffPermission(ctx, session.AgentProfileID)
	if err != nil {
		h.logger.Error("handoff_task_kandev: failed to resolve handoff permission", zap.Error(err))
		return mcpscope.Principal{}, nil, newHandoffError(ws.ErrorCodeForbidden, handoffForbiddenMessage)
	}
	if !granted {
		return mcpscope.Principal{}, nil, newHandoffError(ws.ErrorCodeForbidden, handoffPermissionDeniedMessage)
	}
	return principal, session, nil
}

// authorizeHandoffTargetWorkspace is D3a step 4 (AC-11): the existing
// per-user workspace scoping, indistinguishable from not-found on denial.
func (h *Handlers) authorizeHandoffTargetWorkspace(ctx context.Context, targetWorkspaceID string) (*models.Workspace, *handoffError) {
	workspace, err := h.taskSvc.GetWorkspace(ctx, targetWorkspaceID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrWorkspaceNotFound) || errors.Is(err, sql.ErrNoRows) {
			return nil, newHandoffError(ws.ErrorCodeNotFound, fmt.Sprintf("target_workspace_id %q was not found", targetWorkspaceID))
		}
		h.logger.Error("handoff_task_kandev: failed to load target workspace",
			zap.String("workspace_id", targetWorkspaceID), zap.Error(err))
		return nil, newHandoffError(ws.ErrorCodeInternalError, "failed to validate target_workspace_id")
	}
	return workspace, nil
}

// resolveHandoffTargetResources is D3a step 5, in its stated order:
// workflow_id, destination step, agent_profile_id, executor_profile_id,
// repository_id, base_branch.
func (h *Handlers) resolveHandoffTargetResources(
	ctx context.Context, args *handoffTaskArgs, workspace *models.Workspace,
) (*handoffResolvedResources, *handoffError) {
	if herr := h.validateHandoffWorkflow(ctx, args.WorkflowID, workspace); herr != nil {
		return nil, herr
	}
	stepID, herr := h.resolveHandoffDestinationStep(ctx, args.WorkflowID, args.StartAgent)
	if herr != nil {
		return nil, herr
	}
	if herr := h.validateHandoffAgentProfile(ctx, args.AgentProfileID, workspace.ID); herr != nil {
		return nil, herr
	}
	executorID, herr := h.validateHandoffExecutorProfile(ctx, args.ExecutorProfileID)
	if herr != nil {
		return nil, herr
	}
	repos, herr := h.validateHandoffRepository(ctx, args.RepositoryID, args.BaseBranch, workspace.ID)
	if herr != nil {
		return nil, herr
	}
	return &handoffResolvedResources{WorkflowStepID: stepID, ExecutorID: executorID, Repositories: repos}, nil
}

// validateHandoffWorkflow is AC-12/AC-12a/AC-12b/AC-12c. It deliberately does
// not reuse validateMCPWorkflowWorkspace's message, which discloses the
// workflow's actual owning workspace (AC-12a).
func (h *Handlers) validateHandoffWorkflow(ctx context.Context, workflowID string, workspace *models.Workspace) *handoffError {
	const notAMemberMessage = "workflow_id is not a workflow of the target workspace"
	workflow, err := h.taskSvc.GetWorkflow(ctx, workflowID)
	if err != nil {
		if isMCPWorkflowNotFoundError(err) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return newHandoffError(ws.ErrorCodeValidation, notAMemberMessage)
		}
		h.logger.Error("handoff_task_kandev: failed to load workflow_id", zap.String("workflow_id", workflowID), zap.Error(err))
		return newHandoffError(ws.ErrorCodeInternalError, "failed to validate workflow_id")
	}
	if workflow.WorkspaceID != workspace.ID {
		return newHandoffError(ws.ErrorCodeValidation, notAMemberMessage)
	}
	if workspace.OfficeWorkflowID != "" && workflowID == workspace.OfficeWorkflowID {
		return newHandoffError(ws.ErrorCodeValidation,
			"workflow_id must be a delivery workflow, not the target workspace's office workflow")
	}
	return nil
}

// resolveHandoffDestinationStep is AC-15a/AC-15b/AC-15c: a deterministic,
// (position, id)-ordered selection distinct from resolveMCPDestinationStep's
// bare empty-string-on-anything return, so configuration and failure are
// distinguishable.
func (h *Handlers) resolveHandoffDestinationStep(ctx context.Context, workflowID string, startAgent bool) (string, *handoffError) {
	if h.workflowCtrl == nil {
		return "", newHandoffError(ws.ErrorCodeInternalError, "workflow steps are not available")
	}
	resp, err := h.workflowCtrl.ListStepsByWorkflow(ctx, workflowctrl.ListStepsRequest{WorkflowID: workflowID})
	if err != nil {
		h.logger.Error("handoff_task_kandev: failed to list workflow steps", zap.String("workflow_id", workflowID), zap.Error(err))
		return "", newHandoffError(ws.ErrorCodeInternalError, "failed to resolve destination step")
	}
	var steps []*workflowmodels.WorkflowStep
	if resp != nil {
		steps = resp.Steps
	}
	step := selectHandoffDestinationStep(steps, startAgent)
	if step == nil {
		return "", newHandoffError(ws.ErrorCodeValidation, "workflow_id has no resolvable destination step")
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
func (h *Handlers) validateHandoffAgentProfile(ctx context.Context, agentProfileID, targetWorkspaceID string) *handoffError {
	belongs, err := h.agentSettingsCtrl.AgentProfileBelongsToWorkspace(ctx, agentProfileID, targetWorkspaceID)
	if err != nil {
		h.logger.Error("handoff_task_kandev: failed to validate agent_profile_id",
			zap.String("agent_profile_id", agentProfileID), zap.Error(err))
		return newHandoffError(ws.ErrorCodeInternalError, "failed to validate agent_profile_id")
	}
	if !belongs {
		return newHandoffError(ws.ErrorCodeValidation, "agent_profile_id does not resolve in target_workspace_id")
	}
	return nil
}

// validateHandoffExecutorProfile is AC-14b's executor_profile_id predicate:
// existence only, since ExecutorProfile carries no workspace field.
func (h *Handlers) validateHandoffExecutorProfile(ctx context.Context, executorProfileID string) (string, *handoffError) {
	profile, err := h.taskSvc.GetExecutorProfile(ctx, executorProfileID)
	if err != nil {
		if isMCPExecutorProfileNotFoundError(err) {
			return "", newHandoffError(ws.ErrorCodeValidation, "executor_profile_id does not resolve")
		}
		h.logger.Error("handoff_task_kandev: failed to validate executor_profile_id",
			zap.String("executor_profile_id", executorProfileID), zap.Error(err))
		return "", newHandoffError(ws.ErrorCodeInternalError, "failed to validate executor_profile_id")
	}
	if profile == nil {
		return "", newHandoffError(ws.ErrorCodeValidation, "executor_profile_id does not resolve")
	}
	return profile.ExecutorID, nil
}

func isMCPExecutorProfileNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "executor profile not found")
}

// validateHandoffRepository is AC-5b.
func (h *Handlers) validateHandoffRepository(
	ctx context.Context, repositoryID, baseBranch, targetWorkspaceID string,
) ([]service.TaskRepositoryInput, *handoffError) {
	if repositoryID == "" {
		if baseBranch != "" {
			return nil, newHandoffError(ws.ErrorCodeValidation, "base_branch requires repository_id")
		}
		return nil, nil
	}
	repo, err := h.taskSvc.GetRepository(ctx, repositoryID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrRepositoryNotFound) {
			return nil, newHandoffError(ws.ErrorCodeValidation, "repository_id does not exist in target_workspace_id")
		}
		h.logger.Error("handoff_task_kandev: failed to validate repository_id",
			zap.String("repository_id", repositoryID), zap.Error(err))
		return nil, newHandoffError(ws.ErrorCodeInternalError, "failed to validate repository_id")
	}
	if repo == nil || repo.WorkspaceID != targetWorkspaceID {
		return nil, newHandoffError(ws.ErrorCodeValidation, "repository_id does not exist in target_workspace_id")
	}
	resolvedBaseBranch := baseBranch
	if resolvedBaseBranch == "" {
		resolvedBaseBranch = repo.DefaultBranch
	}
	return []service.TaskRepositoryInput{{RepositoryID: repositoryID, BaseBranch: resolvedBaseBranch}}, nil
}

// executeHandoff is D3a step 6 (idempotency resolution and the create
// itself), branching into D3b's created-path and Found-path sequences.
func (h *Handlers) executeHandoff(
	ctx context.Context, msg *ws.Message, args *handoffTaskArgs,
	principal mcpscope.Principal, session *models.TaskSession, resolved *handoffResolvedResources,
) (*ws.Message, error) {
	handedOffAtStr := formatHandoffTimestamp(time.Now().UTC())

	metadata := map[string]interface{}{
		models.MetaKeyAgentProfileID:    args.AgentProfileID,
		models.MetaKeyExecutorProfileID: args.ExecutorProfileID,
		models.MetaKeyHandoffSource: map[string]interface{}{
			handoffSourceTaskIDKey:    principal.CallerTaskID,
			"source_workspace_id":     principal.WorkspaceID,
			"source_session_id":       principal.CallerSessionID,
			"source_agent_profile_id": session.AgentProfileID,
			handoffHandedOffAtKey:     handedOffAtStr,
		},
	}
	if resolved.ExecutorID != "" {
		metadata[models.MetaKeyExecutorID] = resolved.ExecutorID
	}

	result, err := h.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    args.TargetWorkspaceID,
		WorkflowID:     args.WorkflowID,
		WorkflowStepID: resolved.WorkflowStepID,
		Title:          args.Title,
		Description:    args.Prompt,
		Repositories:   resolved.Repositories,
		Metadata:       metadata,
		StartAgent:     args.StartAgent,
		ExternalID:     args.ExternalID,
	})
	if err != nil {
		h.logger.Error("handoff_task_kandev: failed to create delivery task", zap.Error(err))
		code := classifyCreateTaskError(err)
		message := "Failed to create delivery task"
		if code != ws.ErrorCodeInternalError {
			message = err.Error()
		}
		return ws.NewError(msg.ID, msg.Action, code, message, nil)
	}

	if result.Outcome != service.CreateTaskOutcomeCreated {
		return h.handleHandoffFoundOutcome(ctx, msg, principal, session, args, result)
	}
	return h.handleHandoffCreatedOutcome(ctx, msg, principal, session, args, resolved, result.Task, handedOffAtStr)
}

// handleHandoffCreatedOutcome is D3b steps 2-5 on the created path: settle
// the external id (AC-24d), write the reverse link (AC-17/AC-27), log
// activity (AC-19), then dispatch the launch (AC-32).
func (h *Handlers) handleHandoffCreatedOutcome(
	ctx context.Context, msg *ws.Message,
	principal mcpscope.Principal, session *models.TaskSession, args *handoffTaskArgs,
	resolved *handoffResolvedResources, task *models.Task, handedOffAtStr string,
) (*ws.Message, error) {
	settled, survivor, settleErr := h.taskSvc.SettleExternalID(ctx, task.ID, task.ExternalID)
	if settleErr != nil {
		// R-F39: a settlement error halts here. No reverse link, no
		// activity, no launch.
		h.logger.Error("handoff_task_kandev: failed to settle external_id", zap.String("task_id", task.ID), zap.Error(settleErr))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			fmt.Sprintf("delivery task %s was created but could not settle its external_id", task.ID), nil)
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

	reverseLink := h.ensureHandoffReverseLink(ctx, principal.CallerTaskID, task.ID, args.TargetWorkspaceID, handedOffAtStr, false)
	h.logHandoffActivity(ctx, principal, session, settledTask, args.TargetWorkspaceID, outcome)

	started := false
	var startError string
	if outcome == handoffOutcomeCreated && args.StartAgent {
		if launchErr := h.dispatchHandoffLaunch(ctx, task, args.AgentProfileID, resolved.ExecutorID, args.ExecutorProfileID); launchErr != nil {
			startError = launchErr.Error()
		} else {
			started = true
		}
	}

	response := handoffTaskResult{
		TaskID:              task.ID,
		WorkspaceID:         args.TargetWorkspaceID,
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
		response.ReverseLinkError = reverseLink.errorMessage(args.ExternalID != "")
	}
	if outcome == handoffOutcomeCreatedIdentityLost {
		response.Message = fmt.Sprintf(
			"the delivery task exists but no longer holds external_id %q; record task_id rather than replaying, which would create a second task",
			args.ExternalID)
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

// handleHandoffFoundOutcome is D3b on either Found outcome: steps 2 and 5
// are skipped (AC-24a); step 3 repairs the reverse link (AC-25/AC-25a/AC-25b)
// and step 4 still logs both activity entries (AC-19).
func (h *Handlers) handleHandoffFoundOutcome(
	ctx context.Context, msg *ws.Message,
	principal mcpscope.Principal, session *models.TaskSession, args *handoffTaskArgs,
	result service.CreateTaskResult,
) (*ws.Message, error) {
	found := result.Task
	outcome := handoffOutcomeFoundSettled
	if result.Outcome == service.CreateTaskOutcomeFoundUnsettled {
		outcome = handoffOutcomeFoundUnsettled
	}

	handedOffAtRaw, present, matches := handoffSourceMatches(found, principal.CallerTaskID)
	if !present || !matches {
		// AC-25a: a hard refusal, not the AC-29 partial-failure shape. No
		// reverse link, no activity, no task id disclosed.
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"external_id is already held by a task this source did not hand off", nil)
	}

	reverseLink := h.ensureHandoffReverseLink(ctx, principal.CallerTaskID, found.ID, args.TargetWorkspaceID, handedOffAtRaw, true)
	h.logHandoffActivity(ctx, principal, session, found, args.TargetWorkspaceID, outcome)

	response := handoffTaskResult{
		TaskID:              found.ID,
		WorkspaceID:         args.TargetWorkspaceID,
		WorkflowID:          found.WorkflowID,
		WorkflowStepID:      found.WorkflowStepID,
		Outcome:             outcome,
		CreationComplete:    result.Outcome == service.CreateTaskOutcomeFoundSettled,
		Started:             false,
		ReverseLinkRecorded: reverseLink.recorded,
		HandedOffAt:         reverseLink.handedOffAt,
	}
	if !reverseLink.recorded {
		response.ReverseLinkError = reverseLink.errorMessage(args.ExternalID != "")
	}
	if outcome == handoffOutcomeFoundUnsettled {
		response.Message = "the delivery task exists but another create may still be finishing it; " +
			"do not release external_id — proceed with the returned task_id or escalate to a human"
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

// handoffSourceMatches reads a found task's handoff_source metadata (AC-16)
// and reports whether it names sourceTaskID as AC-25a requires, returning
// the raw stored handed_off_at for AC-25's repair.
func handoffSourceMatches(task *models.Task, sourceTaskID string) (handedOffAtRaw string, present, matches bool) {
	src, ok := task.Metadata[models.MetaKeyHandoffSource].(map[string]interface{})
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
func (h *Handlers) ensureHandoffReverseLink(
	ctx context.Context, sourceTaskID, deliveryTaskID, targetWorkspaceID, handedOffAtRaw string, requireReadable bool,
) reverseLinkOutcome {
	const maxAttempts = 5
	readable := isReadableHandoffTimestamp(handedOffAtRaw)
	respondValue := ""
	if readable {
		respondValue = handedOffAtRaw
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		currentRaw, err := h.taskRepo.GetTaskHandoffsRaw(ctx, sourceTaskID)
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

		entries = append(entries, map[string]interface{}{
			keyTaskID:             deliveryTaskID,
			"target_workspace_id": targetWorkspaceID,
			handoffHandedOffAtKey: handedOffAtRaw,
		})
		sortHandoffEntries(entries)
		newRaw, marshalErr := json.Marshal(entries)
		if marshalErr != nil {
			return reverseLinkOutcome{errMsg: "failed to encode the source task's reverse-link metadata", handedOffAt: respondValue}
		}
		stored, _, casErr := h.taskRepo.SetTaskHandoffsIfUnchanged(ctx, sourceTaskID, currentRaw, string(newRaw))
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
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return "source task no longer exists; the delivery task was created but its reverse link could not be written"
	}
	return "failed to read the source task's reverse-link metadata"
}

func handoffReverseLinkWriteFailureMessage(err error) string {
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return "source task no longer exists; the delivery task was created but its reverse link could not be written"
	}
	return "failed to write the source task's reverse-link metadata"
}

// parseHandoffEntries reads the source task's raw handoffs JSON, applying
// AC-27's exhaustive corruption rules. An entry carrying additional unknown
// fields is well-formed and preserved unchanged (returned as the full map).
func parseHandoffEntries(raw, deliveryTaskID string) (entries []map[string]interface{}, alreadyPresent bool, corruptMsg string) {
	const corruptMessage = "the source task's handoffs metadata is corrupt and cannot be safely appended to"
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false, ""
	}
	if trimmed == "null" {
		// A genuinely absent handoffs key is reported by handoffsRawValue as
		// "" (handled above), so a literal "null" here means the key is
		// *present* with an explicit null value — one of AC-27's exhaustive
		// corrupt shapes ("present but not an array"), not an empty array.
		return nil, false, corruptMessage
	}
	var rawEntries []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &rawEntries); err != nil {
		return nil, false, corruptMessage
	}
	for _, re := range rawEntries {
		var entry map[string]interface{}
		if err := json.Unmarshal(re, &entry); err != nil {
			return nil, false, corruptMessage
		}
		taskID, _ := entry[keyTaskID].(string)
		handedOffAt, ok := entry[handoffHandedOffAtKey].(string)
		if taskID == "" || !ok {
			return nil, false, corruptMessage
		}
		if _, err := time.Parse(time.RFC3339, handedOffAt); err != nil {
			return nil, false, corruptMessage
		}
		if taskID == deliveryTaskID {
			alreadyPresent = true
		}
		entries = append(entries, entry)
	}
	return entries, alreadyPresent, ""
}

// sortHandoffEntries is AC-28/R-F37(3): sorted by handed_off_at as a parsed
// instant, task_id lexicographic as the tiebreak. Safe unconditionally
// because every entry here already passed parseHandoffEntries.
func sortHandoffEntries(entries []map[string]interface{}) {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, entries[i][handoffHandedOffAtKey].(string))
		tj, _ := time.Parse(time.RFC3339, entries[j][handoffHandedOffAtKey].(string))
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return entries[i][keyTaskID].(string) < entries[j][keyTaskID].(string)
	})
}

// formatHandoffTimestamp is D8's canonical write form: RFC 3339 UTC, "Z",
// millisecond precision.
func formatHandoffTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// logHandoffActivity is AC-19/AC-19a: one entry in the source workspace
// (task.handed_off) and one in the target workspace (task.handoff_received).
// Never fails the call (D6) — LogActivityWithRun swallows its own errors.
func (h *Handlers) logHandoffActivity(
	ctx context.Context, principal mcpscope.Principal, session *models.TaskSession,
	deliveryTask *models.Task, targetWorkspaceID, outcome string,
) {
	if h.dashboardSvc == nil {
		return
	}
	runID := h.dashboardSvc.ResolveRunForTaskAndSession(ctx, principal.CallerTaskID, principal.CallerSessionID)
	actorID := session.AgentProfileID

	sourceDetails, _ := json.Marshal(map[string]interface{}{
		"counterpart_task_id":      deliveryTask.ID,
		"counterpart_workspace_id": targetWorkspaceID,
		"outcome":                  outcome,
	})
	h.dashboardSvc.LogActivityWithRun(ctx, principal.WorkspaceID, "agent", actorID, "task.handed_off",
		"task", principal.CallerTaskID, string(sourceDetails), runID, principal.CallerSessionID)

	targetDetails, _ := json.Marshal(map[string]interface{}{
		"counterpart_task_id":      principal.CallerTaskID,
		"counterpart_workspace_id": principal.WorkspaceID,
		"outcome":                  outcome,
	})
	h.dashboardSvc.LogActivityWithRun(ctx, targetWorkspaceID, "agent", actorID, "task.handoff_received",
		"task", deliveryTask.ID, string(targetDetails), runID, principal.CallerSessionID)
}

// dispatchHandoffLaunch is AC-32: the launch is dispatched synchronously and
// its error observed, bounded by constants.AgentLaunchTimeout — deliberately
// not launchAutoStartTask, which is fire-and-forget. ProfileExplicit is set
// per AC-14a: agent_profile_id is used exactly as supplied to this tool, with
// no inheritance or defaulting from the destination workflow step or
// workflow default — without it, the orchestrator's resolveEffectiveAgentProfile
// would silently substitute the step's pinned profile.
func (h *Handlers) dispatchHandoffLaunch(ctx context.Context, task *models.Task, agentProfileID, executorID, executorProfileID string) error {
	if h.sessionLauncher == nil {
		return errors.New("no session launcher is configured")
	}
	launchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constants.AgentLaunchTimeout)
	defer cancel()
	_, err := h.sessionLauncher.LaunchSession(launchCtx, &orchestrator.LaunchSessionRequest{
		TaskID:            task.ID,
		Intent:            orchestrator.IntentStart,
		AgentProfileID:    agentProfileID,
		ProfileExplicit:   true,
		ExecutorID:        executorID,
		ExecutorProfileID: executorProfileID,
		WorkflowStepID:    task.WorkflowStepID,
		Prompt:            task.Description,
	})
	return err
}
