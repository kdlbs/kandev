package types

// OpenCodePluginRuntime is the resolved runtime evidence for one OpenCode
// launch. It is resolved by the backend and carried as typed configuration;
// agentctl never infers it from a shell command.
type OpenCodePluginRuntime struct {
	// Family is the resolved OpenCode family ("v1" or "v2").
	Family string `json:"family,omitempty"`
	// Source is the resolved runtime source ("managed" or "native").
	Source string `json:"source,omitempty"`
	// Version is the exact runtime version that will own the process.
	Version string `json:"version,omitempty"`
}
