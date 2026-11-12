package config

import (
	"errors"
	"fmt"
	"slices"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

func injectInstanceBuiltinMCP(cfg *InstanceConfig) ([]McpServerConfig, error) {
	if cfg.McpProfile == nil || cfg.McpProfile.BuiltinPackages == nil {
		return injectKandevMcpServerAtHost(cfg.McpServers, cfg.Port, cfg.MCPHost, cfg.BuiltinMCPCredential), nil
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
	servers, err := BuiltinMCPServers(manifest, cfg.MCPHost, cfg.Port, cfg.BuiltinMCPCredential)
	if err != nil {
		return nil, err
	}
	return append(servers, cfg.McpServers...), nil
}

// BuiltinMCPServers materializes only the backend-bound built-in package views.
// Each name offers HTTP before SSE so existing capability filtering keeps HTTP
// when a provider supports both transports. A non-empty credential is attached
// to every entry as the instance's caller credential.
func BuiltinMCPServers(manifest *p.BuiltinManifest, host string, port int, credential string) ([]McpServerConfig, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	if manifest == nil {
		return injectKandevMcpServerAtHost(nil, port, host, credential), nil
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
		servers = append(servers,
			McpServerConfig{Name: descriptor.RuntimeName, Type: "http", URL: MCPServerURL(host, port, "/mcp"+suffix), Headers: BuiltinMCPHeaders(credential)},
			McpServerConfig{Name: descriptor.RuntimeName, Type: "sse", URL: MCPServerURL(host, port, "/sse"+suffix), Headers: BuiltinMCPHeaders(credential)},
		)
	}
	return servers, nil
}
