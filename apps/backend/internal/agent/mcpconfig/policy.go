package mcpconfig

import "github.com/kandev/kandev/internal/agent/executor"

// DefaultPolicyForRuntime returns the baseline MCP policy for a runtime.
// We only enable local runtimes today; non-local runtimes default to deny-all
// until we add explicit executor policies for their networks.
func DefaultPolicyForRuntime(runtimeName executor.Name) Policy {
	switch runtimeName {
	case executor.NameLocal, executor.NameStandalone, executor.NameDocker:
		return Policy{
			AllowStdio:          true,
			AllowHTTP:           true,
			AllowSSE:            true,
			AllowStreamableHTTP: true,
			URLRewrite:          map[string]string{},
			EnvInjection:        map[string]string{},
			AllowlistServers:    nil,
			DenylistServers:     nil,
		}
	default:
		return Policy{
			AllowStdio:          false,
			AllowHTTP:           false,
			AllowSSE:            false,
			AllowStreamableHTTP: false,
			URLRewrite:          map[string]string{},
			EnvInjection:        map[string]string{},
			AllowlistServers:    nil,
			DenylistServers:     nil,
		}
	}
}
