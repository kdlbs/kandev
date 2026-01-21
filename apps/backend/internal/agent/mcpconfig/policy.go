package mcpconfig

import "github.com/kandev/kandev/internal/agent/runtime"

// DefaultPolicyForRuntime returns the baseline MCP policy for a runtime.
// We only enable local runtimes today; non-local runtimes default to deny-all
// until we add explicit executor policies for their networks.
func DefaultPolicyForRuntime(runtimeName runtime.Name) Policy {
	switch runtimeName {
	case runtime.NameLocal, runtime.NameStandalone, runtime.NameDocker:
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
