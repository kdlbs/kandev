// Package handlers provides WebSocket handlers for MCP tool requests.
// These handlers are called by agentctl via the WS tunnel and execute
// operations against the backend services directly.
package handlers

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/repository"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Action constants for MCP WebSocket messages
const (
	ActionMCPListWorkspaces    = "mcp.list_workspaces"
	ActionMCPListBoards        = "mcp.list_boards"
	ActionMCPListWorkflowSteps = "mcp.list_workflow_steps"
	ActionMCPListTasks         = "mcp.list_tasks"
	ActionMCPCreateTask        = "mcp.create_task"
	ActionMCPUpdateTask        = "mcp.update_task"
	ActionMCPAskUserQuestion   = "mcp.ask_user_question"
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

// Handlers provides MCP WebSocket handlers.
type Handlers struct {
	workspaceCtrl    *controller.WorkspaceController
	boardCtrl        *controller.BoardController
	taskCtrl         *controller.TaskController
	workflowCtrl     *workflowctrl.Controller
	clarificationSvc ClarificationService
	messageCreator   MessageCreator
	repo             repository.Repository
	logger           *logger.Logger
}

// NewHandlers creates new MCP handlers.
func NewHandlers(
	workspaceCtrl *controller.WorkspaceController,
	boardCtrl *controller.BoardController,
	taskCtrl *controller.TaskController,
	workflowCtrl *workflowctrl.Controller,
	clarificationSvc ClarificationService,
	messageCreator MessageCreator,
	repo repository.Repository,
	log *logger.Logger,
) *Handlers {
	return &Handlers{
		workspaceCtrl:    workspaceCtrl,
		boardCtrl:        boardCtrl,
		taskCtrl:         taskCtrl,
		workflowCtrl:     workflowCtrl,
		clarificationSvc: clarificationSvc,
		messageCreator:   messageCreator,
		repo:             repo,
		logger:           log.WithFields(zap.String("component", "mcp-handlers")),
	}
}

// RegisterHandlers registers all MCP handlers with the dispatcher.
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ActionMCPListWorkspaces, h.handleListWorkspaces)
	d.RegisterFunc(ActionMCPListBoards, h.handleListBoards)
	d.RegisterFunc(ActionMCPListWorkflowSteps, h.handleListWorkflowSteps)
	d.RegisterFunc(ActionMCPListTasks, h.handleListTasks)
	d.RegisterFunc(ActionMCPCreateTask, h.handleCreateTask)
	d.RegisterFunc(ActionMCPUpdateTask, h.handleUpdateTask)
	d.RegisterFunc(ActionMCPAskUserQuestion, h.handleAskUserQuestion)

	h.logger.Info("registered MCP handlers", zap.Int("count", 7))
}

// handleListWorkspaces lists all workspaces.
func (h *Handlers) handleListWorkspaces(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.workspaceCtrl.ListWorkspaces(ctx, dto.ListWorkspacesRequest{})
	if err != nil {
		h.logger.Error("failed to list workspaces", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workspaces", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// handleListBoards lists boards for a workspace.
func (h *Handlers) handleListBoards(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	resp, err := h.boardCtrl.ListBoards(ctx, dto.ListBoardsRequest{WorkspaceID: req.WorkspaceID})
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list boards", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// handleListWorkflowSteps lists workflow steps for a board.
func (h *Handlers) handleListWorkflowSteps(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		BoardID string `json:"board_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}

	resp, err := h.workflowCtrl.ListStepsByBoard(ctx, workflowctrl.ListStepsRequest{BoardID: req.BoardID})
	if err != nil {
		h.logger.Error("failed to list workflow steps", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workflow steps", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// handleListTasks lists tasks for a board.
func (h *Handlers) handleListTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		BoardID string `json:"board_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}

	resp, err := h.taskCtrl.ListTasks(ctx, dto.ListTasksRequest{BoardID: req.BoardID})
	if err != nil {
		h.logger.Error("failed to list tasks", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list tasks", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
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
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}
	if req.Title == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "title is required", nil)
	}

	resp, err := h.taskCtrl.CreateTask(ctx, req)
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
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

	resp, err := h.taskCtrl.UpdateTask(ctx, req)
	if err != nil {
		h.logger.Error("failed to update task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// handleAskUserQuestion creates a clarification request and waits for response.
func (h *Handlers) handleAskUserQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string                  `json:"session_id"`
		TaskID    string                  `json:"task_id"`
		Question  clarification.Question  `json:"question"`
		Context   string                  `json:"context"`
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

	// Look up task ID from session if not provided
	taskID := req.TaskID
	if taskID == "" && h.repo != nil {
		session, err := h.repo.GetTaskSession(ctx, req.SessionID)
		if err != nil {
			h.logger.Warn("failed to look up session for task ID",
				zap.String("session_id", req.SessionID),
				zap.Error(err))
		} else {
			taskID = session.TaskID
		}
	}

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

	// Wait for user response
	resp, err := h.clarificationSvc.WaitForResponse(ctx, pendingID)
	if err != nil {
		h.logger.Error("failed waiting for clarification response",
			zap.String("pending_id", pendingID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get user response: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// generateOptionID generates an option ID for a question.
func generateOptionID(questionIndex, optionIndex int) string {
	return "q" + string(rune('1'+questionIndex)) + "_opt" + string(rune('1'+optionIndex))
}

