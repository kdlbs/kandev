package mcpconfig

// DefaultPolicyForRuntime returns a permissive policy for the current runtime.
// This can be tightened per executor type later.
func DefaultPolicyForRuntime(runtimeName string) Policy {
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
}
