package controller

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

// ReorderAgentProfilesResult is the server's accepted order and revision.
type ReorderAgentProfilesResult struct {
	AgentID    string   `json:"agent_id"`
	ProfileIDs []string `json:"profile_ids"`
	Revision   int64    `json:"revision"`
	Changed    bool     `json:"-"`
}

// ReorderAgentProfiles saves a complete order for one installed global profile list.
func (c *Controller) ReorderAgentProfiles(ctx context.Context, agentID string, profileIDs []string) (*ReorderAgentProfilesResult, error) {
	agent, err := c.repo.GetAgent(ctx, agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAgentNotFound
	}
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	if agent.ID == agents.DynamicAgentID {
		return nil, ErrProfileOrderUnsupported
	}
	revision, changed, err := c.repo.ReorderAgentProfiles(ctx, agentID, profileIDs)
	if errors.Is(err, store.ErrProfileOrderAgentNotFound) {
		return nil, ErrAgentNotFound
	}
	if errors.Is(err, store.ErrProfileOrderSetMismatch) {
		return nil, ErrProfileOrderStale
	}
	if err != nil {
		return nil, err
	}
	ids := append([]string(nil), profileIDs...)
	return &ReorderAgentProfilesResult{AgentID: agentID, ProfileIDs: ids, Revision: revision, Changed: changed}, nil
}
