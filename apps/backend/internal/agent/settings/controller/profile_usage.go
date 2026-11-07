package controller

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

var ErrProviderUsageDisabled = errors.New("provider usage snapshots are disabled")

type ProfileUsageProvider interface {
	ListProfileUsage(context.Context) ([]dto.AgentProfileUsageDTO, error)
}

// SetProfileUsageProvider is called only when the native feature is enabled.
func (c *Controller) SetProfileUsageProvider(provider ProfileUsageProvider) {
	c.profileUsage = provider
}

func (c *Controller) ListProfileUsage(ctx context.Context) (*dto.ListAgentProfileUsageResponse, error) {
	if c.profileUsage == nil {
		return nil, ErrProviderUsageDisabled
	}
	profiles, err := c.profileUsage.ListProfileUsage(ctx)
	if err != nil {
		return nil, err
	}
	if profiles == nil {
		profiles = []dto.AgentProfileUsageDTO{}
	}
	return &dto.ListAgentProfileUsageResponse{Profiles: profiles}, nil
}
