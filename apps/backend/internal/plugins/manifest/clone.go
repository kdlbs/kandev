package manifest

// Clone returns an independent copy of the manifest and all nested mutable
// values. Registry callers can mutate the result without changing stored state.
func (m Manifest) Clone() Manifest {
	clone := m
	clone.Categories = cloneStrings(m.Categories)
	clone.Webhooks = cloneWebhooks(m.Webhooks)
	clone.Actions = cloneActions(m.Actions)
	clone.RepositoryProviders = cloneStrings(m.RepositoryProviders)
	clone.ReferenceSources = cloneReferenceSources(m.ReferenceSources)
	clone.AuthProviders = cloneAuthProviders(m.AuthProviders)
	clone.ConfigSchema = cloneMap(m.ConfigSchema)
	clone.AgentTools = cloneAgentTools(m.AgentTools)
	clone.Capabilities.Events = cloneStrings(m.Capabilities.Events)
	clone.Capabilities.APIRead = cloneStrings(m.Capabilities.APIRead)
	clone.Capabilities.APIWrite = cloneStrings(m.Capabilities.APIWrite)
	clone.UI.Pages = append([]UIPage(nil), m.UI.Pages...)
	clone.UI.Styles = cloneStrings(m.UI.Styles)
	clone.UI.Keybindings = append([]UIKeybinding(nil), m.UI.Keybindings...)
	clone.UI.WebApps = cloneWebApps(m.UI.WebApps)
	clone.Runtime.Executables = cloneStringMap(m.Runtime.Executables)
	return clone
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneWebhooks(values []Webhook) []Webhook {
	if values == nil {
		return nil
	}
	return append([]Webhook(nil), values...)
}

func cloneActions(values []Action) []Action {
	if values == nil {
		return nil
	}
	return append([]Action(nil), values...)
}

func cloneReferenceSources(values []ReferenceSource) []ReferenceSource {
	if values == nil {
		return nil
	}
	return append([]ReferenceSource(nil), values...)
}

func cloneAuthProviders(values []AuthProvider) []AuthProvider {
	if values == nil {
		return nil
	}
	return append([]AuthProvider(nil), values...)
}

func cloneWebApps(values []WebApp) []WebApp {
	if values == nil {
		return nil
	}
	clone := make([]WebApp, len(values))
	for index, value := range values {
		clone[index] = value
		clone[index].Placements = cloneStrings(value.Placements)
		clone[index].NetworkOrigins = cloneStrings(value.NetworkOrigins)
	}
	return clone
}

func cloneAgentTools(values []AgentTool) []AgentTool {
	if values == nil {
		return nil
	}
	clone := make([]AgentTool, len(values))
	for index, value := range values {
		clone[index] = value
		clone[index].Surfaces = cloneStrings(value.Surfaces)
		clone[index].InputSchema = cloneMap(value.InputSchema)
		clone[index].OutputSchema = cloneMap(value.OutputSchema)
		clone[index].Annotations.ReadOnlyHint = cloneBool(value.Annotations.ReadOnlyHint)
		clone[index].Annotations.DestructiveHint = cloneBool(value.Annotations.DestructiveHint)
		clone[index].Annotations.IdempotentHint = cloneBool(value.Annotations.IdempotentHint)
		clone[index].Annotations.OpenWorldHint = cloneBool(value.Annotations.OpenWorldHint)
	}
	return clone
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneValue(value)
	}
	return clone
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case map[any]any:
		clone := make(map[any]any, len(typed))
		for key, nested := range typed {
			clone[key] = cloneValue(nested)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for index, nested := range typed {
			clone[index] = cloneValue(nested)
		}
		return clone
	case []string:
		return cloneStrings(typed)
	default:
		return value
	}
}
