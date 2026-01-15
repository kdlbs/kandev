package controller

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

type ExecutorController struct {
	service *service.Service
}

func NewExecutorController(svc *service.Service) *ExecutorController {
	return &ExecutorController{service: svc}
}

func (c *ExecutorController) ListExecutors(ctx context.Context, _ dto.ListExecutorsRequest) (dto.ListExecutorsResponse, error) {
	executors, err := c.service.ListExecutors(ctx)
	if err != nil {
		return dto.ListExecutorsResponse{}, err
	}
	resp := dto.ListExecutorsResponse{
		Executors: make([]dto.ExecutorDTO, 0, len(executors)),
		Total:     len(executors),
	}
	for _, executor := range executors {
		resp.Executors = append(resp.Executors, dto.FromExecutor(executor))
	}
	return resp, nil
}

func (c *ExecutorController) GetExecutor(ctx context.Context, req dto.GetExecutorRequest) (dto.ExecutorDTO, error) {
	executor, err := c.service.GetExecutor(ctx, req.ID)
	if err != nil {
		return dto.ExecutorDTO{}, err
	}
	return dto.FromExecutor(executor), nil
}

func (c *ExecutorController) CreateExecutor(ctx context.Context, req dto.CreateExecutorRequest) (dto.ExecutorDTO, error) {
	executor, err := c.service.CreateExecutor(ctx, &service.CreateExecutorRequest{
		Name:     req.Name,
		Type:     req.Type,
		Status:   req.Status,
		IsSystem: req.IsSystem,
		Config:   req.Config,
	})
	if err != nil {
		return dto.ExecutorDTO{}, err
	}
	return dto.FromExecutor(executor), nil
}

func (c *ExecutorController) UpdateExecutor(ctx context.Context, req dto.UpdateExecutorRequest) (dto.ExecutorDTO, error) {
	executor, err := c.service.UpdateExecutor(ctx, req.ID, &service.UpdateExecutorRequest{
		Name:   req.Name,
		Type:   req.Type,
		Status: req.Status,
		Config: req.Config,
	})
	if err != nil {
		return dto.ExecutorDTO{}, err
	}
	return dto.FromExecutor(executor), nil
}

func (c *ExecutorController) DeleteExecutor(ctx context.Context, req dto.DeleteExecutorRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteExecutor(ctx, req.ID); err != nil {
		if errors.Is(err, service.ErrActiveAgentSessions) {
			return dto.SuccessResponse{}, ErrActiveAgentSessions
		}
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}
