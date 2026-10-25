package engine_adapters

import (
	"context"

	"github.com/kandev/kandev/internal/workflow/engine"
)

// AgentProfileExistenceRepo captures the office-repo subset needed to answer
// whether an agent profile still exists and is not soft-deleted. This must
// resolve any agent_profiles row — Office agent or shallow Kanban profile —
// since a seat's agent_profile_id can come from the task's runner rather
// than an Office-scoped candidate.
type AgentProfileExistenceRepo interface {
	AgentProfileExists(ctx context.Context, id string) (bool, error)
}

// AgentProfileResolverAdapter implements engine.AgentProfileResolver against
// the office agent_profiles table, for the quorum guard's
// REQ-OFFICE-REVIEW-SEATS-004.3 skip.
type AgentProfileResolverAdapter struct {
	Office AgentProfileExistenceRepo
}

// NewAgentProfileResolverAdapter builds an AgentProfileResolverAdapter
// wrapping the office repo.
func NewAgentProfileResolverAdapter(office AgentProfileExistenceRepo) *AgentProfileResolverAdapter {
	return &AgentProfileResolverAdapter{Office: office}
}

// AgentProfileExists satisfies engine.AgentProfileResolver.
func (a *AgentProfileResolverAdapter) AgentProfileExists(ctx context.Context, agentProfileID string) (bool, error) {
	if agentProfileID == "" {
		return false, nil
	}
	return a.Office.AgentProfileExists(ctx, agentProfileID)
}

// Compile-time interface assertion.
var _ engine.AgentProfileResolver = (*AgentProfileResolverAdapter)(nil)
