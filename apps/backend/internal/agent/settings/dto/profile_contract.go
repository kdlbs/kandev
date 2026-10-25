package dto

import (
	"fmt"
	"strings"
)

// ProfileCreateRequest is the shared wire contract for HTTP and MCP profile
// creation. Value fields retain the existing create defaults; omitted optional
// fields are left at their domain defaults.
type ProfileCreateRequest struct {
	AgentID                 string                  `json:"agent_id"`
	Name                    string                  `json:"name"`
	Model                   string                  `json:"model,omitempty"`
	FallbackModel           string                  `json:"fallback_model,omitempty"`
	AutoFallback            bool                    `json:"auto_fallback,omitempty"`
	RequireExactModel       bool                    `json:"require_exact_model,omitempty"`
	Mode                    string                  `json:"mode,omitempty"`
	ConfigOptions           map[string]string       `json:"config_options,omitempty"`
	AllowIndexing           bool                    `json:"allow_indexing,omitempty"`
	AutoApprove             bool                    `json:"auto_approve,omitempty"`
	CLIPassthrough          bool                    `json:"cli_passthrough,omitempty"`
	CLIFlags                []CLIFlagDTO            `json:"cli_flags,omitempty"`
	EnvVars                 []ProfileEnvVarDTO      `json:"env_vars,omitempty"`
	CommandPrefix           string                  `json:"command_prefix,omitempty"`
	ProviderKind            string                  `json:"provider_kind,omitempty"`
	ProviderBaseURL         string                  `json:"provider_base_url,omitempty"`
	ProviderAPIKeySecretID  string                  `json:"provider_api_key_secret_id,omitempty"`
	CursorMCPAuthEnabled    *bool                   `json:"cursor_mcp_auth_enabled,omitempty"`
	CursorPluginsMCPEnabled *bool                   `json:"cursor_plugins_mcp_enabled,omitempty"`
	MCPSelectionMode        *string                 `json:"mcp_selection_mode,omitempty"`
	MCPSelectedServers      *[]string               `json:"mcp_selected_servers,omitempty"`
	Dynamic                 *DynamicAgentProfileDTO `json:"dynamic,omitempty"`
}

// ProfileUpdateRequest is the shared partial wire contract. A nil pointer
// means that the caller omitted the field. A non-nil pointer preserves an
// explicit false, empty string, empty map, or empty list.
type ProfileUpdateRequest struct {
	ID                string              `json:"id"`
	Name              *string             `json:"name,omitempty"`
	Model             *string             `json:"model,omitempty"`
	FallbackModel     *string             `json:"fallback_model,omitempty"`
	AutoFallback      *bool               `json:"auto_fallback,omitempty"`
	RequireExactModel *bool               `json:"require_exact_model,omitempty"`
	Mode              *string             `json:"mode,omitempty"`
	ConfigOptions     *map[string]string  `json:"config_options,omitempty"`
	AllowIndexing     *bool               `json:"allow_indexing,omitempty"`
	AutoApprove       *bool               `json:"auto_approve,omitempty"`
	CLIPassthrough    *bool               `json:"cli_passthrough,omitempty"`
	Enabled           *bool               `json:"enabled,omitempty"`
	CLIFlags          *[]CLIFlagDTO       `json:"cli_flags,omitempty"`
	EnvVars           *[]ProfileEnvVarDTO `json:"env_vars,omitempty"`
	CommandPrefix     *string             `json:"command_prefix,omitempty"`
	// Provider* replace their value when non-nil. The settings editor always
	// sends the full triple on save, so a switch back to Native persists.
	ProviderKind            *string                 `json:"provider_kind,omitempty"`
	ProviderBaseURL         *string                 `json:"provider_base_url,omitempty"`
	ProviderAPIKeySecretID  *string                 `json:"provider_api_key_secret_id,omitempty"`
	CursorMCPAuthEnabled    *bool                   `json:"cursor_mcp_auth_enabled,omitempty"`
	CursorPluginsMCPEnabled *bool                   `json:"cursor_plugins_mcp_enabled,omitempty"`
	MCPSelectionMode        *string                 `json:"mcp_selection_mode,omitempty"`
	MCPSelectedServers      *[]string               `json:"mcp_selected_servers,omitempty"`
	Dynamic                 *DynamicAgentProfileDTO `json:"dynamic,omitempty"`
	Force                   bool                    `json:"force,omitempty"`
}

