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
	clone.ConfigSchema["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["enabled"] = false
	clone.AgentTools[0].Surfaces[0] = "office-task"
	*clone.AgentTools[0].Annotations.ReadOnlyHint = false
	clone.UI.WebApps[0].Placements[0] = "workspace-canvas"
	clone.Runtime.Executables["linux-amd64"] = "changed"

	require.Equal(t, "tools", manifest.Categories[0])
	require.Equal(t, true, manifest.ConfigSchema["nested"].(map[string]any)["values"].([]any)[1].(map[string]any)["enabled"])
	require.Equal(t, "kanban-task", manifest.AgentTools[0].Surfaces[0])
	require.True(t, *manifest.AgentTools[0].Annotations.ReadOnlyHint)
	require.Equal(t, "task-canvas", manifest.UI.WebApps[0].Placements[0])
	require.Equal(t, "server/plugin", manifest.Runtime.Executables["linux-amd64"])
}
