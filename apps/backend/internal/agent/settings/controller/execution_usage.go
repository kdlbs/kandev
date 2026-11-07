package controller

import (
	"context"
	"github.com/kandev/kandev/internal/agent/usage"
)

func (c *Controller) SetNativeUsageReader(reader func(context.Context, string) ([]usage.ProfileExecutionUsage, error)) {
	c.nativeUsageReader = reader
}

func (c *Controller) SetCachedNativeUsageReader(reader func(context.Context, string) ([]usage.ProfileExecutionUsage, error)) {
	c.cachedNativeUsageReader = reader
}

func (c *Controller) ReadNativeProfileUsage(ctx context.Context, profileID string) ([]usage.ProfileExecutionUsage, error) {
	return c.readProfileUsage(ctx, profileID, c.nativeUsageReader)
}

func (c *Controller) ReadCachedNativeProfileUsage(ctx context.Context, profileID string) ([]usage.ProfileExecutionUsage, error) {
	return c.readProfileUsage(ctx, profileID, c.cachedNativeUsageReader)
}

func (c *Controller) readProfileUsage(ctx context.Context, profileID string, reader func(context.Context, string) ([]usage.ProfileExecutionUsage, error)) ([]usage.ProfileExecutionUsage, error) {
	if reader == nil {
		return nil, ErrAgentFeatureDisabled
	}
	exists, err := c.AgentProfileExists(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrAgentNotFound
	}
	return reader(ctx, profileID)
}
