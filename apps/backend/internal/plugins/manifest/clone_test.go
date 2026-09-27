package manifest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManifestCloneDoesNotAliasNestedValues(t *testing.T) {
	readOnly := true
	manifest := Manifest{
		Categories:          []string{"tools"},
		RepositoryProviders: []string{"github"},
		ExecutorProviders: []ExecutorProvider{{
			Key: "microvm", SupportedStateVersions: []int{1},
			ProfileSchema:     map[string]any{"properties": map[string]any{"region": map[string]any{"type": "string"}}},
			LocalizedMessages: map[string]string{"expired": "provider.environment.expired"},
		}},
		ConfigSchema: map[string]any{
			"nested": map[string]any{"values": []any{"one", map[string]any{"enabled": true}}},
		},
		AgentTools: []AgentTool{{
			Surfaces:    []string{"kanban-task"},
			InputSchema: map[string]any{"type": "object"},
			Annotations: AgentToolAnnotations{ReadOnlyHint: &readOnly},
		}},
		UI:      UISection{WebApps: []WebApp{{Placements: []string{"task-canvas"}}}},
		Runtime: Runtime{Executables: map[string]string{"linux-amd64": "server/plugin"}},
	}

	clone := manifest.Clone()
	clone.Categories[0] = "changed"
	clone.ExecutorProviders[0].SupportedStateVersions[0] = 2
	clone.ExecutorProviders[0].ProfileSchema["properties"].(map[string]any)["region"].(map[string]any)["type"] = "boolean"
	clone.ExecutorProviders[0].LocalizedMessages["expired"] = "changed"
	clone.ConfigSchema["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["enabled"] = false
	clone.AgentTools[0].Surfaces[0] = "office-task"
	*clone.AgentTools[0].Annotations.ReadOnlyHint = false
	clone.UI.WebApps[0].Placements[0] = "workspace-canvas"
	clone.Runtime.Executables["linux-amd64"] = "changed"

	require.Equal(t, "tools", manifest.Categories[0])
	require.Equal(t, 1, manifest.ExecutorProviders[0].SupportedStateVersions[0])
	require.Equal(t, "string", manifest.ExecutorProviders[0].ProfileSchema["properties"].(map[string]any)["region"].(map[string]any)["type"])
	require.Equal(t, "provider.environment.expired", manifest.ExecutorProviders[0].LocalizedMessages["expired"])
	require.Equal(t, true, manifest.ConfigSchema["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["enabled"])
	require.Equal(t, "kanban-task", manifest.AgentTools[0].Surfaces[0])
	require.True(t, *manifest.AgentTools[0].Annotations.ReadOnlyHint)
	require.Equal(t, "task-canvas", manifest.UI.WebApps[0].Placements[0])
	require.Equal(t, "server/plugin", manifest.Runtime.Executables["linux-amd64"])
}
