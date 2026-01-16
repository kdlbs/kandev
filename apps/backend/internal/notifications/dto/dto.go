package dto

import (
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
)

type NotificationProviderDTO struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Config    map[string]interface{} `json:"config"`
	Enabled   bool                   `json:"enabled"`
	Events    []string               `json:"events"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
}

type NotificationProvidersResponse struct {
	Providers        []NotificationProviderDTO `json:"providers"`
	AppriseAvailable bool                      `json:"apprise_available"`
	Events           []string                  `json:"events"`
}

type UpsertProviderRequest struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Config  map[string]interface{} `json:"config,omitempty"`
	Enabled *bool                  `json:"enabled,omitempty"`
	Events  []string               `json:"events,omitempty"`
}

type UpdateProviderRequest struct {
	Name    *string                `json:"name,omitempty"`
	Type    *string                `json:"type,omitempty"`
	Config  map[string]interface{} `json:"config,omitempty"`
	Enabled *bool                  `json:"enabled,omitempty"`
	Events  *[]string              `json:"events,omitempty"`
}

func FromProvider(provider *models.Provider, events []string) NotificationProviderDTO {
	return NotificationProviderDTO{
		ID:        provider.ID,
		Name:      provider.Name,
		Type:      string(provider.Type),
		Config:    provider.Config,
		Enabled:   provider.Enabled,
		Events:    events,
		CreatedAt: provider.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: provider.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
