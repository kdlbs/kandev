// Package registry manages available agent types and their configurations.
package registry

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
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
)

// Registry manages agent configurations
type Registry struct {
	agents map[string]agents.Agent
	loaded bool
	mu     sync.RWMutex
	logger *logger.Logger
	// managedRuntimeSelections supplies exact versions to host-local provider
	// recovery probes. It is wired after the settings store is available.
	managedRuntimeSelections managedruntime.SelectionReader
}

// SetManagedRuntimeSelectionStore wires the install-wide exact-version reader
// used by host-local managed-runtime recovery probes.
func (r *Registry) SetManagedRuntimeSelectionStore(store managedruntime.SelectionReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.managedRuntimeSelections = store
}

func (r *Registry) managedRuntimeSelectionReader() managedruntime.SelectionReader {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.managedRuntimeSelections
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
		agents.NewDynamicAgent(),
		agents.NewAuggie(),
		agents.NewClaudeACP(),
		agents.NewCodexACP(),
		agents.NewCopilotACP(),
		agents.NewGemini(),
		agents.NewOpenCodeACP(),
		agents.NewAmpACP(),
		agents.NewQwenACP(),
		agents.NewIFlowACP(),
		agents.NewDroidACP(),
		agents.NewKilocodeACP(),
		agents.NewPiACP(),
		agents.NewCursorACP(),
		agents.NewKimiACP(),
		agents.NewKiroACP(),
		agents.NewQoderACP(),
		agents.NewTraeACP(),
		agents.NewOmpACP(),
		agents.NewDevinACP(),
		agents.NewGrokACP(),
		agents.NewHermesACP(),
		agents.NewGooseACP(),
		agents.NewAntigravityACP(),
		agents.NewMockAgent(),
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ag := range all {
		r.agents[ag.ID()] = ag
		r.logger.Debug("loaded default agent type", zap.String("id", ag.ID()))
	}
	r.loaded = true
}

// MarkLoaded records that the initial registry population completed.
func (r *Registry) MarkLoaded() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loaded = true
}

// IsLoaded reports whether the initial registry population completed.
func (r *Registry) IsLoaded() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loaded
}

// Get returns an agent by ID
func (r *Registry) Get(id string) (agents.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ag, exists := r.agents[id]
	return ag, exists
}

// GetDefault returns the default agent.
// It tries "auggie" first, then falls back to the enabled agent with the lowest DisplayOrder.
func (r *Registry) GetDefault() (agents.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if ag, exists := r.agents["auggie"]; exists && ag.Enabled() {
		return ag, nil
	}

	// Collect enabled agents and sort by DisplayOrder for deterministic fallback
	var enabled []agents.Agent
	for _, ag := range r.agents {
		if ag.Enabled() && !agents.IsVirtualAgent(ag) {
			enabled = append(enabled, ag)
		}
	}
	if len(enabled) == 0 {
		return nil, fmt.Errorf("no default agent type available")
	}
	sortByDisplayOrder(enabled)
	return enabled[0], nil
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

// ListInferenceAgents returns enabled agents that implement InferenceAgent.
func (r *Registry) ListInferenceAgents() []agents.InferenceAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []agents.InferenceAgent
	var forSort []agents.Agent
	for _, ag := range r.agents {
		if !ag.Enabled() {
			continue
		}
		if ia, ok := ag.(agents.InferenceAgent); ok {
			if cfg := ia.InferenceConfig(); cfg != nil && cfg.Supported {
				result = append(result, ia)
				forSort = append(forSort, ag)
			}
		}
	}
	// Sort by DisplayOrder
	slices.SortStableFunc(result, func(a, b agents.InferenceAgent) int {
		aIdx, bIdx := -1, -1
		for i, ag := range forSort {
			if ag.(agents.InferenceAgent) == a {
				aIdx = i
			}
			if ag.(agents.InferenceAgent) == b {
				bIdx = i
			}
		}
		return cmp.Compare(forSort[aIdx].DisplayOrder(), forSort[bIdx].DisplayOrder())
	})
	return result
}

// GetInferenceAgent returns an inference-capable agent by ID.
func (r *Registry) GetInferenceAgent(id string) (agents.InferenceAgent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ag, exists := r.agents[id]
	if !exists || !ag.Enabled() {
		return nil, false
	}
	ia, ok := ag.(agents.InferenceAgent)
	if !ok {
		return nil, false
	}
	if cfg := ia.InferenceConfig(); cfg == nil || !cfg.Supported {
		return nil, false
	}
	return ia, true
}

// Exists checks if an agent exists
func (r *Registry) Exists(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.agents[id]
	return exists
}

// ResolveFamilyIDs maps a human-written agent family reference onto the canonical
// IDs of every registered agent that answers to it, sorted. Empty means the
// reference names no known agent; one entry resolves it; more than one means the
// reference is ambiguous.
//
// Workflow definitions are hand-authored and reasonably say "Claude" where the
// runtime stores "claude-acp", so the display name and the agent name are
// accepted as aliases, case-insensitively and ignoring surrounding whitespace.
// Aliases are NOT unique: a custom TUI agent picks its own slug and display name
// and shares this namespace with the built-ins, so "Claude" can name both the
// built-in and a user's own agent. Guessing between them silently mis-routes a
// workflow rule, so the whole candidate set is returned rather than one arbitrary
// winner — which candidates those are decides whether an ambiguous reference is
// relevant to a given caller at all, and only the caller knows that.
//
// A canonical ID written exactly, byte for byte, is unambiguous by construction —
// IDs are the registry's keys — and resolves alone even when another agent claims
// it as an alias. That precedence is case-SENSITIVE, unlike the alias scan below
// it: "claude-acp" takes the map lookup, while "Claude-ACP" falls through to the
// scan and can therefore come back ambiguous if some other agent answers to that
// spelling too. Writing the ID as the registry spells it is what buys the
// guarantee; anything else is an alias and is treated like one.
func (r *Registry) ResolveFamilyIDs(name string) []string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if ag, exists := r.agents[trimmed]; exists {
		return []string{ag.ID()}
	}
	matches := make([]string, 0, 1)
	for _, ag := range r.agents {
		if agentAnswersToFamily(ag, trimmed) {
			matches = append(matches, ag.ID())
		}
	}
	if len(matches) == 0 {
		return nil
	}
	// Map iteration order is random; callers compare and report these.
	slices.Sort(matches)
	return matches
}

// agentAnswersToFamily reports whether any of an agent's identifiers is the
// given family reference, ignoring case and surrounding whitespace.
func agentAnswersToFamily(ag agents.Agent, reference string) bool {
	for _, alias := range []string{ag.ID(), ag.DisplayName(), ag.Name()} {
		if strings.EqualFold(strings.TrimSpace(alias), reference) {
			return true
		}
	}
	return false
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

// Replace registers ag under its ID, overwriting any previously
// registered agent for that ID. Returns an error only when ag has no
// ID. Used by the dev/E2E mock-provider seam to swap a real provider
// agent for a MockAgent alias without going through Unregister +
// Register (which would race with concurrent readers).
func (r *Registry) Replace(ag agents.Agent) error {
	if ag.ID() == "" {
		return fmt.Errorf("agent type ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, existed := r.agents[ag.ID()]
	r.agents[ag.ID()] = ag
	if existed {
		r.logger.Info("replaced agent type", zap.String("id", ag.ID()))
	} else {
		r.logger.Info("registered agent type", zap.String("id", ag.ID()))
	}
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
		Enabled:         ag.Enabled(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}
