package engine_adapters

import (
	"context"

	"github.com/kandev/kandev/internal/workflow/engine"
)

// AgentProfileExistenceRepo captures the office-repo subset needed to answer
// whether an agent profile still exists and is not soft-deleted.
type AgentProfileExistenceRepo interface {
	AgentInstanceExists(ctx context.Context, id string) (bool, error)
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
	return a.Office.AgentInstanceExists(ctx, agentProfileID)
}

// Compile-time interface assertion.
var _ engine.AgentProfileResolver = (*AgentProfileResolverAdapter)(nil)
