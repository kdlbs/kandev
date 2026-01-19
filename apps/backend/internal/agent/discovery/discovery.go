// Package discovery provides agent installation detection and discovery functionality.
// It reads agent definitions from the unified agents.json configuration in the registry package.
package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/agent/registry"
)

// OSPaths defines OS-specific paths for discovery (re-exported from registry for API compatibility)
type OSPaths = registry.OSPaths

// Capabilities defines discovery-specific capabilities (re-exported from registry)
type Capabilities = registry.DiscoveryCapabilities

// ModelConfig defines the model configuration for an agent (re-exported from registry)
type ModelConfig = registry.ModelConfig

// ModelEntry defines a single model available for an agent (re-exported from registry)
type ModelEntry = registry.ModelEntry

// ForOS returns paths for the specified operating system
func ForOS(p OSPaths, goos string) []string {
	switch goos {
	case "windows":
		return p.Windows
	case "darwin":
		return p.MacOS
	default:
		return p.Linux
	}
}

// KnownAgent represents an agent definition with discovery metadata
type KnownAgent struct {
	Name             string       `json:"name"`
	DisplayName      string       `json:"display_name"`
	SupportsMCP      bool         `json:"supports_mcp"`
	MCPConfigPath    OSPaths      `json:"mcp_config_path"`
	InstallationPath OSPaths      `json:"installation_path"`
	Capabilities     Capabilities `json:"capabilities"`
	ModelConfig      ModelConfig  `json:"model_config"`
}

// Config is the legacy structure for agent discovery configuration
type Config struct {
	Agents []KnownAgent `json:"agents"`
}

// Availability represents the result of detecting an agent's installation
type Availability struct {
	Name              string   `json:"name"`
	SupportsMCP       bool     `json:"supports_mcp"`
	MCPConfigPath     string   `json:"mcp_config_path,omitempty"`
	InstallationPaths []string `json:"installation_paths,omitempty"`
	Available         bool     `json:"available"`
	MatchedPath       string   `json:"matched_path,omitempty"`
}

// Adapter defines the interface for agent detection strategies
type Adapter interface {
	Detect(ctx context.Context) (Availability, error)
}

// Registry manages agent discovery adapters and definitions
type Registry struct {
	adapters    []Adapter
	definitions []KnownAgent
}

// LoadRegistry creates a new discovery registry by loading agent definitions
// from the unified agents.json configuration in the registry package.
func LoadRegistry() (*Registry, error) {
	agentConfigs, err := registry.GetAgentDefinitions()
	if err != nil {
		return nil, fmt.Errorf("load agent definitions: %w", err)
	}

	definitions := make([]KnownAgent, 0, len(agentConfigs))
	adapters := make([]Adapter, 0, len(agentConfigs))

	for _, agentCfg := range agentConfigs {
		// Convert registry.AgentTypeConfig to discovery.KnownAgent
		knownAgent := KnownAgent{
			Name:             extractAgentName(agentCfg.ID),
			DisplayName:      agentCfg.DisplayName,
			SupportsMCP:      agentCfg.Discovery.SupportsMCP,
			MCPConfigPath:    agentCfg.Discovery.MCPConfigPath,
			InstallationPath: agentCfg.Discovery.InstallationPath,
			Capabilities:     agentCfg.Discovery.DiscoveryCapabilities,
			ModelConfig:      agentCfg.ModelConfig,
		}
		if knownAgent.DisplayName == "" {
			knownAgent.DisplayName = agentCfg.Name
		}
		definitions = append(definitions, knownAgent)
		adapters = append(adapters, NewFilePresenceAdapter(knownAgent))
	}

	return &Registry{adapters: adapters, definitions: definitions}, nil
}

// extractAgentName extracts the short name from an agent ID (e.g., "auggie-agent" -> "auggie")
func extractAgentName(id string) string {
	name := strings.TrimSuffix(id, "-agent")
	return name
}

func (r *Registry) Definitions() []KnownAgent {
	if r == nil {
		return nil
	}
	return append([]KnownAgent(nil), r.definitions...)
}

func (r *Registry) Detect(ctx context.Context) ([]Availability, error) {
	results := make([]Availability, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		availability, err := adapter.Detect(ctx)
		if err != nil {
			return nil, err
		}
		results = append(results, availability)
	}
	return results, nil
}

type FilePresenceAdapter struct {
	definition KnownAgent
}

func NewFilePresenceAdapter(def KnownAgent) *FilePresenceAdapter {
	return &FilePresenceAdapter{definition: def}
}

func (a *FilePresenceAdapter) Detect(ctx context.Context) (Availability, error) {
	_ = ctx
	paths := resolvePaths(ForOS(a.definition.InstallationPath, runtime.GOOS))
	mcpPaths := resolvePaths(ForOS(a.definition.MCPConfigPath, runtime.GOOS))
	available, matched := anyPathExists(paths)
	mcpPath := ""
	if len(mcpPaths) > 0 {
		mcpPath = mcpPaths[0]
	}
	return Availability{
		Name:              a.definition.Name,
		SupportsMCP:       a.definition.SupportsMCP,
		MCPConfigPath:     mcpPath,
		InstallationPaths: paths,
		Available:         available,
		MatchedPath:       matched,
	}, nil
}

func resolvePaths(paths []string) []string {
	resolved := make([]string, 0, len(paths))
	for _, rawPath := range paths {
		expanded := expandPath(rawPath)
		if expanded == "" {
			continue
		}
		resolved = append(resolved, expanded)
	}
	return resolved
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if strings.Contains(path, "$XDG_CONFIG_HOME") || strings.Contains(path, "${XDG_CONFIG_HOME}") {
		if _, ok := os.LookupEnv("XDG_CONFIG_HOME"); !ok {
			return ""
		}
	}
	expanded := os.ExpandEnv(path)
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
		}
	}
	if expanded == "" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(expanded))
}

func anyPathExists(paths []string) (bool, string) {
	for _, candidate := range paths {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return true, candidate
		}
	}
	return false, ""
}
