package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// WorkflowStepLister provides access to workflow steps.
type WorkflowStepLister interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*workflowmodels.WorkflowStep, error)
}

type WorkflowHandlers struct {
	service            *service.Service
	workflowStepLister WorkflowStepLister
	logger             *logger.Logger
}

func NewWorkflowHandlers(svc *service.Service, stepLister WorkflowStepLister, log *logger.Logger) *WorkflowHandlers {
	return &WorkflowHandlers{
		service:            svc,
		workflowStepLister: stepLister,
		logger:             log.WithFields(zap.String("component", "task-workflow-handlers")),
	}
}

func RegisterWorkflowRoutes(
	router *gin.Engine,
	dispatcher *ws.Dispatcher,
	svc *service.Service,
	stepLister WorkflowStepLister,
	log *logger.Logger,
) {
	handlers := NewWorkflowHandlers(svc, stepLister, log)
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

func (h *WorkflowHandlers) listWorkflows(ctx context.Context, workspaceID string) (dto.ListWorkflowsResponse, error) {
	workflows, err := h.service.ListWorkflows(ctx, workspaceID)
	if err != nil {
		return dto.ListWorkflowsResponse{}, err
	}
	result := dto.ListWorkflowsResponse{
		Workflows: make([]dto.WorkflowDTO, 0, len(workflows)),
		Total:     len(workflows),
	}
	for _, w := range workflows {
		result.Workflows = append(result.Workflows, dto.FromWorkflow(w))
	}
	return result, nil
}

// HTTP handlers

func (h *WorkflowHandlers) httpListWorkflows(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	resp, err := h.listWorkflows(c.Request.Context(), workspaceID)
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workflows"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpListWorkflowsByWorkspace(c *gin.Context) {
	workspaceID := c.Param("id")
	resp, err := h.listWorkflows(c.Request.Context(), workspaceID)
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workflows"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandlers) httpGetWorkflow(c *gin.Context) {
	workflow, err := h.service.GetWorkflow(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromWorkflow(workflow))
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
	workflow, err := h.service.CreateWorkflow(c.Request.Context(), &service.CreateWorkflowRequest{
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
	c.JSON(http.StatusCreated, dto.FromWorkflow(workflow))
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
	id := c.Param("id")
	workflow, err := h.service.UpdateWorkflow(c.Request.Context(), id, &service.UpdateWorkflowRequest{
		Name:        body.Name,
		Description: body.Description,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromWorkflow(workflow))
}

func (h *WorkflowHandlers) httpDeleteWorkflow(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteWorkflow(c.Request.Context(), id); err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

func (h *WorkflowHandlers) httpGetWorkflowSnapshot(c *gin.Context) {
	workflowID := c.Param("id")

	workflow, err := h.service.GetWorkflow(c.Request.Context(), workflowID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	steps, err := h.getStepsForWorkflow(c.Request.Context(), workflow)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	tasks, err := h.service.ListTasks(c.Request.Context(), workflowID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	tasks = applyTaskLimit(c, tasks)

	taskDTOs, err := h.convertTasksWithPrimarySessions(c.Request.Context(), tasks)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	c.JSON(http.StatusOK, dto.WorkflowSnapshotDTO{
		Workflow: dto.FromWorkflow(workflow),
		Steps:    steps,
		Tasks:    taskDTOs,
	})
}

func (h *WorkflowHandlers) httpGetWorkspaceSnapshot(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace id is required"})
		return
	}

	workflowID := c.Query("workflow_id")
	if workflowID == "" {
		workflows, err := h.service.ListWorkflows(c.Request.Context(), workspaceID)
		if err != nil {
			handleNotFound(c, h.logger, err, "workflow not found")
			return
		}
		if len(workflows) == 0 {
			handleNotFound(c, h.logger,
				fmt.Errorf("no workflows found for workspace: %s", workspaceID),
				"workflow not found")
			return
		}
		workflowID = workflows[0].ID
	}

	workflow, err := h.service.GetWorkflow(c.Request.Context(), workflowID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}
	if workflow.WorkspaceID != workspaceID {
		handleNotFound(c, h.logger,
			fmt.Errorf("workflow does not belong to workspace: %s", workspaceID),
			"workflow not found")
		return
	}

	steps, err := h.getStepsForWorkflow(c.Request.Context(), workflow)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	tasks, err := h.service.ListTasks(c.Request.Context(), workflowID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	tasks = applyTaskLimit(c, tasks)

	taskDTOs, err := h.convertTasksWithPrimarySessions(c.Request.Context(), tasks)
	if err != nil {
		handleNotFound(c, h.logger, err, "workflow not found")
		return
	}

	c.JSON(http.StatusOK, dto.WorkflowSnapshotDTO{
		Workflow: dto.FromWorkflow(workflow),
		Steps:    steps,
		Tasks:    taskDTOs,
	})
}

// applyTaskLimit truncates tasks if a task_limit query param is set.
func applyTaskLimit(c *gin.Context, tasks []*models.Task) []*models.Task {
	if limitStr := c.Query("task_limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit < len(tasks) {
			return tasks[:limit]
		}
	}
	return tasks
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
	workflows, err := h.service.ListWorkflows(ctx, req.WorkspaceID)
	if err != nil {
		h.logger.Error("failed to list workflows", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workflows", nil)
	}
	result := dto.ListWorkflowsResponse{
		Workflows: make([]dto.WorkflowDTO, 0, len(workflows)),
		Total:     len(workflows),
	}
	for _, w := range workflows {
		result.Workflows = append(result.Workflows, dto.FromWorkflow(w))
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
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

	workflow, err := h.service.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromWorkflow(workflow))
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

	workflow, err := h.service.GetWorkflow(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Workflow not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromWorkflow(workflow))
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

	workflow, err := h.service.UpdateWorkflow(ctx, req.ID, &service.UpdateWorkflowRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to update workflow", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workflow", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromWorkflow(workflow))
}

func (h *WorkflowHandlers) wsDeleteWorkflow(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsHandleIDRequest(ctx, msg, h.logger, "failed to delete workflow",
		func(ctx context.Context, id string) (any, error) {
			if err := h.service.DeleteWorkflow(ctx, id); err != nil {
				return nil, err
			}
			return dto.SuccessResponse{Success: true}, nil
		})
}

// getStepsForWorkflow returns workflow steps for a workflow as DTOs.
func (h *WorkflowHandlers) getStepsForWorkflow(
	ctx context.Context,
	workflow *models.Workflow,
) ([]dto.WorkflowStepDTO, error) {
	if h.workflowStepLister == nil {
		return nil, fmt.Errorf("workflow step lister not configured")
	}
	steps, err := h.workflowStepLister.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.WorkflowStepDTO, 0, len(steps))
	for _, step := range steps {
		result = append(result, dto.FromWorkflowStepWithTimestamps(step))
	}
	return result, nil
}

// convertTasksWithPrimarySessions converts task models to DTOs with primary session IDs.
func (h *WorkflowHandlers) convertTasksWithPrimarySessions(
	ctx context.Context,
	tasks []*models.Task,
) ([]dto.TaskDTO, error) {
	return buildTaskDTOsWithSessionInfo(ctx, h.service, tasks)
}
