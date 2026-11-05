package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// CrossWorkspaceGateway uses the current collaboration boundary and signed
// run identity. Control actions and completion reporting are separate surfaces.
type CrossWorkspaceGateway interface {
	Directory(context.Context, RunContext) (json.RawMessage, error)
	Coordinator(context.Context, RunContext, string) (json.RawMessage, error)
	ListTasks(context.Context, RunContext, string, string) (json.RawMessage, error)
	TaskSessions(context.Context, RunContext, string) (json.RawMessage, error)
	TaskConversation(context.Context, RunContext, string, ConversationQuery) (json.RawMessage, error)
	TaskPlan(context.Context, RunContext, string) (json.RawMessage, error)
	SendMessage(context.Context, RunContext, CrossWorkspaceMessage) (json.RawMessage, error)
}

type ConversationQuery struct {
	SessionID string `json:"session_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Sort      string `json:"sort,omitempty"`
}

type CrossWorkspaceMessage struct {
	TaskID      string `json:"task_id"`
	Prompt      string `json:"prompt"`
	OperationID string `json:"operation_id,omitempty"`
}

type CrossWorkspaceError struct {
	Status  int
	Message string
}

func (e *CrossWorkspaceError) Error() string { return e.Message }

// SetCrossWorkspace is wired before the HTTP listener becomes ready. An absent
// gateway fails closed without changing existing own-workspace runtime routes.
func (h *Handler) SetCrossWorkspace(gateway CrossWorkspaceGateway) {
	h.crossWorkspace = gateway
}

func registerCrossWorkspaceRoutes(group *gin.RouterGroup, h *Handler) {
	group.GET("/runtime/workspaces", h.listReachableWorkspaces)
	group.GET("/runtime/workspaces/:id/coordinator", h.getWorkspaceCoordinator)
	group.GET("/runtime/workspaces/:id/tasks", h.listWorkspaceTasks)
	group.GET("/runtime/tasks/:id/sessions", h.getTaskSessions)
	group.GET("/runtime/tasks/:id/conversation", h.getTaskConversation)
	group.GET("/runtime/tasks/:id/plan", h.getTaskPlan)
	group.POST("/runtime/tasks/:id/messages", h.sendTaskMessage)
}

func (h *Handler) crossWorkspaceRead(c *gin.Context, action, targetType, targetID string) (RunContext, bool) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return RunContext{}, false
	}
	if !runCtx.Capabilities.Allows(CapabilityListTasks) {
		h.respondRuntimeError(c, runCtx, action, targetType, targetID, ErrCapabilityDenied)
		return RunContext{}, false
	}
	if h.crossWorkspace == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace collaboration is not available"})
		return RunContext{}, false
	}
	return runCtx, true
}

func (h *Handler) listReachableWorkspaces(c *gin.Context) {
	runCtx, ok := h.crossWorkspaceRead(c, "list_workspaces", "workspace", "")
	if !ok {
		return
	}
	body, err := h.crossWorkspace.Directory(c.Request.Context(), runCtx)
	h.respondCrossWorkspaceRead(c, runCtx, "list_workspaces", "workspace", runCtx.WorkspaceID, body, err)
}

func (h *Handler) getWorkspaceCoordinator(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	runCtx, ok := h.crossWorkspaceRead(c, "get_workspace_coordinator", "workspace", id)
	if !ok {
		return
	}
	body, err := h.crossWorkspace.Coordinator(c.Request.Context(), runCtx, id)
	h.respondCrossWorkspaceRead(c, runCtx, "get_workspace_coordinator", "workspace", id, body, err)
}

func (h *Handler) listWorkspaceTasks(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	runCtx, ok := h.crossWorkspaceRead(c, "list_workspace_tasks", "workspace", id)
	if !ok {
		return
	}
	body, err := h.crossWorkspace.ListTasks(c.Request.Context(), runCtx, id, strings.TrimSpace(c.Query("workflow_id")))
	h.respondCrossWorkspaceRead(c, runCtx, "list_workspace_tasks", "workspace", id, body, err)
}

func (h *Handler) getTaskSessions(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	runCtx, ok := h.crossWorkspaceRead(c, "get_task_sessions", "task", id)
	if !ok {
		return
	}
	body, err := h.crossWorkspace.TaskSessions(c.Request.Context(), runCtx, id)
	h.respondCrossWorkspaceRead(c, runCtx, "get_task_sessions", "task", id, body, err)
}

func (h *Handler) getTaskConversation(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	runCtx, ok := h.crossWorkspaceRead(c, "get_task_conversation", "task", id)
	if !ok {
		return
	}
	query, err := parseConversationQuery(c.Request.URL.Query())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body, err := h.crossWorkspace.TaskConversation(c.Request.Context(), runCtx, id, query)
	h.respondCrossWorkspaceRead(c, runCtx, "get_task_conversation", "task", id, body, err)
}

func (h *Handler) getTaskPlan(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	runCtx, ok := h.crossWorkspaceRead(c, "get_task_plan", "task", id)
	if !ok {
		return
	}
	body, err := h.crossWorkspace.TaskPlan(c.Request.Context(), runCtx, id)
	h.respondCrossWorkspaceRead(c, runCtx, "get_task_plan", "task", id, body, err)
}

func (h *Handler) sendTaskMessage(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	if !runCtx.Capabilities.Allows(CapabilityPostComment) {
		h.respondRuntimeError(c, runCtx, "send_task_message", "task", id, ErrCapabilityDenied)
		return
	}
	var req struct {
		Prompt      string `json:"prompt"`
		OperationID string `json:"operation_id"`
	}
	if !bindClosedJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
		return
	}
	if h.crossWorkspace == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace collaboration is not available"})
		return
	}
	body, err := h.crossWorkspace.SendMessage(c.Request.Context(), runCtx, CrossWorkspaceMessage{
		TaskID: id, Prompt: req.Prompt, OperationID: strings.TrimSpace(req.OperationID),
	})
	h.respondCrossWorkspaceRead(c, runCtx, "send_task_message", "task", id, body, err)
}

func (h *Handler) respondCrossWorkspaceRead(
	c *gin.Context, runCtx RunContext, action, targetType, targetID string, body json.RawMessage, err error,
) {
	if err != nil {
		var target *CrossWorkspaceError
		if errors.As(err, &target) {
			h.appendDeniedRunEvent(c.Request.Context(), runCtx, action, targetType, targetID, err)
			c.JSON(target.Status, gin.H{"error": target.Message})
			return
		}
		h.respondRuntimeError(c, runCtx, action, targetType, targetID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, action, targetType, targetID)
	if len(body) == 0 {
		body = json.RawMessage(`{}`)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func parseConversationQuery(values url.Values) (ConversationQuery, error) {
	query := ConversationQuery{
		SessionID: strings.TrimSpace(values.Get("session_id")), Before: strings.TrimSpace(values.Get("before")),
		After: strings.TrimSpace(values.Get("after")), Sort: strings.TrimSpace(values.Get("sort")),
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 {
			return ConversationQuery{}, errors.New("limit must be a non-negative integer")
		}
		query.Limit = limit
	}
	if query.Sort != "" && query.Sort != "asc" && query.Sort != "desc" {
		return ConversationQuery{}, errors.New("sort must be asc or desc")
	}
	return query, nil
}
