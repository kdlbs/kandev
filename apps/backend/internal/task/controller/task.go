package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

// WorktreeLookup provides worktree information for tasks.
type WorktreeLookup interface {
	// GetWorktreeInfo returns worktree path and branch for a task, or nil if no worktree exists.
	GetWorktreeInfo(ctx context.Context, taskID string) (path, branch *string)
}

type TaskController struct {
	service         *service.Service
	worktreeLookup  WorktreeLookup
}

func NewTaskController(svc *service.Service) *TaskController {
	return &TaskController{service: svc}
}

// SetWorktreeLookup sets the worktree lookup for enriching task responses.
func (c *TaskController) SetWorktreeLookup(lookup WorktreeLookup) {
	c.worktreeLookup = lookup
}

// enrichWithWorktree adds worktree info to a TaskDTO if available.
func (c *TaskController) enrichWithWorktree(ctx context.Context, task *dto.TaskDTO) {
	if c.worktreeLookup == nil {
		return
	}
	path, branch := c.worktreeLookup.GetWorktreeInfo(ctx, task.ID)
	task.WorktreePath = path
	task.WorktreeBranch = branch
}

func (c *TaskController) ListTasks(ctx context.Context, req dto.ListTasksRequest) (dto.ListTasksResponse, error) {
	tasks, err := c.service.ListTasks(ctx, req.BoardID)
	if err != nil {
		return dto.ListTasksResponse{}, err
	}
	resp := dto.ListTasksResponse{
		Tasks: make([]dto.TaskDTO, 0, len(tasks)),
		Total: len(tasks),
	}
	for _, task := range tasks {
		taskDTO := dto.FromTask(task)
		c.enrichWithWorktree(ctx, &taskDTO)
		resp.Tasks = append(resp.Tasks, taskDTO)
	}
	return resp, nil
}

func (c *TaskController) ListTaskSessions(ctx context.Context, req dto.ListTaskSessionsRequest) (dto.ListTaskSessionsResponse, error) {
	sessions, err := c.service.ListTaskSessions(ctx, req.TaskID)
	if err != nil {
		return dto.ListTaskSessionsResponse{}, err
	}
	resp := dto.ListTaskSessionsResponse{
		Sessions: make([]dto.TaskSessionDTO, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, session := range sessions {
		resp.Sessions = append(resp.Sessions, dto.FromTaskSession(session))
	}
	return resp, nil
}

func (c *TaskController) GetTask(ctx context.Context, req dto.GetTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.GetTask(ctx, req.ID)
	if err != nil {
		return dto.TaskDTO{}, err
	}
	taskDTO := dto.FromTask(task)
	c.enrichWithWorktree(ctx, &taskDTO)
	return taskDTO, nil
}

func (c *TaskController) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:   req.WorkspaceID,
		BoardID:       req.BoardID,
		ColumnID:      req.ColumnID,
		Title:         req.Title,
		Description:   req.Description,
		Priority:      req.Priority,
		State:         req.State,
		RepositoryID:  req.RepositoryID,
		BaseBranch:    req.BaseBranch,
		AssignedTo:    req.AssignedTo,
		Position:      req.Position,
		Metadata:      req.Metadata,
	})
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) UpdateTask(ctx context.Context, req dto.UpdateTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.UpdateTask(ctx, req.ID, &service.UpdateTaskRequest{
		Title:       req.Title,
		Description: req.Description,
		Priority:    req.Priority,
		State:       req.State,
		RepositoryID: req.RepositoryID,
		BaseBranch:   req.BaseBranch,
		AssignedTo:  req.AssignedTo,
		Position:    req.Position,
		Metadata:    req.Metadata,
	})
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) DeleteTask(ctx context.Context, req dto.DeleteTaskRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteTask(ctx, req.ID); err != nil {
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}

func (c *TaskController) MoveTask(ctx context.Context, req dto.MoveTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.MoveTask(ctx, req.ID, req.BoardID, req.ColumnID, req.Position)
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) UpdateTaskState(ctx context.Context, req dto.UpdateTaskStateRequest) (dto.TaskDTO, error) {
	task, err := c.service.UpdateTaskState(ctx, req.ID, req.State)
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}
