package plugins

import (
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

func TestAgentToolCatalogIncludesActiveTools(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	destructive := false
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID:          "task-tags.v1",
			DisplayName: "Task tags",
			AgentTools: []manifest.AgentTool{{
				Name:        "add_tag",
				Description: "Add a tag to a task",
				Surfaces:    []string{manifest.AgentToolSurfaceKanban},
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"tag": map[string]any{"type": "string"}},
					"required":   []any{"tag"},
				},
				Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly, DestructiveHint: &destructive},
			}},
		},
		Status: store.StatusActive,
	})
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "disabled.v1",
			AgentTools: []manifest.AgentTool{{
				Name:        "hidden",
				Description: "Hidden tool",
				Surfaces:    []string{manifest.AgentToolSurfaceKanban},
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		Status: store.StatusDisabled,
	})

	snapshot, err := svc.AgentToolCatalog()
	if err != nil {
		t.Fatalf("AgentToolCatalog() error: %v", err)
	}
	if snapshot.Generation == "" || snapshot.Revision == 0 {
		t.Fatalf("snapshot identity = %#v, want generation and revision", snapshot)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("snapshot tools = %d, want 1", len(snapshot.Tools))
	}
	tool := snapshot.Tools[0]
	if tool.ExposedName != plugintools.ExposedName("task-tags.v1", "add_tag") {
		t.Fatalf("exposed name = %q", tool.ExposedName)
	}
	if !tool.ReadOnlyHint || tool.DestructiveHint {
		t.Fatalf("annotations = %#v, want read-only true and destructive false", tool)
	}
	again, err := svc.AgentToolCatalog()
	if err != nil {
		t.Fatalf("second AgentToolCatalog() error: %v", err)
	}
	if again.Revision != snapshot.Revision {
		t.Fatalf("revision changed without catalog change: %d -> %d", snapshot.Revision, again.Revision)
	}
}
