// Package handlers provides WebSocket handlers for MCP tool requests.
// These handlers are called by agentctl via the WS tunnel and execute
// operations against the backend services directly.
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ClarificationService defines the interface for clarification operations.
type ClarificationService interface {
	CreateRequest(req *clarification.Request) string
	WaitForResponse(ctx context.Context, pendingID string) (*clarification.Response, error)
}

// MessageCreator creates messages for clarification requests.
type MessageCreator interface {
	CreateClarificationRequestMessage(ctx context.Context, taskID, sessionID, pendingID string, question clarification.Question, clarificationContext string) (string, error)
}

// SessionRepository interface for updating session state.
type SessionRepository interface {
	UpdateTaskSessionState(ctx context.Context, sessionID string, state models.TaskSessionState, errorMessage string) error
}

// TaskRepository interface for updating task state.
type TaskRepository interface {
	UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error
}

// EventBus interface for publishing events.
type EventBus interface {
	Publish(ctx context.Context, topic string, event *bus.Event) error
}

// Handlers provides MCP WebSocket handlers.
type Handlers struct {
	taskSvc          *service.Service
	workflowCtrl     *workflowctrl.Controller
	clarificationSvc ClarificationService
	messageCreator   MessageCreator
	sessionRepo      SessionRepository
	taskRepo         TaskRepository
	eventBus         EventBus
	planService      *service.PlanService
	logger           *logger.Logger
}

// NewHandlers creates new MCP handlers.
func NewHandlers(
	taskSvc *service.Service,
	workflowCtrl *workflowctrl.Controller,
	clarificationSvc ClarificationService,
	messageCreator MessageCreator,
	sessionRepo SessionRepository,
	taskRepo TaskRepository,
	eventBus EventBus,
	planService *service.PlanService,
	log *logger.Logger,
) *Handlers {
	return &Handlers{
		taskSvc:          taskSvc,
		workflowCtrl:     workflowCtrl,
		clarificationSvc: clarificationSvc,
		messageCreator:   messageCreator,
		sessionRepo:      sessionRepo,
		taskRepo:         taskRepo,
		eventBus:         eventBus,
		planService:      planService,
		logger:           log.WithFields(zap.String("component", "mcp-handlers")),
	}
}

// RegisterHandlers registers all MCP handlers with the dispatcher.
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionMCPListWorkspaces, h.handleListWorkspaces)
	d.RegisterFunc(ws.ActionMCPListWorkflows, h.handleListWorkflows)
	d.RegisterFunc(ws.ActionMCPListWorkflowSteps, h.handleListWorkflowSteps)
	d.RegisterFunc(ws.ActionMCPListTasks, h.handleListTasks)
	d.RegisterFunc(ws.ActionMCPCreateTask, h.handleCreateTask)
	d.RegisterFunc(ws.ActionMCPUpdateTask, h.handleUpdateTask)
	d.RegisterFunc(ws.ActionMCPAskUserQuestion, h.handleAskUserQuestion)
	d.RegisterFunc(ws.ActionMCPCreateTaskPlan, h.handleCreateTaskPlan)
	d.RegisterFunc(ws.ActionMCPGetTaskPlan, h.handleGetTaskPlan)
	d.RegisterFunc(ws.ActionMCPUpdateTaskPlan, h.handleUpdateTaskPlan)
	d.RegisterFunc(ws.ActionMCPDeleteTaskPlan, h.handleDeleteTaskPlan)

	h.logger.Info("registered MCP handlers", zap.Int("count", 11))
}

// handleListWorkspaces lists all workspaces.
func (h *Handlers) handleListWorkspaces(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		h.logger.Error("failed to list workspaces", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workspaces", nil)
	}
	dtos := make([]dto.WorkspaceDTO, 0, len(workspaces))
	for _, w := range workspaces {
		dtos = append(dtos, dto.FromWorkspace(w))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.ListWorkspacesResponse{Workspaces: dtos, Total: len(dtos)})
}

