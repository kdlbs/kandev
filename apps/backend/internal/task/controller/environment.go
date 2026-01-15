package controller

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

type EnvironmentController struct {
	service *service.Service
}

func NewEnvironmentController(svc *service.Service) *EnvironmentController {
	return &EnvironmentController{service: svc}
}

func (c *EnvironmentController) ListEnvironments(ctx context.Context, _ dto.ListEnvironmentsRequest) (dto.ListEnvironmentsResponse, error) {
	environments, err := c.service.ListEnvironments(ctx)
	if err != nil {
		return dto.ListEnvironmentsResponse{}, err
	}
	resp := dto.ListEnvironmentsResponse{
		Environments: make([]dto.EnvironmentDTO, 0, len(environments)),
		Total:        len(environments),
	}
	for _, environment := range environments {
		resp.Environments = append(resp.Environments, dto.FromEnvironment(environment))
	}
	return resp, nil
}

func (c *EnvironmentController) GetEnvironment(ctx context.Context, req dto.GetEnvironmentRequest) (dto.EnvironmentDTO, error) {
	environment, err := c.service.GetEnvironment(ctx, req.ID)
	if err != nil {
		return dto.EnvironmentDTO{}, err
	}
	return dto.FromEnvironment(environment), nil
}

func (c *EnvironmentController) CreateEnvironment(ctx context.Context, req dto.CreateEnvironmentRequest) (dto.EnvironmentDTO, error) {
	environment, err := c.service.CreateEnvironment(ctx, &service.CreateEnvironmentRequest{
		Name:         req.Name,
		Kind:         req.Kind,
		WorktreeRoot: req.WorktreeRoot,
		ImageTag:     req.ImageTag,
		Dockerfile:   req.Dockerfile,
		BuildConfig:  req.BuildConfig,
	})
	if err != nil {
		return dto.EnvironmentDTO{}, err
	}
	return dto.FromEnvironment(environment), nil
}

func (c *EnvironmentController) UpdateEnvironment(ctx context.Context, req dto.UpdateEnvironmentRequest) (dto.EnvironmentDTO, error) {
	environment, err := c.service.UpdateEnvironment(ctx, req.ID, &service.UpdateEnvironmentRequest{
		Name:         req.Name,
		Kind:         req.Kind,
		WorktreeRoot: req.WorktreeRoot,
		ImageTag:     req.ImageTag,
		Dockerfile:   req.Dockerfile,
		BuildConfig:  req.BuildConfig,
	})
	if err != nil {
		return dto.EnvironmentDTO{}, err
	}
	return dto.FromEnvironment(environment), nil
}

func (c *EnvironmentController) DeleteEnvironment(ctx context.Context, req dto.DeleteEnvironmentRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteEnvironment(ctx, req.ID); err != nil {
		if errors.Is(err, service.ErrActiveTaskSessions) {
			return dto.SuccessResponse{}, ErrActiveTaskSessions
		}
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}
