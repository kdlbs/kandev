package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/coordinator"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// coordinatorSurfaceActions is the execution-time mirror of the fixed
// seven-tool coordinator catalog (docs/specs/coordinator/system-design/
// copilot-tools.md#tool-surface, AC-COORDINATOR-COPILOT-003.1). Discovery
// alone is not an authorization boundary because an agent can still send a
// raw WebSocket action.
var coordinatorSurfaceActions = map[string]struct{}{
	ws.ActionMCPListTasks:            {},
	ws.ActionMCPGetTaskConversation:  {},
	ws.ActionMCPListWorkflows:        {},
	ws.ActionMCPListWorkflowSteps:    {},
	ws.ActionMCPListRepositories:     {},
	coordinator.ActionProposeTask:    {},
	coordinator.ActionProposeResume:  {},
	coordinator.ActionProposeMessage: {},
	coordinator.ActionProposeMove:    {},
	coordinator.ActionGetItem:        {},
	coordinator.ActionListActivity:   {},

	coordinator.ActionProposeImprovement: {},
	coordinator.ActionListTurns:          {},
}

// coordinatorPrincipalOnlyActions are registered coordinator-surface actions
// that must never be reachable by a non-coordinator principal, even though
// they are allowlisted above: coordinator.propose_task and
// coordinator.get_item both dispatch to handlers that trust the principal's
// own WorkspaceID/CoordinatorID rather than any payload field
// (copilot-tools.md#tool-surface).
var coordinatorPrincipalOnlyActions = map[string]struct{}{
	coordinator.ActionProposeTask:    {},
	coordinator.ActionProposeResume:  {},
	coordinator.ActionProposeMessage: {},
	coordinator.ActionProposeMove:    {},
	coordinator.ActionGetItem:        {},
	coordinator.ActionListActivity:   {},

	coordinator.ActionProposeImprovement: {},
	coordinator.ActionListTurns:          {},
}

// authorizeCoordinatorRequest is the one execution-time boundary for the
// fixed coordinator surface: a coordinator principal may call only the seven
// allowlisted actions, each scoped to its own workspace, and the two
// coordinatorPrincipalOnlyActions (coordinator.propose_task,
// coordinator.get_item) may only be called by a coordinator principal.
// Before either of those checks, the reserved decision action names
// (coordinator.DecisionActions) are refused for a coordinator principal or an
// unresolved (no) principal, per
// docs/specs/coordinator/system-design/proposals.md#security; an ordinary
// principal is left untouched here and reaches the dispatcher, which answers
// the same unregistered action as unknown.
//
// Cross-workspace checks on coordinator.propose_task's own workflow_id,
// step_id, repository_id and source_task_id fields are deliberately left to
// coordinator.Service.ProposeTask, which returns a *FieldError naming the
// offending field (AC-COORDINATOR-PROPOSALS-001.3) rather than this guard's
// blanket not-found. coordinator.get_item's own kind/id validation and
// per-kind scope resolution are likewise left to its handler
// (copilot-tools.md#item-read).
func (h *Handlers) authorizeCoordinatorRequest(ctx context.Context, msg *ws.Message) (*ws.Message, *ws.Message, error) {
	principal, hasPrincipal := mcpscope.PrincipalFromContext(ctx)
	isCoordinator := hasPrincipal && principal.IsCoordinator()

	if _, reserved := coordinator.DecisionActions[msg.Action]; reserved && (isCoordinator || !hasPrincipal) {
		return coordinatorUnknownAction(msg)
	}
	if msg.Action == coordinator.ActionProposeImprovement && (h.coordinatorSvc == nil || !h.coordinatorSvc.Phase3Enabled()) {
		return coordinatorUnknownAction(msg)
	}
	if msg.Action == coordinator.ActionListTurns && (h.coordinatorSvc == nil || !h.coordinatorSvc.Phase31Enabled() || h.coordinatorTurns == nil) {
		return coordinatorUnknownAction(msg)
	}
	if _, principalOnly := coordinatorPrincipalOnlyActions[msg.Action]; principalOnly && !isCoordinator {
		return coordinatorUnknownAction(msg)
	}
	if !isCoordinator {
		return nil, msg, nil
	}
	return h.authorizeCoordinatorSurface(ctx, principal, msg)
}

