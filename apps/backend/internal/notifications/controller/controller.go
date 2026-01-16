package controller

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/notifications/dto"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/service"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type Controller struct {
	service *service.Service
}

func NewController(svc *service.Service) *Controller {
	return &Controller{service: svc}
}

func (c *Controller) ListProviders(ctx context.Context) (dto.NotificationProvidersResponse, error) {
	userID := userstore.DefaultUserID
	providers, subscriptions, err := c.service.ListProviders(ctx, userID)
	if err != nil {
		return dto.NotificationProvidersResponse{}, err
	}
	result := make([]dto.NotificationProviderDTO, 0, len(providers))
	for _, provider := range providers {
		events := subscriptions[provider.ID]
		result = append(result, dto.FromProvider(provider, events))
	}
	return dto.NotificationProvidersResponse{
		Providers:        result,
		AppriseAvailable: c.service.AppriseAvailable(),
		Events:           c.service.AvailableEvents(),
	}, nil
}

func (c *Controller) CreateProvider(ctx context.Context, req dto.UpsertProviderRequest) (dto.NotificationProviderDTO, error) {
	userID := userstore.DefaultUserID
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = defaultNameForType(req.Type)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	providerType := models.ProviderType(req.Type)
	config := req.Config
	provider, err := c.service.CreateProvider(ctx, userID, name, providerType, config, enabled, req.Events)
	if err != nil {
		return dto.NotificationProviderDTO{}, err
	}
	return dto.FromProvider(provider, req.Events), nil
}

func (c *Controller) UpdateProvider(ctx context.Context, providerID string, req dto.UpdateProviderRequest) (dto.NotificationProviderDTO, error) {
	var providerType *models.ProviderType
	if req.Type != nil {
		t := models.ProviderType(*req.Type)
		providerType = &t
	}
	updates := service.ProviderUpdate{
		Name:    req.Name,
		Enabled: req.Enabled,
		Type:    providerType,
		Config:  req.Config,
		Events:  req.Events,
	}
	provider, err := c.service.UpdateProvider(ctx, providerID, updates)
	if err != nil {
		return dto.NotificationProviderDTO{}, err
	}
	userID := userstore.DefaultUserID
	_, subscriptions, err := c.service.ListProviders(ctx, userID)
	if err != nil {
		return dto.NotificationProviderDTO{}, err
	}
	return dto.FromProvider(provider, subscriptions[provider.ID]), nil
}

func (c *Controller) DeleteProvider(ctx context.Context, providerID string) error {
	return c.service.DeleteProvider(ctx, providerID)
}

func defaultNameForType(providerType string) string {
	switch providerType {
	case string(models.ProviderTypeApprise):
		return "Apprise"
	default:
		return "Notifications"
	}
}