// unmarshalStringField unmarshals a JSON payload and returns the value of a single string field.
func unmarshalStringField(payload json.RawMessage, fieldName string) (string, error) {
	var m map[string]string
	if err := json.Unmarshal(payload, &m); err != nil {
		return "", err
	}
	return m[fieldName], nil
}

// handleListByField is a generic handler for listing resources identified by a single string field.
func (h *Handlers) handleListByField(
	ctx context.Context, msg *ws.Message,
	fieldName, logErrMsg, clientErrMsg string,
	fn func(context.Context, string) (any, error),
) (*ws.Message, error) {
	value, err := unmarshalStringField(msg.Payload, fieldName)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if value == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fieldName+" is required", nil)
	}
	resp, err := fn(ctx, value)
	if err != nil {
		h.logger.Error(logErrMsg, zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// handleListWorkflows lists workflows for a workspace.
func (h *Handlers) handleListWorkflows(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workspace_id", "failed to list workflows", "Failed to list workflows",
		func(ctx context.Context, workspaceID string) (any, error) {
			workflows, err := h.taskSvc.ListWorkflows(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.WorkflowDTO, 0, len(workflows))
			for _, w := range workflows {
				dtos = append(dtos, dto.FromWorkflow(w))
			}
			return dto.ListWorkflowsResponse{Workflows: dtos, Total: len(dtos)}, nil
		})
}

// handleListWorkflowSteps lists workflow steps for a workflow.
func (h *Handlers) handleListWorkflowSteps(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workflow_id", "failed to list workflow steps", "Failed to list workflow steps",
		func(ctx context.Context, workflowID string) (any, error) {
			return h.workflowCtrl.ListStepsByWorkflow(ctx, workflowctrl.ListStepsRequest{WorkflowID: workflowID})
		})
}

// handleListTasks lists tasks for a workflow.
func (h *Handlers) handleListTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workflow_id", "failed to list tasks", "Failed to list tasks",
		func(ctx context.Context, workflowID string) (any, error) {
			tasks, err := h.taskSvc.ListTasks(ctx, workflowID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.TaskDTO, 0, len(tasks))
			for _, t := range tasks {
				dtos = append(dtos, dto.FromTask(t))
			}
			return dto.ListTasksResponse{Tasks: dtos, Total: len(dtos)}, nil
		})
}

