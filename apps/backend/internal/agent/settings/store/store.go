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
	ListAgentProfiles(ctx context.Context, agentID string) ([]*models.AgentProfile, error)

	Close() error
}
