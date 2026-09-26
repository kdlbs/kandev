package store

import (
	"context"

	"github.com/kandev/kandev/internal/utility/models"
)

// Repository defines the interface for utility agent storage.
type Repository interface {
	// Agents
	ListAgents(ctx context.Context) ([]*models.UtilityAgent, error)
	GetAgentByID(ctx context.Context, id string) (*models.UtilityAgent, error)
	GetAgentByName(ctx context.Context, name string) (*models.UtilityAgent, error)
	CreateAgent(ctx context.Context, agent *models.UtilityAgent) error
	UpdateAgent(ctx context.Context, agent *models.UtilityAgent) error
	// NormalizeEmptyBuiltinBinding changes an empty, unconfigured built-in to
	// inherit only while the row still has that exact stale binding.
	NormalizeEmptyBuiltinBinding(ctx context.Context, id string) (bool, error)
	DeleteAgent(ctx context.Context, id string) error

	// Calls (history)
	ListCalls(ctx context.Context, utilityID string, limit int) ([]*models.UtilityAgentCall, error)
	GetCallByID(ctx context.Context, id string) (*models.UtilityAgentCall, error)
	CreateCall(ctx context.Context, call *models.UtilityAgentCall) error
	UpdateCall(ctx context.Context, call *models.UtilityAgentCall) error
}
