package handlers

import (
	"context"
	"errors"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const mcpCreateTaskParentResolutionReasonKanbanDepthLimit = "kanban_depth_limit"

// mcpCreateTaskParentResolution explains an internal self-parent resolution
// to the MCP caller. It is intentionally part of the MCP result only; REST
// task creation has no equivalent literal-self input.
type mcpCreateTaskParentResolution struct {
	RequestedParentID string `json:"requested_parent_id"`
	ResolvedParentID  string `json:"resolved_parent_id"`
	Reason            string `json:"reason"`
	Message           string `json:"message"`
}

func (r *mcpCreateTaskParentResolution) forOutcome(created bool) *mcpCreateTaskParentResolution {
	if r == nil {
		return nil
	}
	result := *r
	if created {
		result.Message = "Created a sibling task under " + r.ResolvedParentID + " because the Kanban subtask depth limit was reached. " +
			r.ResolvedParentID + " owns coordination; your session remains the creation source."
	} else {
		result.Message = "Resolved self placement to " + r.ResolvedParentID + " because the Kanban subtask depth limit was reached. An existing task was returned without creation or reparenting. The returned task's parent_id is its actual parent."
	}
	return &result
}

// resolveMCPCreateTaskSelfPlacement applies the one-hop Kanban self-parent
// rule after the caller principal has been authenticated. The marker is an
// internal transport hint, so every authority check remains server-owned.
func (h *Handlers) resolveMCPCreateTaskSelfPlacement(
	ctx context.Context,
	principal mcpscope.Principal,
	req *mcpCreateTaskRequest,
) (*mcpCreateTaskParentResolution, error) {
	if !req.ParentIsSelf {
		return nil, nil
	}
	if principal.Surface != mcpprofile.SurfaceKanbanTask || principal.IsAutomation() {
		return nil, denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"parent_id=self placement is available only to session-bound Kanban task callers",
		)
	}
	if req.ParentID != principal.CallerTaskID {
		return nil, denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"parent_is_self must resolve from the current session task",
		)
	}

	caller, err := h.taskSvc.GetTask(ctx, principal.CallerTaskID)
	if err != nil || caller == nil {
		return nil, denyMCPCreateTask(ws.ErrorCodeUnauthorized, "the session caller identity could not be verified")
	}
	if caller.IsEphemeral {
		return nil, denyMCPCreateTask(ws.ErrorCodeValidation, "cannot create subtasks of an ephemeral task (quick chat); omit parent_id to create a top-level task")
	}
	if caller.ParentID == "" {
		return nil, nil
	}

	parent, err := h.mcpSelfPlacementParent(ctx, caller, principal.WorkspaceID)
	if err != nil {
		return nil, err
	}
	req.ParentID = parent.ID
	return &mcpCreateTaskParentResolution{
		RequestedParentID: caller.ID,
		ResolvedParentID:  parent.ID,
		Reason:            mcpCreateTaskParentResolutionReasonKanbanDepthLimit,
	}, nil
}

func (h *Handlers) mcpSelfPlacementParent(ctx context.Context, caller *models.Task, workspaceID string) (*models.Task, error) {
	parent, err := h.taskSvc.GetTask(ctx, caller.ParentID)
	if err != nil && !errors.Is(err, repoerrors.ErrTaskNotFound) &&
		!errors.Is(err, repoerrors.ErrWorkspaceNotFound) && !service.IsForbidden(err) {
		h.logger.Error("failed to resolve self placement parent", zap.Error(err))
		return nil, denyMCPCreateTask(ws.ErrorCodeInternalError, "failed to resolve self placement parent")
	}
	if err != nil || parent == nil || parent.WorkspaceID != workspaceID || parent.WorkspaceID != caller.WorkspaceID {
		// Keep inaccessible parents indistinguishable from missing parents. The
		// caller must not learn another workspace's task identity.
		return nil, denyMCPCreateTask(ws.ErrorCodeNotFound, "target parent task not found")
	}
	if parent.IsEphemeral {
		return nil, denyMCPCreateTask(
			ws.ErrorCodeValidation,
			"self placement requires a non-ephemeral direct Kanban parent",
		)
	}
	if parent.IsFromOffice {
		return nil, denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"session MCP task creation is restricted to Kanban tasks",
		)
	}
	if parent.ParentID != "" {
		return nil, denyMCPCreateTask(
			ws.ErrorCodeValidation,
			"self placement resolves only to the direct Kanban parent",
		)
	}

	return parent, nil
}
