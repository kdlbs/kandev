// Package discovery provides agent installation detection and discovery functionality.
// It delegates to the agents.Agent interface for discovery and model information.
package discovery

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

const defaultCacheTTL = 30 * time.Second

// Capabilities describes what the agent supports.
type Capabilities struct {
	SupportsSessionResume bool `json:"supports_session_resume"`
	SupportsShell         bool `json:"supports_shell"`
	SupportsWorkspaceOnly bool `json:"supports_workspace_only"`
}

// KnownAgent represents an agent definition with discovery metadata.
type KnownAgent struct {
	Name              string         `json:"name"`
	DisplayName       string         `json:"display_name"`
	SupportsMCP       bool           `json:"supports_mcp"`
	MCPConfigPaths    []string       `json:"mcp_config_paths"`
	InstallationPaths []string       `json:"installation_paths"`
	Capabilities      Capabilities   `json:"capabilities"`
	DefaultModel      string         `json:"default_model"`
	Models            []agents.Model `json:"models"`
	SupportsDynamic   bool           `json:"supports_dynamic"`
}

// Availability represents the result of detecting an agent's installation.
type Availability struct {
	Name              string       `json:"name"`
	SupportsMCP       bool         `json:"supports_mcp"`
	MCPConfigPath     string       `json:"mcp_config_path,omitempty"`
	InstallationPaths []string     `json:"installation_paths,omitempty"`
	Available         bool         `json:"available"`
	MatchedPath       string       `json:"matched_path,omitempty"`
	Capabilities      Capabilities `json:"capabilities"`
}

// Registry manages agent discovery using the agents.Agent interface.
type Registry struct {
	agents      []agents.Agent
	definitions []KnownAgent
	logger      *logger.Logger

	mu            sync.RWMutex
	cachedResults []Availability
	cachedAt      time.Time
	cacheTTL      time.Duration
}

// LoadRegistry creates a new discovery registry from the agent registry.
// It iterates over all enabled agents, calls IsInstalled and ListModels
// to populate the KnownAgent definitions.
func LoadRegistry(ctx context.Context, reg *registry.Registry, log *logger.Logger) (*Registry, error) {
	enabled := reg.ListEnabled()

	definitions := make([]KnownAgent, 0, len(enabled))
	agentList := make([]agents.Agent, 0, len(enabled))

	for _, ag := range enabled {
		// Gather discovery info from the agent.
		result, err := ag.IsInstalled(ctx)
		if err != nil {
			log.Warn("discovery: failed to check agent installation",
				zap.String("agent", ag.ID()),
				zap.Error(err),
			)
			// Still include the agent but with empty discovery data.
			result = &agents.DiscoveryResult{}
		}

		// Gather model info from the agent.
		var models []agents.Model
		var supportsDynamic bool
		modelList, err := ag.ListModels(ctx)
		if err != nil {
			log.Warn("discovery: failed to list agent models",
				zap.String("agent", ag.ID()),
				zap.Error(err),
			)
		} else if modelList != nil {
			models = modelList.Models
			supportsDynamic = modelList.SupportsDynamic
		}

		displayName := ag.DisplayName()
		if displayName == "" {
			displayName = ag.Name()
		}

		knownAgent := KnownAgent{
			Name:              ag.ID(),
			DisplayName:       displayName,
			SupportsMCP:       result.SupportsMCP,
			MCPConfigPaths:    result.MCPConfigPaths,
			InstallationPaths: result.InstallationPaths,
			Capabilities: Capabilities{
				SupportsSessionResume: result.Capabilities.SupportsSessionResume,
				SupportsShell:         result.Capabilities.SupportsShell,
				SupportsWorkspaceOnly: result.Capabilities.SupportsWorkspaceOnly,
			},
			DefaultModel:    ag.DefaultModel(),
			Models:          models,
			SupportsDynamic: supportsDynamic,
		}

		definitions = append(definitions, knownAgent)
		agentList = append(agentList, ag)
	}

	return &Registry{
		agents:      agentList,
		definitions: definitions,
		logger:      log,
		cacheTTL:    defaultCacheTTL,
	}, nil
}

// Definitions returns a copy of all known agent definitions.
func (r *Registry) Definitions() []KnownAgent {
	if r == nil {
		return nil
	}
	return append([]KnownAgent(nil), r.definitions...)
}

// Detect checks whether each agent is installed by calling IsInstalled.
// Results are cached with a TTL to avoid redundant detection on repeated calls.
func (r *Registry) Detect(ctx context.Context) ([]Availability, error) {
	if cached := r.getCached(); cached != nil {
		return cached, nil
	}

	results := r.detectAll(ctx)

	r.mu.Lock()
	r.cachedResults = results
	r.cachedAt = time.Now()
	r.mu.Unlock()

	return results, nil
}

// InvalidateCache clears the cached detection results, forcing the next
// Detect call to re-run agent detection.
func (r *Registry) InvalidateCache() {
	r.mu.Lock()
	r.cachedResults = nil
	r.cachedAt = time.Time{}
	r.mu.Unlock()
}

func (r *Registry) getCached() []Availability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.cachedResults == nil {
		return nil
	}
	if time.Since(r.cachedAt) > r.cacheTTL {
		return nil
	}
	// Return a copy to prevent mutation.
	copied := make([]Availability, len(r.cachedResults))
	copy(copied, r.cachedResults)
	return copied
}

// detectAll runs IsInstalled for all agents concurrently.
func (r *Registry) detectAll(ctx context.Context) []Availability {
	type indexedResult struct {
		index int
		avail Availability
		err   error
	}

	ch := make(chan indexedResult, len(r.agents))
	for i, ag := range r.agents {
		go func(idx int, ag agents.Agent) {
			result, err := ag.IsInstalled(ctx)
			if err != nil {
				ch <- indexedResult{index: idx, err: err}
				return
			}

			mcpPath := ""
			if len(result.MCPConfigPaths) > 0 {
				mcpPath = result.MCPConfigPaths[0]
			}

			ch <- indexedResult{
				index: idx,
				avail: Availability{
					Name:              ag.ID(),
					SupportsMCP:       result.SupportsMCP,
					MCPConfigPath:     mcpPath,
					InstallationPaths: result.InstallationPaths,
					Available:         result.Available,
					MatchedPath:       result.MatchedPath,
					Capabilities: Capabilities{
						SupportsSessionResume: result.Capabilities.SupportsSessionResume,
						SupportsShell:         result.Capabilities.SupportsShell,
						SupportsWorkspaceOnly: result.Capabilities.SupportsWorkspaceOnly,
					},
				},
			}
		}(i, ag)
	}

	// Collect results preserving original order.
	slots := make([]Availability, len(r.agents))
	valid := make([]bool, len(r.agents))
	for range r.agents {
		res := <-ch
		if res.err != nil {
			r.logger.Warn("discovery: detect failed for agent",
				zap.String("agent", r.agents[res.index].ID()),
				zap.Error(res.err),
			)
			continue
		}
		slots[res.index] = res.avail
		valid[res.index] = true
	}

	results := make([]Availability, 0, len(r.agents))
	for i, v := range valid {
		if v {
			results = append(results, slots[i])
		}
	}
	return results
}
