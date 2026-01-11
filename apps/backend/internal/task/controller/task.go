package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

type TaskController struct {
	service *service.Service
}

func NewTaskController(svc *service.Service) *TaskController {
	return &TaskController{service: svc}
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
		resp.Tasks = append(resp.Tasks, dto.FromTask(task))
	}
	return resp, nil
}

func (c *TaskController) GetTask(ctx context.Context, req dto.GetTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.GetTask(ctx, req.ID)
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.CreateTask(ctx, &service.CreateTaskRequest{
		BoardID:       req.BoardID,
		ColumnID:      req.ColumnID,
		Title:         req.Title,
		Description:   req.Description,
		Priority:      req.Priority,
		AgentType:     req.AgentType,
		RepositoryURL: req.RepositoryURL,
		Branch:        req.Branch,
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
		AgentType:   req.AgentType,
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
	task, err := c.service.MoveTask(ctx, req.ID, req.ColumnID, req.Position)
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
