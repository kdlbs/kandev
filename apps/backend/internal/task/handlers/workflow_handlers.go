package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type WorkflowHandlers struct {
	controller *controller.WorkflowController
	logger     *logger.Logger
}

func NewWorkflowHandlers(svc *controller.WorkflowController, log *logger.Logger) *WorkflowHandlers {
	return &WorkflowHandlers{
		controller: svc,
		logger:     log.WithFields(zap.String("component", "task-workflow-handlers")),
	}
}

func RegisterWorkflowRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.WorkflowController, log *logger.Logger) {
	handlers := NewWorkflowHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *WorkflowHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workflows", h.httpListWorkflows)
	api.GET("/workspaces/:id/workflows", h.httpListWorkflowsByWorkspace)
	api.GET("/workspaces/:id/snapshot", h.httpGetWorkspaceSnapshot)
	api.GET("/workflows/:id", h.httpGetWorkflow)
	api.GET("/workflows/:id/snapshot", h.httpGetWorkflowSnapshot)
	api.POST("/workflows", h.httpCreateWorkflow)
	api.PATCH("/workflows/:id", h.httpUpdateWorkflow)
	api.DELETE("/workflows/:id", h.httpDeleteWorkflow)
}

func (h *WorkflowHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionWorkflowList, h.wsListWorkflows)
	dispatcher.RegisterFunc(ws.ActionWorkflowCreate, h.wsCreateWorkflow)
	dispatcher.RegisterFunc(ws.ActionWorkflowGet, h.wsGetWorkflow)
	dispatcher.RegisterFunc(ws.ActionWorkflowUpdate, h.wsUpdateWorkflow)
	dispatcher.RegisterFunc(ws.ActionWorkflowDelete, h.wsDeleteWorkflow)
}

// HTTP handlers

func (h *WorkflowHandlers) httpListWorkflows(c *gin.Context) {
	resp, err := h.controller.ListWorkflows(c.Request.Context(), dto.ListWorkflowsRequest{
		WorkspaceID: c.Query("workspace_id"),
	})
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workflows"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpListWorkflowsByWorkspace(c *gin.Context) {
	resp, err := h.controller.ListWorkflows(c.Request.Context(), dto.ListWorkflowsRequest{
		WorkspaceID: c.Param("id"),
	})
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workflows"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpGetWorkflow(c *gin.Context) {
	resp, err := h.controller.GetWorkflow(c.Request.Context(), dto.GetWorkflowRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateWorkflowRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

func (h *WorkflowHandlers) httpCreateWorkflow(c *gin.Context) {
	var body httpCreateWorkflowRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.WorkspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and name are required"})
		return
	}
	resp, err := h.controller.CreateWorkflow(c.Request.Context(), dto.CreateWorkflowRequest{
		WorkspaceID:        body.WorkspaceID,
		Name:               body.Name,
		Description:        body.Description,
		WorkflowTemplateID: body.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create workflow", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create workflow"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

type httpUpdateWorkflowRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h *WorkflowHandlers) httpUpdateWorkflow(c *gin.Context) {
	var body httpUpdateWorkflowRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := h.controller.UpdateWorkflow(c.Request.Context(), dto.UpdateWorkflowRequest{
		ID:          c.Param("id"),
		Name:        body.Name,
		Description: body.Description,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpDeleteWorkflow(c *gin.Context) {
	resp, err := h.controller.DeleteWorkflow(c.Request.Context(), dto.DeleteWorkflowRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpGetWorkflowSnapshot(c *gin.Context) {
	resp, err := h.controller.GetSnapshot(c.Request.Context(), dto.GetWorkflowSnapshotRequest{WorkflowID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpGetWorkspaceSnapshot(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace id is required"})
		return
	}
	workflowID := c.Query("workflow_id")
	resp, err := h.controller.GetWorkspaceSnapshot(c.Request.Context(), dto.GetWorkspaceSnapshotRequest{
		WorkspaceID: workspaceID,
		WorkflowID:  workflowID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

func (h *WorkflowHandlers) wsListWorkflows(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id,omitempty"`
	}
	if msg.Payload != nil {
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		}
	}
	resp, err := h.controller.ListWorkflows(ctx, dto.ListWorkflowsRequest{WorkspaceID: req.WorkspaceID})
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workflows", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateWorkflowRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

func (h *WorkflowHandlers) wsCreateWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateWorkflowRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	resp, err := h.controller.CreateWorkflow(ctx, dto.CreateWorkflowRequest{
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetWorkflowRequest struct {
	ID string `json:"id"`
}

func (h *WorkflowHandlers) wsGetWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetWorkflowRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.GetWorkflow(ctx, dto.GetWorkflowRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Workflow not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateWorkflowRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (h *WorkflowHandlers) wsUpdateWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateWorkflowRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.UpdateWorkflow(ctx, dto.UpdateWorkflowRequest{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to update workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

func (h *WorkflowHandlers) wsDeleteWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsHandleIDRequest(ctx, msg, h.logger, "failed to delete workflow",
		func(ctx context.Context, id string) (any, error) {
			return h.controller.DeleteWorkflow(ctx, dto.DeleteWorkflowRequest{ID: id})
		})
}
