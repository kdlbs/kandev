// Package catalog adapts the agent-settings domain contracts to the shared
// platform settings catalog.
package catalog

import (
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/settingscatalog"
)

func ProfileDescriptor() settingscatalog.DomainDescriptor {
	fields := make([]settingscatalog.FieldDescriptor, 0, len(dto.ProfileContractFields()))
	for _, contract := range dto.ProfileContractFields() {
		fields = append(fields, settingscatalog.FieldDescriptor{
			Key:            "agent_profile." + contract.Path,
			FieldPath:      contract.Path,
			Label:          contract.Path,
			Description:    contract.Description,
			JSONType:       contract.JSONType,
			Support:        settingscatalog.SupportSupported,
			Classification: settingscatalog.ClassificationWritable,
			Owner:          "agent-settings",
			Writable:       true,
			Validator:      "agent-settings.profile",
			Authority:      "org.config.manage",
			Sensitive:      contract.Sensitive,
			Replacement:    contract.Replacement,
			ChangeTiming:   "future_sessions",
			SettingsHref:   "/settings/agents",
		})
	}
	return settingscatalog.DomainDescriptor{
		Domain:       "profiles",
		ResourceType: "agent_profile",
		Label:        "Agent profiles",
		Description:  "Saved agent profile settings used for future sessions.",
		Owner:        "agent-settings",
		Scope:        settingscatalog.ScopeResource,
		Target:       settingscatalog.TargetRules{Scope: settingscatalog.ScopeResource, RequiresResourceID: true, AllowsWorkspaceID: true},
		SettingsHref: "/settings/agents",
		Operations: []settingscatalog.OperationDescriptor{{
			Name:      "update",
			Binding:   "agent-settings.profile.update",
			Authority: "org.config.manage",
		}},
		Fields: fields,
	}
}

func MCPDescriptor() settingscatalog.DomainDescriptor {
	return settingscatalog.DomainDescriptor{
		Domain:       "profiles",
		ResourceType: "agent_profile_mcp",
		Label:        "Profile MCP documents",
		Description:  "Separate MCP server settings for a saved agent profile.",
		Owner:        "agent-settings",
		Scope:        settingscatalog.ScopeResource,
		Target:       settingscatalog.TargetRules{Scope: settingscatalog.ScopeResource, RequiresResourceID: true},
		SettingsHref: "/settings/agents",
		Operations: []settingscatalog.OperationDescriptor{{
			Name:      "update",
			Binding:   "agent-settings.profile-mcp.update",
			Authority: "org.config.manage",
		}},
		Fields: []settingscatalog.FieldDescriptor{
			{
				Key: "agent_profile_mcp.enabled", FieldPath: "enabled", Label: "MCP enabled",
				Description: "Enable the profile MCP document.", JSONType: "boolean",
				Support: settingscatalog.SupportSupported, Classification: settingscatalog.ClassificationWritable,
				Owner: "agent-settings", Writable: true, Validator: "agent-settings.profile-mcp", Authority: "org.config.manage",
			},
			{
				Key: "agent_profile_mcp.servers", FieldPath: "servers", Label: "MCP servers",
				Description: "Complete replacement map of validated MCP servers.", JSONType: "object",
				Support: settingscatalog.SupportSupported, Classification: settingscatalog.ClassificationWritable,
				Owner: "agent-settings", Writable: true, Validator: "agent-settings.profile-mcp", Authority: "org.config.manage", Sensitive: true, Replacement: true,
			},
			{
				Key: "agent_profile_mcp.meta", FieldPath: "meta", Label: "MCP metadata",
				Description: "Preserved metadata from the MCP document.", JSONType: "object",
				Support: settingscatalog.SupportReadOnly, Classification: settingscatalog.ClassificationComputed,
				Owner: "agent-settings", Sensitive: true,
			},
		},
	}
}

func Descriptors() []settingscatalog.DomainDescriptor {
	return []settingscatalog.DomainDescriptor{ProfileDescriptor(), MCPDescriptor()}
}
