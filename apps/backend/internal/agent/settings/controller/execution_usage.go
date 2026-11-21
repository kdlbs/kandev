package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/usage"
)

func (c *Controller) SetNativeUsageReader(reader func(context.Context, string) ([]usage.ProfileExecutionUsage, error)) {
	c.nativeUsageReader = reader
}

func (c *Controller) ReadNativeProfileUsage(ctx context.Context, profileID string) ([]usage.ProfileExecutionUsage, error) {
	if c.nativeUsageReader == nil {
		return nil, ErrAgentFeatureDisabled
	}
	exists, err := c.AgentProfileExists(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrAgentNotFound
	}
	return c.nativeUsageReader(ctx, profileID)
}