// handleCreateTask creates a new task.
func (h *Handlers) handleCreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req dto.CreateTaskRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}
	if req.Title == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "title is required", nil)
	}

	var repos []service.TaskRepositoryInput
	for _, r := range req.Repositories {
		repos = append(repos, service.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	task, err := h.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Priority:       req.Priority,
		State:          req.State,
		Repositories:   repos,
		Position:       req.Position,
		Metadata:       req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

// handleUpdateTask updates an existing task.
func (h *Handlers) handleUpdateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req dto.UpdateTaskRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	var repos []service.TaskRepositoryInput
	for _, r := range req.Repositories {
		repos = append(repos, service.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	task, err := h.taskSvc.UpdateTask(ctx, req.ID, &service.UpdateTaskRequest{
		Title:        req.Title,
		Description:  req.Description,
		Priority:     req.Priority,
		State:        req.State,
		Repositories: repos,
		Position:     req.Position,
		Metadata:     req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to update task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

// handleAskUserQuestion creates a clarification request and waits for response.
func (h *Handlers) handleAskUserQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string                 `json:"session_id"`
		TaskID    string                 `json:"task_id"`
		Question  clarification.Question `json:"question"`
		Context   string                 `json:"context"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Question.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "question.prompt is required", nil)
	}

	// Generate question ID if missing
	if req.Question.ID == "" {
		req.Question.ID = "q1"
	}
	// Generate option IDs if missing
	for i := range req.Question.Options {
		if req.Question.Options[i].ID == "" {
			req.Question.Options[i].ID = generateOptionID(0, i)
		}
	}

	// Use provided task ID (session -> task lookup is done by caller)
	taskID := req.TaskID

	// Create the clarification request
	clarificationReq := &clarification.Request{
		SessionID: req.SessionID,
		TaskID:    taskID,
		Question:  req.Question,
		Context:   req.Context,
	}
	pendingID := h.clarificationSvc.CreateRequest(clarificationReq)

	// Create the message in the database (triggers WS event to frontend)
	if h.messageCreator != nil {
		_, err := h.messageCreator.CreateClarificationRequestMessage(
			ctx,
			taskID,
			req.SessionID,
			pendingID,
			req.Question,
			req.Context,
		)
		if err != nil {
			h.logger.Error("failed to create clarification request message",
				zap.String("pending_id", pendingID),
				zap.String("session_id", req.SessionID),
				zap.Error(err))
		}
	}

	// Update session and task states to waiting for input
	h.setSessionWaitingForInput(ctx, taskID, req.SessionID)

	// Wait for user response
	resp, err := h.clarificationSvc.WaitForResponse(ctx, pendingID)
	if err != nil {
		h.logger.Error("failed waiting for clarification response",
			zap.String("pending_id", pendingID),
			zap.Error(err))

		// When clarification times out, keep session in WAITING_FOR_INPUT
		// The expired clarification will be marked as expired when user tries to respond
		// This allows the UI to stay responsive while clarification is pending
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get user response: "+err.Error(), nil)
	}

	// Don't update session state here. Let the agent's normal message flow handle it.
	//
	// Previously, we would set the session to RUNNING immediately after the user responded.
	// However, this causes issues when:
	// 1. The agent's MCP client has already timed out (agent stopped waiting)
	// 2. The user responds anyway (UI still showing the clarification)
	// 3. We set session to RUNNING, but agent won't process the response
	// 4. Session gets stuck in RUNNING state with no agent activity
	//
	// By keeping the session in WAITING_FOR_INPUT, the user can:
	// - Send new messages to continue the conversation
	// - The agent will process those messages normally
	// - Session state will be updated by the agent's actual activity
	//
	// If the agent successfully receives this response and continues, the agent's
	// tool calls or messages will naturally update the session to RUNNING.

	h.logger.Debug("user responded to clarification, returning response to agent without updating session state",
		zap.String("pending_id", pendingID),
		zap.String("session_id", req.SessionID),
		zap.String("task_id", taskID))

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// setSessionWaitingForInput updates the session and task states to waiting for input
func (h *Handlers) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string) {
	// Update session state to WAITING_FOR_INPUT
	if err := h.sessionRepo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		h.logger.Warn("failed to update session state to WAITING_FOR_INPUT",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Update task state to REVIEW
	if err := h.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
		h.logger.Warn("failed to update task state to REVIEW",
			zap.String("task_id", taskID),
			zap.Error(err))
	}

	// Publish session state changed event
	if h.eventBus != nil {
		eventData := map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"new_state":  string(models.TaskSessionStateWaitingForInput),
		}
		_ = h.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"mcp-handlers",
			eventData,
		))
	}
}

// generateOptionID generates an option ID for a question.
func generateOptionID(questionIndex, optionIndex int) string {
	return "q" + string(rune('1'+questionIndex)) + "_opt" + string(rune('1'+optionIndex))
}

// handleCreateTaskPlan creates a new task plan.
func (h *Handlers) handleCreateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "agent"
	}

	plan, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID:    req.TaskID,
		Title:     req.Title,
		Content:   req.Content,
		CreatedBy: createdBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// handleGetTaskPlan retrieves a task plan.
func (h *Handlers) handleGetTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	plan, err := h.planService.GetPlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task plan", nil)
	}
	if plan == nil {
		// Return empty object if no plan exists
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{})
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// handleUpdateTaskPlan updates an existing task plan.
func (h *Handlers) handleUpdateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "agent"
	}

	plan, err := h.planService.UpdatePlan(ctx, service.UpdatePlanRequest{
		TaskID:    req.TaskID,
		Title:     req.Title,
		Content:   req.Content,
		CreatedBy: createdBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// handleDeleteTaskPlan deletes a task plan.
func (h *Handlers) handleDeleteTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	err := h.planService.DeletePlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}
