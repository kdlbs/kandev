package controller

import (
	"context"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// HostUsageLister lists subscription utilization for host-installed agent CLIs.
// Implemented by usage.HostService. fresh bypasses the 5-minute cache (still
// bounded by a short server-side clamp).
type HostUsageLister interface {
	List(ctx context.Context, fresh bool) []agentusage.HostAgentUsage
}

// SetHostUsageLister wires in the host subscription-usage service. Optional —
// when unset SubscriptionUsage returns an empty listing.
func (c *Controller) SetHostUsageLister(l HostUsageLister) {
	c.hostUsage = l
}

// SubscriptionUsage returns utilization for host agents authenticated with
// subscription (OAuth) credentials, enriched with registry display names.
func (c *Controller) SubscriptionUsage(ctx context.Context, fresh bool) *dto.AgentSubscriptionUsageResponse {
	resp := &dto.AgentSubscriptionUsageResponse{Agents: []dto.AgentSubscriptionUsage{}}
	if c.hostUsage == nil {
		return resp
	}
	for _, entry := range c.hostUsage.List(ctx, fresh) {
		displayName := entry.AgentID
		if ag, ok := c.agentRegistry.Get(entry.AgentID); ok {
			displayName = ag.DisplayName()
		}
		resp.Agents = append(resp.Agents, dto.AgentSubscriptionUsage{
			AgentID:     entry.AgentID,
			DisplayName: displayName,
			Usage:       entry.Usage,
			Error:       entry.Error,
		})
	}
	return resp
}
