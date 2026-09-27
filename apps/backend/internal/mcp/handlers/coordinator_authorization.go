package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// coordinatorSurfaceActions is the execution-time mirror of the fixed
// six-tool coordinator catalog (docs/specs/coordinator/system-design/
// copilot.md#tool-surface, AC-COORDINATOR-COPILOT-003.1). Discovery alone is
// not an authorization boundary because an agent can still send a raw
// WebSocket action.
var coordinatorSurfaceActions = map[string]struct{}{
	ws.ActionMCPListTasks:           {},
	ws.ActionMCPGetTaskConversation: {},
	ws.ActionMCPListWorkflows:       {},
	ws.ActionMCPListWorkflowSteps:   {},
	ws.ActionMCPListRepositories:    {},
	coordinator.ActionProposeTask:   {},
}

// authorizeCoordinatorRequest is the one execution-time boundary for the
// fixed coordinator surface: a coordinator principal may call only the six
// allowlisted actions, each scoped to its own workspace, and
// coordinator.propose_task may only be called by a coordinator principal.
//
// Cross-workspace checks on coordinator.propose_task's own workflow_id,
// step_id, repository_id and source_task_id fields are deliberately left to
// coordinator.Service.ProposeTask, which returns a *FieldError naming the
// offending field (AC-COORDINATOR-PROPOSALS-001.3) rather than this guard's
// blanket not-found.
func (h *Handlers) authorizeCoordinatorRequest(ctx context.Context, msg *ws.Message) (*ws.Message, *ws.Message, error) {
	principal, hasPrincipal := mcpscope.PrincipalFromContext(ctx)
	isCoordinator := hasPrincipal && principal.IsCoordinator()

	if msg.Action == coordinator.ActionProposeTask && !isCoordinator {
		return coordinatorUnknownAction(msg)
	}
	if !isCoordinator {
		return nil, msg, nil
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
	return nil, msg, nil
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
	if action == coordinator.ActionProposeTask {
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
