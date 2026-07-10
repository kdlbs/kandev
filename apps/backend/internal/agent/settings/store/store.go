package store

import (
	"context"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

type Repository interface {
	CreateAgent(ctx context.Context, agent *models.Agent) error
	GetAgent(ctx context.Context, id string) (*models.Agent, error)
	GetAgentByName(ctx context.Context, name string) (*models.Agent, error)
	UpdateAgent(ctx context.Context, agent *models.Agent) error
	DeleteAgent(ctx context.Context, id string) error
	ListAgents(ctx context.Context) ([]*models.Agent, error)
	ListTUIAgents(ctx context.Context) ([]*models.Agent, error)

	GetAgentProfileMcpConfig(ctx context.Context, profileID string) (*models.AgentProfileMcpConfig, error)
	UpsertAgentProfileMcpConfig(ctx context.Context, config *models.AgentProfileMcpConfig) error

	CreateAgentProfile(ctx context.Context, profile *models.AgentProfile) error
	UpdateAgentProfile(ctx context.Context, profile *models.AgentProfile) error
	DeleteAgentProfile(ctx context.Context, id string) error
	GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error)
	// GetAgentProfileIncludingDeleted returns the row even when soft-deleted.
	// Check profile.DeletedAt != nil to detect orphaned references (watchers,
	// automations) pointing at removed profiles. ErrAgentProfileDeleted is
	// only used by callers of ProfileResolver, which wraps this method.
	GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error)
	ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error)
	// HasDeletedAgentProfiles reports whether the agent has any soft-deleted
	// profile rows. Seeding paths use this to distinguish a fresh agent that
	// has never been provisioned (no rows at all -> seed a default) from one
	// whose profiles the user deliberately deleted (deleted rows present ->
	// do not resurrect them on the next boot).
	HasDeletedAgentProfiles(ctx context.Context, agentID string) (bool, error)

	Close() error
}
