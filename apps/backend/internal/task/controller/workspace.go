package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

type WorkspaceController struct {
	service *service.Service
}

func NewWorkspaceController(svc *service.Service) *WorkspaceController {
	return &WorkspaceController{service: svc}
}

func (c *WorkspaceController) ListWorkspaces(ctx context.Context, _ dto.ListWorkspacesRequest) (dto.ListWorkspacesResponse, error) {
	workspaces, err := c.service.ListWorkspaces(ctx)
	if err != nil {
		return dto.ListWorkspacesResponse{}, err
	}
	resp := dto.ListWorkspacesResponse{
		Workspaces: make([]dto.WorkspaceDTO, 0, len(workspaces)),
		Total:      len(workspaces),
	}
	for _, workspace := range workspaces {
		resp.Workspaces = append(resp.Workspaces, dto.FromWorkspace(workspace))
	}
	return resp, nil
}

func (c *WorkspaceController) GetWorkspace(ctx context.Context, req dto.GetWorkspaceRequest) (dto.WorkspaceDTO, error) {
	workspace, err := c.service.GetWorkspace(ctx, req.ID)
	if err != nil {
		return dto.WorkspaceDTO{}, err
	}
	return dto.FromWorkspace(workspace), nil
}

func (c *WorkspaceController) CreateWorkspace(ctx context.Context, req dto.CreateWorkspaceRequest) (dto.WorkspaceDTO, error) {
	workspace, err := c.service.CreateWorkspace(ctx, &service.CreateWorkspaceRequest{
		Name:                  req.Name,
		Description:           req.Description,
		OwnerID:               req.OwnerID,
		DefaultExecutorID:     req.DefaultExecutorID,
		DefaultEnvironmentID:  req.DefaultEnvironmentID,
		DefaultAgentProfileID: req.DefaultAgentProfileID,
	})
	if err != nil {
		return dto.WorkspaceDTO{}, err
	}
	return dto.FromWorkspace(workspace), nil
}

func (c *WorkspaceController) UpdateWorkspace(ctx context.Context, req dto.UpdateWorkspaceRequest) (dto.WorkspaceDTO, error) {
	workspace, err := c.service.UpdateWorkspace(ctx, req.ID, &service.UpdateWorkspaceRequest{
		Name:                  req.Name,
		Description:           req.Description,
		DefaultExecutorID:     req.DefaultExecutorID,
		DefaultEnvironmentID:  req.DefaultEnvironmentID,
		DefaultAgentProfileID: req.DefaultAgentProfileID,
	})
	if err != nil {
		return dto.WorkspaceDTO{}, err
	}
	return dto.FromWorkspace(workspace), nil
}

func (c *WorkspaceController) DeleteWorkspace(ctx context.Context, req dto.DeleteWorkspaceRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteWorkspace(ctx, req.ID); err != nil {
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}