func (r ProfileCreateRequest) Validate() error {
	if strings.TrimSpace(r.AgentID) == "" {
		return fmt.Errorf("agent_id is required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return ValidateMCPSelection(r.MCPSelectionMode, r.MCPSelectedServers)
}

func (r ProfileUpdateRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("id is required")
	}
	return ValidateMCPSelection(r.MCPSelectionMode, r.MCPSelectedServers)
}

const (
	MCPSelectionModeInherit  = "inherit"
	MCPSelectionModeSelected = "selected"
	maxMCPSelectedServers    = 256
	maxMCPServerIDLength     = 512
)

func ValidateMCPSelection(mode *string, servers *[]string) error {
	if mode != nil && *mode != MCPSelectionModeInherit && *mode != MCPSelectionModeSelected {
		return fmt.Errorf("mcp_selection_mode must be inherit or selected")
	}
	if servers == nil {
		return nil
	}
	if len(*servers) > maxMCPSelectedServers {
		return fmt.Errorf("mcp_selected_servers cannot contain more than %d identifiers", maxMCPSelectedServers)
	}
	seen := make(map[string]struct{}, len(*servers))
	for index, serverID := range *servers {
		if strings.TrimSpace(serverID) == "" || len(serverID) > maxMCPServerIDLength {
			return fmt.Errorf("mcp_selected_servers[%d] must be a non-empty identifier of at most %d bytes", index, maxMCPServerIDLength)
		}
		if _, exists := seen[serverID]; exists {
			return fmt.Errorf("mcp_selected_servers contains duplicate identifier %q", serverID)
		}
		seen[serverID] = struct{}{}
	}
	return nil
}

// ProfileContractField describes one profile setting without exposing the
// controller's internal request type to schema consumers.
type ProfileContractField struct {
	Path        string `json:"path"`
	JSONType    string `json:"json_type"`
	Support     string `json:"support"`
	Description string `json:"description"`
	Sensitive   bool   `json:"sensitive,omitempty"`
	Replacement bool   `json:"replacement,omitempty"`
}

// ProfileContractFields is intentionally explicit. It is an independent
// contract root used by schema export and coverage checks.
func ProfileContractFields() []ProfileContractField {
	return []ProfileContractField{
		{Path: "name", JSONType: "string", Support: "read_write", Description: "Profile display name."},
		{Path: "model", JSONType: "string", Support: "read_write", Description: "Preferred model; empty uses the agent default."},
		{Path: "fallback_model", JSONType: "string", Support: "read_write", Description: "Optional fallback model."},
		{Path: "auto_fallback", JSONType: "boolean", Support: "read_write", Description: "Enable automatic fallback behavior."},
		{Path: "require_exact_model", JSONType: "boolean", Support: "read_write", Description: "Require the configured model to be advertised and applied."},
		{Path: "mode", JSONType: "string", Support: "read_write", Description: "Agent operating mode."},
		{Path: "config_options", JSONType: "object", Support: "read_write", Description: "Typed provider configuration options.", Replacement: true},
		{Path: "allow_indexing", JSONType: "boolean", Support: "compatibility", Description: "Legacy indexing permission field."},
		{Path: "auto_approve", JSONType: "boolean", Support: "read_write", Description: "Automatically approve agent permissions."},
		{Path: "cli_passthrough", JSONType: "boolean", Support: "read_write", Description: "Allow CLI passthrough mode."},
		{Path: "enabled", JSONType: "boolean", Support: "read_write", Description: "Allow this profile for new work."},
		{Path: "cli_flags", JSONType: "array", Support: "read_write", Description: "Complete replacement list of CLI flags.", Replacement: true},
		{Path: "env_vars", JSONType: "array", Support: "read_write", Description: "Complete replacement list of environment variables.", Replacement: true, Sensitive: true},
		{Path: "command_prefix", JSONType: "string", Support: "read_write", Description: "Optional launcher command prefix."},
		{Path: "provider_kind", JSONType: "string", Support: "read_write", Description: "Provider routing kind."},
		{Path: "provider_base_url", JSONType: "string", Support: "read_write", Description: "OpenAI-compatible provider base URL."},
		{Path: "provider_api_key_secret_id", JSONType: "string", Support: "read_write", Description: "Global secret reference for the provider API key.", Sensitive: true},
		{Path: "cursor_mcp_auth_enabled", JSONType: "boolean", Support: "read_write", Description: "Share local Cursor MCP credentials. Defaults to true for new profiles."},
		{Path: "cursor_plugins_mcp_enabled", JSONType: "boolean", Support: "read_write", Description: "Import local Cursor plugin MCP servers. Defaults to true for new profiles."},
		{Path: "mcp_selection_mode", JSONType: "string", Support: "read_write", Description: "Choose all enabled imported MCP servers or only selected native server identities."},
		{Path: "mcp_selected_servers", JSONType: "array", Support: "read_write", Description: "Exact native MCP server identities selected for automatic preparation.", Replacement: true},
		{Path: "dynamic", JSONType: "object", Support: "read_write", Description: "Versioned dynamic routing document.", Replacement: true},
	}
}
