// Package discovery provides agent installation detection and discovery functionality.
// It delegates to the agents.Agent interface for discovery and model information.
package discovery

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

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
	Name              string   `json:"name"`
	SupportsMCP       bool     `json:"supports_mcp"`
	MCPConfigPath     string   `json:"mcp_config_path,omitempty"`
	InstallationPaths []string `json:"installation_paths,omitempty"`
	Available         bool     `json:"available"`
	MatchedPath       string   `json:"matched_path,omitempty"`
}

// Registry manages agent discovery using the agents.Agent interface.
type Registry struct {
	agents      []agents.Agent
	definitions []KnownAgent
	logger      *logger.Logger
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
func (r *Registry) Detect(ctx context.Context) ([]Availability, error) {
	results := make([]Availability, 0, len(r.agents))
	for _, ag := range r.agents {
		result, err := ag.IsInstalled(ctx)
		if err != nil {
			r.logger.Warn("discovery: detect failed for agent",
				zap.String("agent", ag.ID()),
				zap.Error(err),
			)
			continue
		}

		mcpPath := ""
		if len(result.MCPConfigPaths) > 0 {
			mcpPath = result.MCPConfigPaths[0]
		}

		results = append(results, Availability{
			Name:              ag.ID(),
			SupportsMCP:       result.SupportsMCP,
			MCPConfigPath:     mcpPath,
			InstallationPaths: result.InstallationPaths,
			Available:         result.Available,
			MatchedPath:       result.MatchedPath,
		})
	}
	return results, nil
}
