package config

import (
	"errors"
	"fmt"
	"slices"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

func injectInstanceBuiltinMCP(cfg *InstanceConfig) ([]McpServerConfig, error) {
	if cfg.McpProfile == nil || cfg.McpProfile.BuiltinPackages == nil {
		return injectKandevMcpServerForInstance(cfg.McpServers, cfg.Port, cfg.MCPHost, cfg.InstanceID, cfg.ScopedMCPRoutes)
	}
	manifest := cfg.McpProfile.BuiltinPackages
	if manifest.Surface != cfg.McpProfile.Surface || manifest.TaskID != cfg.TaskID {
		return nil, errors.New("built-in MCP manifest does not match the instance")
	}
	for _, server := range cfg.McpServers {
		if p.ReservedBuiltinRuntimeName(server.Name) {
			return nil, fmt.Errorf("reserved built-in MCP name %q must be renamed", server.Name)
		}
	}
	servers, err := BuiltinMCPServersForInstance(manifest, cfg.MCPHost, cfg.Port, cfg.InstanceID, cfg.ScopedMCPRoutes)
	if err != nil {
		return nil, err
	}
	return append(servers, cfg.McpServers...), nil
}

// BuiltinMCPServers materializes only the backend-bound built-in package views.
// Each name offers HTTP before SSE so existing capability filtering keeps HTTP
// when a provider supports both transports.
func BuiltinMCPServers(manifest *p.BuiltinManifest, host string, port int) ([]McpServerConfig, error) {
	return BuiltinMCPServersForInstance(manifest, host, port, "", false)
}

// BuiltinMCPServersForInstance materializes native endpoints for the helper's
// accepted instance identity and route mode.
func BuiltinMCPServersForInstance(manifest *p.BuiltinManifest, host string, port int, instanceID string, scoped bool) ([]McpServerConfig, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if manifest == nil {
		return injectKandevMcpServerForInstance(nil, port, host, instanceID, scoped)
	}
	var servers []McpServerConfig
	for _, descriptor := range p.BuiltinPackageCatalog() {
		if !slices.Contains(manifest.Packages, descriptor.ID) {
			continue
		}
		suffix := ""
		if descriptor.ID != p.PackageCore {
			suffix = "/packages/" + string(descriptor.ID)
		}
		httpURL, err := MCPServerURLForInstance(host, port, instanceID, scoped, "/mcp"+suffix)
		if err != nil {
			return nil, err
		}
		sseURL, err := MCPServerURLForInstance(host, port, instanceID, scoped, "/sse"+suffix)
		if err != nil {
			return nil, err
		}
		servers = append(servers, McpServerConfig{Name: descriptor.RuntimeName, Type: "http", URL: httpURL}, McpServerConfig{Name: descriptor.RuntimeName, Type: "sse", URL: sseURL})
	}
	return servers, nil
}
