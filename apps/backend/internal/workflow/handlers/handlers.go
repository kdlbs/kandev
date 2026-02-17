package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/controller"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Handlers manages workflow HTTP and WebSocket handlers
type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

// NewHandlers creates new workflow handlers
func NewHandlers(ctrl *controller.Controller, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "workflow-handlers")),
	}
}

// RegisterRoutes registers workflow HTTP and WebSocket handlers
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.Controller, log *logger.Logger) {
	handlers := NewHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *Handlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")

	// Template routes
	api.GET("/workflow/templates", h.httpListTemplates)
	api.GET("/workflow/templates/:id", h.httpGetTemplate)

	// Step routes
	api.GET("/workflows/:id/workflow/steps", h.httpListStepsByWorkflow)
	api.GET("/workflow/steps/:id", h.httpGetStep)
	api.POST("/workflows/:id/workflow/steps", h.httpCreateStepsFromTemplate)
	api.POST("/workflow/steps", h.httpCreateStep)
	api.PUT("/workflow/steps/:id", h.httpUpdateStep)
	api.DELETE("/workflow/steps/:id", h.httpDeleteStep)
	api.PUT("/workflows/:id/workflow/steps/reorder", h.httpReorderSteps)

	// Export/Import routes
	api.GET("/workflows/:id/export", h.httpExportWorkflow)
	api.GET("/workspaces/:id/workflows/export", h.httpExportWorkflows)
	api.POST("/workspaces/:id/workflows/import", h.httpImportWorkflows)

	// History routes
	api.GET("/sessions/:id/workflow/history", h.httpListHistoryBySession)
}

func (h *Handlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionWorkflowTemplateList, h.wsListTemplates)
	dispatcher.RegisterFunc(ws.ActionWorkflowTemplateGet, h.wsGetTemplate)
	dispatcher.RegisterFunc(ws.ActionWorkflowStepList, h.wsListSteps)
	dispatcher.RegisterFunc(ws.ActionWorkflowStepGet, h.wsGetStep)
	dispatcher.RegisterFunc(ws.ActionWorkflowStepCreate, h.wsCreateStepsFromTemplate)
	dispatcher.RegisterFunc(ws.ActionWorkflowHistoryList, h.wsListHistory)
}

// HTTP handlers - Templates

func (h *Handlers) httpListTemplates(c *gin.Context) {
	resp, err := h.controller.ListTemplates(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list templates", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list templates"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpGetTemplate(c *gin.Context) {
	resp, err := h.controller.GetTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to get template", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// HTTP handlers - Steps

func (h *Handlers) httpListStepsByWorkflow(c *gin.Context) {
	resp, err := h.controller.ListStepsByWorkflow(c.Request.Context(), controller.ListStepsRequest{
		WorkflowID: c.Param("id"),
	})
	if err != nil {
		h.logger.Error("failed to list steps", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list steps"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpGetStep(c *gin.Context) {
	resp, err := h.controller.GetStep(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to get step", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Step not found"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateStepsRequest struct {
	TemplateID string `json:"template_id"`
}

func (h *Handlers) httpCreateStepsFromTemplate(c *gin.Context) {
	var req httpCreateStepsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	if err := h.controller.CreateStepsFromTemplate(c.Request.Context(), controller.CreateStepsFromTemplateRequest{
		WorkflowID: c.Param("id"),
		TemplateID: req.TemplateID,
	}); err != nil {
		h.logger.Error("failed to create steps from template", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create steps"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

func (h *Handlers) httpCreateStep(c *gin.Context) {
	var req controller.CreateStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	if req.WorkflowID == "" || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow_id and name are required"})
		return
	}
	resp, err := h.controller.CreateStep(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("failed to create step", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create step"})
		return
	}
	c.JSON(http.StatusCreated, resp.Step)
}

func (h *Handlers) httpUpdateStep(c *gin.Context) {
	var req controller.UpdateStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	req.ID = c.Param("id")
	resp, err := h.controller.UpdateStep(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("failed to update step", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update step"})
		return
	}
	c.JSON(http.StatusOK, resp.Step)
}

func (h *Handlers) httpDeleteStep(c *gin.Context) {
	if err := h.controller.DeleteStep(c.Request.Context(), c.Param("id")); err != nil {
		h.logger.Error("failed to delete step", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete step"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type httpReorderStepsRequest struct {
	StepIDs []string `json:"step_ids"`
}

func (h *Handlers) httpReorderSteps(c *gin.Context) {
	var req httpReorderStepsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	if err := h.controller.ReorderSteps(c.Request.Context(), controller.ReorderStepsRequest{
		WorkflowID: c.Param("id"),
		StepIDs: req.StepIDs,
	}); err != nil {
		h.logger.Error("failed to reorder steps", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reorder steps"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// HTTP handlers - History

func (h *Handlers) httpListHistoryBySession(c *gin.Context) {
	resp, err := h.controller.ListHistoryBySession(c.Request.Context(), controller.ListHistoryRequest{
		SessionID: c.Param("id"),
	})
	if err != nil {
		h.logger.Error("failed to list history", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list history"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// HTTP handlers - Export/Import

func (h *Handlers) httpExportWorkflow(c *gin.Context) {
	resp, err := h.controller.ExportWorkflow(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to export workflow", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export workflow"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpExportWorkflows(c *gin.Context) {
	resp, err := h.controller.ExportWorkflows(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to export workflows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export workflows"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpImportWorkflows(c *gin.Context) {
	var req controller.ImportWorkflowsRequest
	if err := c.ShouldBindJSON(&req.Data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	req.WorkspaceID = c.Param("id")
	resp, err := h.controller.ImportWorkflows(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("failed to import workflows", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers - Templates

func (h *Handlers) wsListTemplates(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListTemplates(ctx)
	if err != nil {
		h.logger.Error("failed to list templates", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list templates", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetTemplateRequest struct {
	ID string `json:"id"`
}

func (h *Handlers) wsGetTemplate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetTemplateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.GetTemplate(ctx, req.ID)
	if err != nil {
		h.logger.Error("failed to get template", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Template not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// WS handlers - Steps

type wsListStepsRequest struct {
	WorkflowID string `json:"workflow_id"`
}

func (h *Handlers) wsListSteps(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListStepsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	resp, err := h.controller.ListStepsByWorkflow(ctx, controller.ListStepsRequest{WorkflowID: req.WorkflowID})
	if err != nil {
		h.logger.Error("failed to list steps", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list steps", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetStepRequest struct {
	ID string `json:"id"`
}

func (h *Handlers) wsGetStep(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetStepRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.GetStep(ctx, req.ID)
	if err != nil {
		h.logger.Error("failed to get step", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Step not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateStepsRequest struct {
	WorkflowID string `json:"workflow_id"`
	TemplateID string `json:"template_id"`
}

func (h *Handlers) wsCreateStepsFromTemplate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateStepsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkflowID == "" || req.TemplateID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id and template_id are required", nil)
	}
	if err := h.controller.CreateStepsFromTemplate(ctx, controller.CreateStepsFromTemplateRequest{
		WorkflowID: req.WorkflowID,
		TemplateID: req.TemplateID,
	}); err != nil {
		h.logger.Error("failed to create steps", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create steps", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

// WS handlers - History

type wsListHistoryRequest struct {
	SessionID string `json:"session_id"`
}

func (h *Handlers) wsListHistory(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListHistoryRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	resp, err := h.controller.ListHistoryBySession(ctx, controller.ListHistoryRequest{SessionID: req.SessionID})
	if err != nil {
		h.logger.Error("failed to list history", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list history", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

