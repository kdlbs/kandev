// Package controller provides the business logic coordination layer for the orchestrator.
package controller

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/dto"
)

// Controller coordinates orchestrator business logic
type Controller struct {
	service *orchestrator.Service
}

// NewController creates a new orchestrator controller
func NewController(svc *orchestrator.Service) *Controller {
	return &Controller{
		service: svc,
	}
}

// GetStatus returns the orchestrator status
func (c *Controller) GetStatus(ctx context.Context, req dto.GetStatusRequest) (dto.StatusResponse, error) {
	status := c.service.GetStatus()
	return dto.StatusResponse{
		Running:       status.Running,
		ActiveAgents:  status.ActiveAgents,
		QueuedTasks:   status.QueuedTasks,
		MaxConcurrent: 0, // Not available from service Status; would need scheduler access
	}, nil
}

// GetQueue returns the queued tasks
func (c *Controller) GetQueue(ctx context.Context, req dto.GetQueueRequest) (dto.QueueResponse, error) {
	queuedTasks := c.service.GetQueuedTasks()

	tasks := make([]dto.QueuedTaskDTO, 0, len(queuedTasks))
	for _, qt := range queuedTasks {
		tasks = append(tasks, dto.QueuedTaskDTO{
			TaskID:   qt.TaskID,
			Priority: qt.Priority,
			QueuedAt: qt.QueuedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	return dto.QueueResponse{
		Tasks: tasks,
		Total: len(tasks),
	}, nil
}

// TriggerTask triggers a task for execution
func (c *Controller) TriggerTask(ctx context.Context, req dto.TriggerTaskRequest) (dto.TriggerTaskResponse, error) {
	// For now, just return success - the service would need task details
	return dto.TriggerTaskResponse{
		Success: true,
		Message: "Task triggered",
		TaskID:  req.TaskID,
	}, nil
}

// StartTask starts task execution
func (c *Controller) StartTask(ctx context.Context, req dto.StartTaskRequest) (dto.StartTaskResponse, error) {
	execution, err := c.service.StartTask(ctx, req.TaskID, req.AgentProfileID, req.Priority)
	if err != nil {
		return dto.StartTaskResponse{}, err
	}

	resp := dto.StartTaskResponse{
		Success:          true,
		TaskID:           execution.TaskID,
		AgentExecutionID: execution.AgentExecutionID,
		TaskSessionID:    execution.SessionID,
		State:            string(execution.SessionState),
	}

	// Include worktree info if available
	if execution.WorktreePath != "" {
		resp.WorktreePath = &execution.WorktreePath
	}
	if execution.WorktreeBranch != "" {
		resp.WorktreeBranch = &execution.WorktreeBranch
	}

	return resp, nil
}

// ResumeTaskSession resumes a specific task session execution.
func (c *Controller) ResumeTaskSession(ctx context.Context, req dto.ResumeTaskSessionRequest) (dto.ResumeTaskSessionResponse, error) {
	execution, err := c.service.ResumeTaskSession(ctx, req.TaskID, req.TaskSessionID)
	if err != nil {
		return dto.ResumeTaskSessionResponse{}, err
	}

	resp := dto.ResumeTaskSessionResponse{
		Success:          true,
		TaskID:           execution.TaskID,
		AgentExecutionID: execution.AgentExecutionID,
		TaskSessionID:    execution.SessionID,
		State:            string(execution.SessionState),
	}

	if execution.WorktreePath != "" {
		resp.WorktreePath = &execution.WorktreePath
	}
	if execution.WorktreeBranch != "" {
		resp.WorktreeBranch = &execution.WorktreeBranch
	}

	return resp, nil
}

// GetTaskSessionStatus returns the status of a task session including whether it's resumable
func (c *Controller) GetTaskSessionStatus(ctx context.Context, req dto.TaskSessionStatusRequest) (dto.TaskSessionStatusResponse, error) {
	status, err := c.service.GetTaskSessionStatus(ctx, req.TaskID, req.TaskSessionID)
	if err != nil {
		return dto.TaskSessionStatusResponse{
			SessionID: req.TaskSessionID,
			TaskID:    req.TaskID,
			Error:     err.Error(),
		}, nil
	}
	return status, nil
}

// StopTask stops task execution
func (c *Controller) StopTask(ctx context.Context, req dto.StopTaskRequest) (dto.SuccessResponse, error) {
	reason := req.Reason
	if reason == "" {
		reason = "stopped via API"
	}

	if err := c.service.StopTask(ctx, req.TaskID, reason, req.Force); err != nil {
		return dto.SuccessResponse{}, err
	}

	return dto.SuccessResponse{Success: true}, nil
}

// PromptTask sends a prompt to a running task
func (c *Controller) PromptTask(ctx context.Context, req dto.PromptTaskRequest) (dto.PromptTaskResponse, error) {
	result, err := c.service.PromptTask(ctx, req.TaskID, req.Prompt)
	if err != nil {
		return dto.PromptTaskResponse{}, err
	}

	return dto.PromptTaskResponse{
		Success:    true,
		StopReason: result.StopReason,
	}, nil
}

// CompleteTask marks a task as complete
func (c *Controller) CompleteTask(ctx context.Context, req dto.CompleteTaskRequest) (dto.CompleteTaskResponse, error) {
	if err := c.service.CompleteTask(ctx, req.TaskID); err != nil {
		return dto.CompleteTaskResponse{}, err
	}

	return dto.CompleteTaskResponse{
		Success: true,
		Message: "task completed",
	}, nil
}

// RespondToPermission responds to a permission request
func (c *Controller) RespondToPermission(ctx context.Context, req dto.PermissionRespondRequest) (dto.PermissionRespondResponse, error) {
	if err := c.service.RespondToPermission(ctx, req.TaskID, req.PendingID, req.OptionID, req.Cancelled); err != nil {
		return dto.PermissionRespondResponse{}, err
	}

	return dto.PermissionRespondResponse{
		Success:   true,
		TaskID:    req.TaskID,
		PendingID: req.PendingID,
	}, nil
}

// GetTaskExecution returns the execution state for a task
func (c *Controller) GetTaskExecution(ctx context.Context, req dto.GetTaskExecutionRequest) (dto.TaskExecutionResponse, error) {
	execution, exists := c.service.GetTaskExecution(req.TaskID)
	if !exists {
		return dto.TaskExecutionResponse{
			HasExecution: false,
			TaskID:       req.TaskID,
		}, nil
	}

	return dto.TaskExecutionResponse{
		HasExecution:     true,
		TaskID:           execution.TaskID,
		AgentExecutionID: execution.AgentExecutionID,
		AgentProfileID:   execution.AgentProfileID,
		TaskSessionID:    execution.SessionID,
		State:            string(execution.SessionState),
		Progress:         execution.Progress,
		StartedAt:        execution.StartedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}
