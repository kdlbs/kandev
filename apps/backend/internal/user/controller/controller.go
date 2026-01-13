package controller

import (
	"context"

	"github.com/kandev/kandev/internal/user/dto"
	"github.com/kandev/kandev/internal/user/service"
)

type Controller struct {
	svc *service.Service
}

func NewController(svc *service.Service) *Controller {
	return &Controller{svc: svc}
}

func (c *Controller) GetCurrentUser(ctx context.Context) (dto.UserResponse, error) {
	user, err := c.svc.GetCurrentUser(ctx)
	if err != nil {
		return dto.UserResponse{}, err
	}
	settings, err := c.svc.GetUserSettings(ctx)
	if err != nil {
		return dto.UserResponse{}, err
	}
	return dto.UserResponse{
		User:     dto.FromUser(user),
		Settings: dto.FromUserSettings(settings),
	}, nil
}

func (c *Controller) GetUserSettings(ctx context.Context) (dto.UserSettingsResponse, error) {
	settings, err := c.svc.GetUserSettings(ctx)
	if err != nil {
		return dto.UserSettingsResponse{}, err
	}
	return dto.UserSettingsResponse{Settings: dto.FromUserSettings(settings)}, nil
}

func (c *Controller) UpdateUserSettings(ctx context.Context, req dto.UpdateUserSettingsRequest) (dto.UserSettingsResponse, error) {
	settings, err := c.svc.UpdateUserSettings(ctx, &service.UpdateUserSettingsRequest{
		WorkspaceID:   req.WorkspaceID,
		BoardID:       req.BoardID,
		RepositoryIDs: req.RepositoryIDs,
	})
	if err != nil {
		return dto.UserSettingsResponse{}, err
	}
	return dto.UserSettingsResponse{Settings: dto.FromUserSettings(settings)}, nil
}
