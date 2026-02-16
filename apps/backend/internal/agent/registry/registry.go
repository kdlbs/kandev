// Package registry manages available agent types and their configurations.
package registry

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/common/logger"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Type aliases for backward compatibility during migration.
// These allow existing code to reference registry.PermissionSetting etc.
type (
	PermissionSetting     = agents.PermissionSetting
	MountTemplate         = agents.MountTemplate
	ResourceLimits        = agents.ResourceLimits
	SessionConfig         = agents.SessionConfig
	PassthroughConfig     = agents.PassthroughConfig
	DiscoveryCapabilities = agents.DiscoveryCapabilities
	Model                 = agents.Model
)

// Registry manages agent configurations
type Registry struct {
	agents map[string]agents.Agent
	mu     sync.RWMutex
	logger *logger.Logger
}

// NewRegistry creates a new agent registry
func NewRegistry(log *logger.Logger) *Registry {
	return &Registry{
		agents: make(map[string]agents.Agent),
		logger: log,
	}
}

// LoadDefaults loads default agent configurations
func (r *Registry) LoadDefaults() {
	all := []agents.Agent{
		agents.NewAuggie(),
		agents.NewClaudeCode(),
		agents.NewCodex(),
		agents.NewCopilot(),
		agents.NewGemini(),
		agents.NewOpenCode(),
		agents.NewAmp(),
		agents.NewMockAgent(),
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ag := range all {
		r.agents[ag.ID()] = ag
		r.logger.Debug("loaded default agent type", zap.String("id", ag.ID()))
	}
}

// Get returns an agent by ID
func (r *Registry) Get(id string) (agents.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ag, exists := r.agents[id]
	return ag, exists
}

// GetDefault returns the default agent.
// It tries "auggie" first, then falls back to the first enabled agent.
func (r *Registry) GetDefault() (agents.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if ag, exists := r.agents["auggie"]; exists && ag.Enabled() {
		return ag, nil
	}

	for _, ag := range r.agents {
		if ag.Enabled() {
			return ag, nil
		}
	}

	return nil, fmt.Errorf("no default agent type available")
}

// sortByDisplayOrder sorts agents by their DisplayOrder in ascending order.
func sortByDisplayOrder(list []agents.Agent) {
	slices.SortStableFunc(list, func(a, b agents.Agent) int {
		return cmp.Compare(a.DisplayOrder(), b.DisplayOrder())
	})
}

// List returns all registered agents
func (r *Registry) List() []agents.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]agents.Agent, 0, len(r.agents))
	for _, ag := range r.agents {
		result = append(result, ag)
	}
	sortByDisplayOrder(result)
	return result
}

// ListEnabled returns only enabled agents
func (r *Registry) ListEnabled() []agents.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]agents.Agent, 0, len(r.agents))
	for _, ag := range r.agents {
		if ag.Enabled() {
			result = append(result, ag)
		}
	}
	sortByDisplayOrder(result)
	return result
}

// Exists checks if an agent exists
func (r *Registry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.agents[id]
	return exists
}

// Register adds a new agent
func (r *Registry) Register(ag agents.Agent) error {
	if ag.ID() == "" {
		return fmt.Errorf("agent type ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[ag.ID()]; exists {
		return fmt.Errorf("agent type %q already registered", ag.ID())
	}

	r.agents[ag.ID()] = ag
	r.logger.Info("registered agent type", zap.String("id", ag.ID()))
	return nil
}

// Unregister removes an agent
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[id]; !exists {
		return fmt.Errorf("agent type %q not found", id)
	}

	delete(r.agents, id)
	r.logger.Info("unregistered agent type", zap.String("id", id))
	return nil
}

// ToAPIType converts an agent to an API response type
func ToAPIType(ag agents.Agent) *v1.AgentType {
	rt := ag.Runtime()
	return &v1.AgentType{
		ID:          ag.ID(),
		Name:        ag.Name(),
		Description: ag.Description(),
		DockerImage: rt.Image,
		DockerTag:   rt.Tag,
		DefaultResources: v1.ResourceLimits{
			CPULimit:    fmt.Sprintf("%.1f", rt.ResourceLimits.CPUCores),
			MemoryLimit: fmt.Sprintf("%dM", rt.ResourceLimits.MemoryMB),
		},
		EnvironmentVars: rt.Env,
		Capabilities:    rt.Capabilities,
		Enabled:         ag.Enabled(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}
