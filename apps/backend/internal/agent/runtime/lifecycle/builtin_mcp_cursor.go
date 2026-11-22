package lifecycle

import (
	"encoding/json"
	"fmt"
	"strings"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func composeEffectiveMCPServers(execution *AgentExecution, client *agentctl.Client, profile []types.McpServer, user map[string]json.RawMessage, imported map[string]types.McpServer, host string) (map[string]any, map[string]string, error) {
	port := passthroughMCPConfigPort(execution)
	coreURL := ""
	if port > 0 {
		if client == nil {
			return nil, nil, fmt.Errorf("agent helper is unavailable for Cursor MCP delivery")
		}
		instanceID, scoped := builtinMCPRouteMode(client)
		var err error
		coreURL, err = agentctlconfig.MCPServerURLForInstance(host, port, instanceID, scoped, builtinCoreMCPPath)
		if err != nil {
			return nil, nil, fmt.Errorf("build Cursor MCP endpoint: %w", err)
		}
	}
	if _, bound := execution.metadataValue(p.BuiltinBindingMetadataKey); !bound {
		servers, ownership := composeLegacyMCPServers(execution, profile, user, imported, coreURL)
		return servers, ownership, nil
	}
	if err := validateBuiltinCursorNames(profile, user, imported); err != nil {
		return nil, nil, err
	}
	native, err := builtinPassthroughMCPServers(execution, client, coreURL, nil)
	if err != nil {
		return nil, nil, err
	}
	servers, ownership := composeLegacyMCPServers(execution, profile, user, imported, coreURL)
	for _, server := range native {
		entry := map[string]any{"url": server.URL}
		servers[server.Name] = entry
		if server.Name != kandevMCPServerName {
			ownership[strings.ToLower(server.Name)], _ = canonicalJSONFingerprint(entry)
		}
	}
	return servers, ownership, nil
}

func validateBuiltinCursorNames(profile []types.McpServer, user map[string]json.RawMessage, imported map[string]types.McpServer) error {
	names := make([]string, 0, len(profile)+len(user)+len(imported)*2)
	for _, server := range profile {
		names = append(names, server.Name)
	}
	for name := range user {
		names = append(names, name)
	}
	for name, server := range imported {
		names = append(names, name, server.Name)
	}
	for _, name := range names {
		if p.ReservedBuiltinRuntimeName(name) {
			return fmt.Errorf("reserved built-in MCP name %q must be renamed", name)
		}
	}
	return nil
}
