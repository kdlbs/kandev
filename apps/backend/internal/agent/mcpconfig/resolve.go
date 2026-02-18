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
		rs, warn, err := resolveServer(name, server, policy)
		warnings = append(warnings, warn...)
		if err != nil {
			return nil, warnings, err
		}
		if rs != nil {
			resolved = append(resolved, *rs)
		}
	}

	return resolved, warnings, nil
}

// resolveServer resolves a single server entry against the policy.
// Returns (nil, warnings, nil) when the server should be skipped.
// Returns (nil, warnings, err) on a fatal configuration error.
func resolveServer(name string, server ServerDef, policy Policy) (*ResolvedServer, []string, error) {
	// Skip kandev server - it's automatically injected by agentctl with the correct local URL.
	// This handles existing profiles that still have the old kandev config in the database.
	if name == "kandev" {
		return nil, nil, nil
	}

	if !policyAllowsServerName(policy, name) {
		return nil, []string{fmt.Sprintf("mcp server %q skipped: server not allowed by policy", name)}, nil
	}

	serverType := normalizeServerType(server)
	if serverType == "" {
		return nil, []string{fmt.Sprintf("mcp server %q skipped: missing type", name)}, nil
	}

	mode := resolveServerMode(server.Mode, serverType)

	if serverType == ServerTypeStdio && mode == ServerModeShared {
		return nil, nil, fmt.Errorf("mcp server %q: shared mode requires HTTP/SSE/streamable HTTP transport (stdio is per-session only)", name)
	}

	if !policyAllows(policy, serverType) {
		return nil, []string{fmt.Sprintf("mcp server %q skipped: transport %q not allowed", name, serverType)}, nil
	}

	if warn := validateServerTransport(name, server, serverType); warn != "" {
		return nil, []string{warn}, nil
	}

	rs := buildResolvedServer(name, server, policy, serverType, mode)
	return &rs, nil, nil
}

// resolveServerMode determines the effective mode for a server.
func resolveServerMode(mode ServerMode, serverType ServerType) ServerMode {
	if mode == "" || mode == ServerModeAuto {
		if serverType == ServerTypeStdio {
			return ServerModePerSession
		}
		return ServerModeShared
	}
	return mode
}

// validateServerTransport checks that stdio servers have a command and HTTP servers have a URL.
// Returns a warning string if validation fails, or empty string if valid.
func validateServerTransport(name string, server ServerDef, serverType ServerType) string {
	if serverType == ServerTypeStdio && strings.TrimSpace(server.Command) == "" {
		return fmt.Sprintf("mcp server %q skipped: stdio server missing command", name)
	}
	if serverType != ServerTypeStdio && strings.TrimSpace(server.URL) == "" {
		return fmt.Sprintf("mcp server %q skipped: http server missing url", name)
	}
	return ""
}

// buildResolvedServer constructs a ResolvedServer, merging policy env injection and URL rewriting.
func buildResolvedServer(name string, server ServerDef, policy Policy, serverType ServerType, mode ServerMode) ResolvedServer {
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

	return ResolvedServer{
		Name:    name,
		Type:    serverType,
		Mode:    mode,
		Command: server.Command,
		Args:    append([]string{}, server.Args...),
		Env:     env,
		URL:     url,
		Headers: cloneStringMap(server.Headers),
	}
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
