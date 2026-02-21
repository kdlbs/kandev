package handlers

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type wsListTaskSessionsRequest struct {
	TaskID string `json:"task_id"`
}

func (h *TaskHandlers) wsListTaskSessions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListTaskSessionsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	return h.doListTaskSessions(ctx, msg, req.TaskID)
}

func (h *TaskHandlers) doListTaskSessions(ctx context.Context, msg *ws.Message, taskID string) (*ws.Message, error) {
	if taskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	sessions, err := h.service.ListTaskSessions(ctx, taskID)
	if err != nil {
		h.logger.Error("failed to list task sessions", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task sessions", nil)
	}
	sessionDTOs := make([]dto.TaskSessionDTO, 0, len(sessions))
	for _, session := range sessions {
		sessionDTOs = append(sessionDTOs, dto.FromTaskSession(session))
	}
	resp := dto.ListTaskSessionsResponse{
		Sessions: sessionDTOs,
		Total:    len(sessionDTOs),
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsListTasksRequest struct {
	WorkflowID string `json:"workflow_id"`
}

func (h *TaskHandlers) wsListTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListTasksRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}

	tasks, err := h.service.ListTasks(ctx, req.WorkflowID)
	if err != nil {
		h.logger.Error("failed to list tasks", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list tasks", nil)
	}
	taskDTOs := make([]dto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		taskDTOs = append(taskDTOs, dto.FromTask(task))
	}
	resp := dto.ListTasksResponse{
		Tasks: taskDTOs,
		Total: len(tasks),
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateTaskRequest struct {
	WorkspaceID       string                    `json:"workspace_id"`
	WorkflowID        string                    `json:"workflow_id"`
	WorkflowStepID    string                    `json:"workflow_step_id"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description,omitempty"`
	Priority          int                       `json:"priority,omitempty"`
	State             *v1.TaskState             `json:"state,omitempty"`
	Repositories      []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position          int                       `json:"position,omitempty"`
	Metadata          map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent        bool                      `json:"start_agent,omitempty"`
	AgentProfileID    string                    `json:"agent_profile_id,omitempty"`
	ExecutorID        string                    `json:"executor_id,omitempty"`
	ExecutorProfileID string                    `json:"executor_profile_id,omitempty"`
	PlanMode          bool                      `json:"plan_mode,omitempty"`
}

func (h *TaskHandlers) wsCreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.Title == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "title is required", nil)
	}
	if req.StartAgent && req.AgentProfileID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "agent_profile_id is required to start agent", nil)
	}

	// Convert repositories
	var repos []dto.TaskRepositoryInput
	for _, r := range req.Repositories {
		if r.RepositoryID == "" && r.LocalPath == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "repository_id or local_path is required", nil)
		}
		repos = append(repos, dto.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	title := strings.TrimSpace(req.Title)
	description := strings.TrimSpace(req.Description)

	task, err := h.service.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          title,
		Description:    description,
		Priority:       req.Priority,
		State:          req.State,
		Repositories:   convertToServiceRepos(repos),
		Position:       req.Position,
		Metadata:       req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	taskDTO := dto.FromTask(task)
	response := createTaskResponse{TaskDTO: taskDTO}
	if req.StartAgent && req.AgentProfileID != "" && h.orchestrator != nil {
		// Use task description as the initial prompt with workflow step config (prompt prefix/suffix, plan mode)
		// Use taskDTO.WorkflowStepID (backend-resolved) instead of req.WorkflowStepID
		execution, err := h.orchestrator.StartTask(ctx, taskDTO.ID, req.AgentProfileID, req.ExecutorID, req.ExecutorProfileID, req.Priority, taskDTO.Description, taskDTO.WorkflowStepID, req.PlanMode)
		if err != nil {
			h.logger.Error("failed to start agent for task", zap.Error(err))
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to start agent for task", nil)
		}
		h.logger.Info("wsCreateTask started agent",
			zap.String("task_id", taskDTO.ID),
			zap.String("executor_id", req.ExecutorID),
			zap.String("workflow_step_id", req.WorkflowStepID),
			zap.String("session_id", execution.SessionID))
		response.TaskSessionID = execution.SessionID
		response.AgentExecutionID = execution.AgentExecutionID
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

type wsGetTaskRequest struct {
	ID string `json:"id"`
}

func (h *TaskHandlers) wsGetTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	task, err := h.service.GetTask(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

type wsUpdateTaskRequest struct {
	ID           string                    `json:"id"`
	Title        *string                   `json:"title,omitempty"`
	Description  *string                   `json:"description,omitempty"`
	Priority     *int                      `json:"priority,omitempty"`
	State        *v1.TaskState             `json:"state,omitempty"`
	Repositories []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position     *int                      `json:"position,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
}

func (h *TaskHandlers) wsUpdateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	// Convert repositories if provided
	var repos []dto.TaskRepositoryInput
	if req.Repositories != nil {
		for _, r := range req.Repositories {
			repos = append(repos, dto.TaskRepositoryInput{
				RepositoryID:  r.RepositoryID,
				BaseBranch:    r.BaseBranch,
				LocalPath:     r.LocalPath,
				Name:          r.Name,
				DefaultBranch: r.DefaultBranch,
			})
		}
	}

	// Trim strings like the controller did
	var title *string
	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		title = &trimmed
	}
	var description *string
	if req.Description != nil {
		trimmed := strings.TrimSpace(*req.Description)
		description = &trimmed
	}

	task, err := h.service.UpdateTask(ctx, req.ID, &service.UpdateTaskRequest{
		Title:        title,
		Description:  description,
		Priority:     req.Priority,
		State:        req.State,
		Repositories: convertToServiceRepos(repos),
		Position:     req.Position,
		Metadata:     req.Metadata,
	})
	if err != nil {
		h.logger.Error("failed to update task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

func (h *TaskHandlers) wsDeleteTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsHandleIDRequest(ctx, msg, h.logger, "failed to delete task",
		func(ctx context.Context, id string) (any, error) {
			if err := h.service.DeleteTask(ctx, id); err != nil {
				return nil, err
			}
			return dto.SuccessResponse{Success: true}, nil
		})
}

func (h *TaskHandlers) wsArchiveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsHandleIDRequest(ctx, msg, h.logger, "failed to archive task",
		func(ctx context.Context, id string) (any, error) {
			if err := h.service.ArchiveTask(ctx, id); err != nil {
				return nil, err
			}
			return dto.SuccessResponse{Success: true}, nil
		})
}

type wsMoveTaskRequest struct {
	ID             string `json:"id"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	Position       int    `json:"position"`
}

func (h *TaskHandlers) wsMoveTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsMoveTaskRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}
	if req.WorkflowStepID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_step_id is required", nil)
	}

	result, err := h.service.MoveTask(ctx, req.ID, req.WorkflowID, req.WorkflowStepID, req.Position)
	if err != nil {
		h.logger.Error("failed to move task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to move task", nil)
	}

	response := dto.MoveTaskResponse{
		Task: dto.FromTask(result.Task),
	}
	if result.WorkflowStep != nil {
		response.WorkflowStep = dto.FromWorkflowStep(result.WorkflowStep)
	}
	return ws.NewResponse(msg.ID, msg.Action, response)
}

type wsUpdateTaskStateRequest struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func (h *TaskHandlers) wsUpdateTaskState(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateTaskStateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if req.State == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "state is required", nil)
	}

	task, err := h.service.UpdateTaskState(ctx, req.ID, v1.TaskState(req.State))
	if err != nil {
		h.logger.Error("failed to update task state", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task state", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}
