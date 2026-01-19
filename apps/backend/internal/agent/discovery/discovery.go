package discovery

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed agents.json
var agentsFS embed.FS

type OSPaths struct {
	Linux   []string `json:"linux"`
	Windows []string `json:"windows"`
	MacOS   []string `json:"macos"`
}

type Capabilities struct {
	SupportsSessionResume bool `json:"supports_session_resume"`
	SupportsShell         bool `json:"supports_shell"`
	SupportsWorkspaceOnly bool `json:"supports_workspace_only"`
}

func (p OSPaths) ForOS(goos string) []string {
	switch goos {
	case "windows":
		return p.Windows
	case "darwin":
		return p.MacOS
	default:
		return p.Linux
	}
}

type KnownAgent struct {
	Name             string       `json:"name"`
	DisplayName      string       `json:"display_name"`
	SupportsMCP      bool         `json:"supports_mcp"`
	MCPConfigPath    OSPaths      `json:"mcp_config_path"`
	InstallationPath OSPaths      `json:"installation_path"`
	Capabilities     Capabilities `json:"capabilities"`
}

type Config struct {
	Agents []KnownAgent `json:"agents"`
}

type Availability struct {
	Name              string   `json:"name"`
	SupportsMCP       bool     `json:"supports_mcp"`
	MCPConfigPath     string   `json:"mcp_config_path,omitempty"`
	InstallationPaths []string `json:"installation_paths,omitempty"`
	Available         bool     `json:"available"`
	MatchedPath       string   `json:"matched_path,omitempty"`
}

type Adapter interface {
	Detect(ctx context.Context) (Availability, error)
}

type Registry struct {
	adapters    []Adapter
	definitions []KnownAgent
}

func LoadRegistry() (*Registry, error) {
	data, err := agentsFS.ReadFile("agents.json")
	if err != nil {
		return nil, fmt.Errorf("read agents config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse agents config: %w", err)
	}
	adapters := make([]Adapter, 0, len(cfg.Agents))
	for _, agent := range cfg.Agents {
		adapters = append(adapters, NewFilePresenceAdapter(agent))
	}
	return &Registry{adapters: adapters, definitions: cfg.Agents}, nil
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
	paths := resolvePaths(a.definition.InstallationPath.ForOS(runtime.GOOS))
	mcpPaths := resolvePaths(a.definition.MCPConfigPath.ForOS(runtime.GOOS))
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
