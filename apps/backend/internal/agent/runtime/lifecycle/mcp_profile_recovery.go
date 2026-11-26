package lifecycle

import mcpprofile "github.com/kandev/kandev/internal/mcp/profile"

func restoreMcpProfileContext(metadata map[string]interface{}) *mcpprofile.Context {
	if metadata == nil {
		return nil
	}
	rawSurface, ok := metadata[mcpprofile.SurfaceMetadataKey].(string)
	if !ok {
		return nil
	}
	surface := mcpprofile.Surface(rawSurface)
	switch surface {
	case mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask, mcpprofile.SurfaceConfiguration,
		mcpprofile.SurfaceExternal, mcpprofile.SurfaceAutomation, mcpprofile.SurfaceManagedConversation,
		mcpprofile.SurfaceCoordinator:
	default:
		return nil
	}

	profile := mcpprofile.New(surface, nil, nil)
	if value, present := metadata[mcpprofile.ManagedToolPolicyMetadataKey]; present {
		policy, err := mcpprofile.ParseManagedToolPolicyMetadata(value)
		if err != nil {
			return nil
		}
		profile.ManagedToolPolicy = policy
	}
	if value, present := metadata[mcpprofile.CoordinatorToolPolicyMetadataKey]; present {
		policy, err := mcpprofile.ParseCoordinatorToolPolicyMetadata(value)
		if err != nil {
			return nil
		}
		profile.CoordinatorToolPolicy = policy
	}
	return &profile
}