// authorizeCoordinatorSurface runs the checks for a coordinator principal
// once the reserved-name and principal-only refusals have passed: the phase-2
// policy checks, the fixed surface allowlist, then payload, reference and
// watch checks.
func (h *Handlers) authorizeCoordinatorSurface(
	ctx context.Context, principal mcpscope.Principal, msg *ws.Message,
) (*ws.Message, *ws.Message, error) {
	phase2 := h.coordinatorSvc != nil && h.coordinatorSvc.Phase2()
	if phase2 && principal.WorkspaceID != "" {
		if refused, response, err := h.authorizeCoordinatorPolicy(ctx, principal, msg); refused {
			return response, nil, err
		}
	}
	if _, allowed := coordinatorSurfaceActions[msg.Action]; !allowed {
		return coordinatorUnknownAction(msg)
	}
	if h.taskSvc == nil || principal.WorkspaceID == "" {
		return coordinatorNotFound(msg)
	}

	fields, err := automationPayloadFields(msg.Payload)
	if err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest,
			"Invalid payload: "+err.Error(), nil)
		return response, nil, responseErr
	}
	if !h.authorizeCoordinatorReferenceFields(ctx, principal, msg.Action, fields) {
		return coordinatorNotFound(msg)
	}
	if phase2 && !h.coordinatorWatchesFields(ctx, principal, msg.Action, fields) {
		return coordinatorNotFound(msg)
	}
	return nil, msg, nil
}

// phaseOneToolNames is the tool list of a conversation that has no binding.
var phaseOneToolNames = coordinator.ToolNames(coordinator.PhaseOnePolicy(), false, false)

// actionClass is the activity class a refused request is recorded under: the
// policy action of a propose tool, otherwise unknown.
func actionClass(action string) coordinator.Action {
	if action == coordinator.ActionProposeImprovement {
		return coordinator.ActionImprovement
	}
	if tool, ok := coordinator.ToolForAction(action); ok {
		if class, isPropose := coordinator.ProposeActionFor(tool); isPropose {
			return class
		}
	}
	return coordinator.ActionUnknown
}

