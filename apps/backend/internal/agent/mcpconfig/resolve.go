package mcpconfig

import (
	"fmt"
	"strings"
)

// Resolve converts the stored config into a list of runtime-resolved servers based on policy.
// It returns warnings for skipped servers and a fatal error for invalid configurations.
func Resolve(config *ProfileConfig, policy Policy) ([]ResolvedServer, []string, error) {
	if config == nil || !config.Enabled {
		return nil, nil, nil
	}

	warnings := []string{}
	resolved := make([]ResolvedServer, 0, len(config.Servers))

	for name, server := range config.Servers {
		if !policyAllowsServerName(policy, name) {
			warnings = append(warnings, fmt.Sprintf("mcp server %q skipped: server not allowed by policy", name))
			continue
		}

		serverType := normalizeServerType(server)
		if serverType == "" {
			warnings = append(warnings, fmt.Sprintf("mcp server %q skipped: missing type", name))
			continue
		}

		mode := server.Mode
		if mode == "" || mode == ServerModeAuto {
			if serverType == ServerTypeStdio {
				mode = ServerModePerSession
			} else {
				mode = ServerModeShared
			}
		}

		if serverType == ServerTypeStdio && mode == ServerModeShared {
			return nil, warnings, fmt.Errorf("mcp server %q: shared mode requires HTTP/SSE/streamable HTTP transport (stdio is per-session only)", name)
		}

		if !policyAllows(policy, serverType) {
			warnings = append(warnings, fmt.Sprintf("mcp server %q skipped: transport %q not allowed", name, serverType))
			continue
		}

		if serverType == ServerTypeStdio && strings.TrimSpace(server.Command) == "" {
			warnings = append(warnings, fmt.Sprintf("mcp server %q skipped: stdio server missing command", name))
			continue
		}

		if serverType != ServerTypeStdio && strings.TrimSpace(server.URL) == "" {
			warnings = append(warnings, fmt.Sprintf("mcp server %q skipped: http server missing url", name))
			continue
		}

		env := map[string]string{}
		for k, v := range policy.EnvInjection {
			env[k] = v
		}
		for k, v := range server.Env {
			env[k] = v
		}

		url := server.URL
		if url != "" {
			if rewritten, ok := policy.URLRewrite[url]; ok {
				url = rewritten
			}
		}

		resolved = append(resolved, ResolvedServer{
			Name:    name,
			Type:    serverType,
			Mode:    mode,
			Command: server.Command,
			Args:    append([]string{}, server.Args...),
			Env:     env,
			URL:     url,
			Headers: cloneStringMap(server.Headers),
		})
	}

	return resolved, warnings, nil
}

func normalizeServerType(server ServerDef) ServerType {
	if server.Type != "" {
		return server.Type
	}
	if strings.TrimSpace(server.Command) != "" {
		return ServerTypeStdio
	}
	if strings.TrimSpace(server.URL) != "" {
		return ServerTypeHTTP
	}
	return ""
}

func policyAllows(policy Policy, serverType ServerType) bool {
	switch serverType {
	case ServerTypeStdio:
		return policy.AllowStdio
	case ServerTypeHTTP:
		return policy.AllowHTTP
	case ServerTypeSSE:
		return policy.AllowSSE
	case ServerTypeStreamableHTTP:
		return policy.AllowStreamableHTTP
	default:
		return false
	}
}

func policyAllowsServerName(policy Policy, name string) bool {
	if len(policy.AllowlistServers) > 0 && !containsString(policy.AllowlistServers, name) {
		return false
	}
	if len(policy.DenylistServers) > 0 && containsString(policy.DenylistServers, name) {
		return false
	}
	return true
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
