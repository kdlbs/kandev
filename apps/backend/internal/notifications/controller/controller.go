package controller

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
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

// callerUserID resolves the user whose providers this request may touch: the
// authenticated identity when there is one, otherwise the pre-auth default
// user. A synthetic identity means authentication is disabled, so it maps to
// the same default row single-user installs have always used.
func callerUserID(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic || identity.UserID == "" {
		return userstore.DefaultUserID
	}
	return identity.UserID
}

func (c *Controller) ListProviders(ctx context.Context) (dto.NotificationProvidersResponse, error) {
	userID := callerUserID(ctx)
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
	userID := callerUserID(ctx)
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
	events := []string{service.EventTaskSessionClarificationAsked}
	if req.Events != nil {
		events = *req.Events
	}
	provider, err := c.service.CreateProvider(ctx, userID, name, providerType, config, enabled, events)
	if err != nil {
		return dto.NotificationProviderDTO{}, err
	}
	return dto.FromProvider(provider, events), nil
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
	userID := callerUserID(ctx)
	provider, err := c.service.UpdateProvider(ctx, userID, providerID, updates)
	if err != nil {
		return dto.NotificationProviderDTO{}, err
	}
	_, subscriptions, err := c.service.ListProviders(ctx, userID)
	if err != nil {
		return dto.NotificationProviderDTO{}, err
	}
	return dto.FromProvider(provider, subscriptions[provider.ID]), nil
}

func (c *Controller) DeleteProvider(ctx context.Context, providerID string) error {
	return c.service.DeleteProvider(ctx, callerUserID(ctx), providerID)
}

func (c *Controller) TestProvider(ctx context.Context, providerID string) error {
	return c.service.TestProvider(ctx, callerUserID(ctx), providerID)
}

func defaultNameForType(providerType string) string {
	switch providerType {
	case string(models.ProviderTypeApprise):
		return "Apprise"
	case string(models.ProviderTypeSystem):
		return "System Notifications"
	default:
		return "Notifications"
	}
}