// authorizeCoordinatorPolicy runs the phase-2 checks that precede payload
// validation: the bound tool list is resolved and valid, names the requested
// tool, and, for a propose tool, the coordinator's live policy still allows
// the action. Every refusal but a failed read records one activity row under
// the principal's own ids; a failed read fails closed without a row.
func (h *Handlers) authorizeCoordinatorPolicy(
	ctx context.Context, principal mcpscope.Principal, msg *ws.Message,
) (bool, *ws.Message, error) {
	class := actionClass(msg.Action)
	names, valid := coordinatorBoundNames(ctx, principal)
	if !valid {
		return h.refuseCoordinator(ctx, principal, msg, class, "binding_invalid")
	}
	tool, hasTool := coordinator.ToolForAction(msg.Action)
	if !hasTool || !slices.Contains(names, tool) {
		return h.refuseCoordinator(ctx, principal, msg, class, "not_in_profile")
	}
	action, isPropose := coordinator.ProposeActionFor(tool)
	if !isPropose {
		return false, nil, nil
	}
	allowed, err := h.coordinatorSvc.ActionAllowed(ctx, principal.CoordinatorID, action)
	if err != nil {
		h.logger.Error("coordinator policy read failed; refusing", zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		response, _, respErr := coordinatorNotFound(msg)
		return true, response, respErr
	}
	if !allowed {
		return h.refuseCoordinator(ctx, principal, msg, class, "policy_denied")
	}
	return false, nil, nil
}

// coordinatorBoundNames resolves the tool list the conversation is bound to.
// A conversation with no binding uses the phase-1 seven; a binding that is
// required but absent, invalid, or issued for a different identity is not
// valid.
func coordinatorBoundNames(ctx context.Context, principal mcpscope.Principal) ([]string, bool) {
	execution, ok := streams.MCPExecutionContextFromContext(ctx)
	if !ok {
		return nil, false
	}
	binding := execution.CoordinatorToolPolicy
	if binding == nil {
		return phaseOneToolNames, !execution.CoordinatorToolPolicyRequired
	}
	if !coordinatorBindingMatches(binding, principal) {
		return nil, false
	}
	return binding.ToolNames, true
}

func coordinatorBindingMatches(binding *mcpprofile.CoordinatorToolPolicy, principal mcpscope.Principal) bool {
	return binding.Validate() == nil &&
		binding.CoordinatorID == principal.CoordinatorID &&
		binding.WorkspaceID == principal.WorkspaceID &&
		binding.ConversationTaskID == principal.CallerTaskID
}

func (h *Handlers) refuseCoordinator(
	ctx context.Context, principal mcpscope.Principal, msg *ws.Message, class coordinator.Action, reason string,
) (bool, *ws.Message, error) {
	if err := h.coordinatorSvc.RecordRefusal(ctx, principal.CoordinatorID, principal.WorkspaceID, class, reason); err != nil {
		h.logger.Error("coordinator refusal not recorded",
			zap.String("coordinator_id", principal.CoordinatorID), zap.String("reason", reason), zap.Error(err))
	}
	response, _, err := coordinatorUnknownAction(msg)
	return true, response, err
}

// coordinatorWatchesFields applies the effective watch set to the workflow a
// read names, directly or through its task (a stall item names its task by
// id). A read naming neither is not filtered here. A task without a workflow
// is never watched, and a failed read fails closed.
func (h *Handlers) coordinatorWatchesFields(
	ctx context.Context, principal mcpscope.Principal, action string, fields map[string]json.RawMessage,
) bool {
	if isCoordinatorProposeAction(action) {
		return true
	}
	workflowID := jsonStringField(fields, "workflow_id")
	taskID := jsonStringField(fields, "task_id")
	if action == coordinator.ActionGetItem && jsonStringField(fields, coordinatorItemFieldKind) == coordinatorItemKindStall {
		taskID = strings.TrimSpace(jsonStringField(fields, "id"))
	}
	if taskID == "" && workflowID == "" {
		return true
	}
	if taskID != "" {
		task, err := h.taskSvc.GetTask(ctx, taskID)
		if err != nil || task == nil {
			return false
		}
		return h.coordinatorWatchesTask(ctx, principal, task.ID, task.WorkflowID)
	}
	return h.coordinatorWatchesWorkflow(ctx, principal, workflowID)
}

// coordinatorWatchesTask reports whether the task is in the coordinator's
// Watches, workflow and projects together; an unreadable scope refuses.
func (h *Handlers) coordinatorWatchesTask(ctx context.Context, principal mcpscope.Principal, taskID, workflowID string) bool {
	watched, err := h.coordinatorSvc.TaskWatched(ctx, principal.CoordinatorID, taskID, workflowID)
	if err != nil {
		h.logger.Error("coordinator watch scope read failed; refusing", zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return false
	}
	return watched
}

// coordinatorWatchesWorkflow reports whether the workflow is in the
// coordinator's effective watch set; an unreadable set refuses.
func (h *Handlers) coordinatorWatchesWorkflow(ctx context.Context, principal mcpscope.Principal, workflowID string) bool {
	set, err := h.coordinatorSvc.EffectiveWatchSet(ctx, principal.CoordinatorID)
	if err != nil {
		h.logger.Error("coordinator watch set read failed; refusing", zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return false
	}
	return set.Contains(workflowID)
}

// coordinatorWatchFilter is the effective watch set a coordinator principal's
// enumerating read is filtered by, or nil when the read is not filtered.
func (h *Handlers) coordinatorWatchFilter(ctx context.Context) (*coordinator.WatchSet, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() || h.coordinatorSvc == nil || !h.coordinatorSvc.Phase2() {
		return nil, nil
	}
	set, err := h.coordinatorSvc.EffectiveWatchSet(ctx, principal.CoordinatorID)
	if err != nil {
		return nil, err
	}
	return &set, nil
}

// authorizeCoordinatorReferenceFields checks the workspace_id, workflow_id
// and task_id fields a coordinator-surface read tool can carry
// (list_workflows_kandev / list_repositories_kandev's workspace_id;
// list_workflow_steps_kandev / list_tasks_kandev's workflow_id;
// get_task_conversation_kandev's task_id, in defense-in-depth alongside the
// WS gateway dispatch backstop). It is skipped entirely for
// coordinator.propose_task, whose own field validation runs in
// coordinator.Service.ProposeTask.
func (h *Handlers) authorizeCoordinatorReferenceFields(
	ctx context.Context,
	principal mcpscope.Principal,
	action string,
	fields map[string]json.RawMessage,
) bool {
	if isCoordinatorProposeAction(action) {
		return true
	}
	if workspaceID := jsonStringField(fields, "workspace_id"); workspaceID != "" && workspaceID != principal.WorkspaceID {
		return false
	}
	if workflowID := jsonStringField(fields, "workflow_id"); workflowID != "" {
		workflow, err := h.taskSvc.GetWorkflow(ctx, workflowID)
		if err != nil || workflow == nil || workflow.WorkspaceID != principal.WorkspaceID {
			return false
		}
	}
	if taskID := jsonStringField(fields, "task_id"); taskID != "" {
		task, err := h.taskSvc.GetTask(ctx, taskID)
		if err != nil || task == nil || task.WorkspaceID != principal.WorkspaceID {
			return false
		}
	}
	return true
}

func coordinatorUnknownAction(msg *ws.Message) (*ws.Message, *ws.Message, error) {
	response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction,
		"tool is not available on the coordinator MCP surface", nil)
	return response, nil, err
}

func coordinatorNotFound(msg *ws.Message) (*ws.Message, *ws.Message, error) {
	response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
	return response, nil, err
}

// isCoordinatorProposeAction reports whether the action is one of the four
// propose tools, whose own field and target validation runs in the coordinator
// service and names the offending field.
func isCoordinatorProposeAction(action string) bool {
	if action == coordinator.ActionProposeImprovement {
		return true
	}
	tool, ok := coordinator.ToolForAction(action)
	if !ok {
		return false
	}
	_, isPropose := coordinator.ProposeActionFor(tool)
	return isPropose
}

// filterCoordinatorTasks drops the tasks outside a coordinator principal's
// projects. Other callers, and a coordinator without phase 2, see the list
// unchanged. A resolver failure is an error naming projects.
func (h *Handlers) filterCoordinatorTasks(ctx context.Context, tasks []*taskmodels.Task) ([]*taskmodels.Task, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsCoordinator() || h.coordinatorSvc == nil || !h.coordinatorSvc.Phase2() {
		return tasks, nil
	}
	refs := make([]coordinator.TaskRef, 0, len(tasks))
	for _, t := range tasks {
		refs = append(refs, coordinator.TaskRef{ID: t.ID, WorkflowID: t.WorkflowID})
	}
	watched, err := h.coordinatorSvc.WatchedTaskIDs(ctx, principal.CoordinatorID, refs)
	if err != nil {
		h.logger.Error("coordinator projects read failed", zap.String("coordinator_id", principal.CoordinatorID), zap.Error(err))
		return nil, errors.New("could not read the coordinator's projects")
	}
	kept := make([]*taskmodels.Task, 0, len(tasks))
	for _, t := range tasks {
		if watched[t.ID] {
			kept = append(kept, t)
		}
	}
	return kept, nil
}
