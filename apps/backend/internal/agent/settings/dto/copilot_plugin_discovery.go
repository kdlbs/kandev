package dto

// CopilotPluginDiscoveryRequest asks the backend host to inventory the
// selected Copilot runtime for one saved profile or one complete draft launch
// snapshot. Requests cannot supply directories, executables, or native state.
type CopilotPluginDiscoveryRequest struct {
	ProfileID          string                        `json:"profile_id,omitempty"`
	LaunchSettings     *ProfileLaunchSettingsRequest `json:"launch_settings,omitempty"`
	Refresh            bool                          `json:"refresh,omitempty"`
	AuthorizationScope string                        `json:"-"`
}

// CopilotPluginDiscoveryPlugin is one safe inventory row. Availability and
// reason codes are bounded; roots, raw output, and native documents never
// cross this boundary.
type CopilotPluginDiscoveryPlugin struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Marketplace   string `json:"marketplace,omitempty"`
	Version       string `json:"version,omitempty"`
	NativeEnabled bool   `json:"native_enabled"`
	Availability  string `json:"availability"`
	ReasonCode    string `json:"reason_code,omitempty"`
}

// CopilotPluginDiscoveryResponse is the complete discovery snapshot.
type CopilotPluginDiscoveryResponse struct {
	AgentID         string `json:"agent_id"`
	ProfileID       string `json:"profile_id"`
	ProviderID      string `json:"provider_id"`
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
	ContextRevision string `json:"context_revision,omitempty"`
	RuntimeVersion  string `json:"runtime_version,omitempty"`
	// MountPermitted reports whether a nonempty selection can start a new
	// local/worktree process on the qualified runtime.
	MountPermitted bool                           `json:"mount_permitted"`
	Plugins        []CopilotPluginDiscoveryPlugin `json:"plugins"`
}
