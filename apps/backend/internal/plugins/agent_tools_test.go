package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

func TestAgentToolCatalogUsesConservativeAnnotationDefaults(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "defaults", AgentTools: []manifest.AgentTool{{
				Name: "echo", Description: "Echo input.",
				Surfaces:    []string{manifest.AgentToolSurfaceKanban},
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		Status: store.StatusActive,
	})

	snapshot, err := svc.AgentToolCatalog()
	require.NoError(t, err)
	require.Len(t, snapshot.Tools, 1)
	tool := snapshot.Tools[0]
	require.False(t, tool.ReadOnlyHint)
	require.True(t, tool.DestructiveHint)
	require.False(t, tool.IdempotentHint)
	require.True(t, tool.OpenWorldHint)
}

func TestInvokeAgentToolLogsOnlyBoundMetadata(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	svc := NewService(store.NewFSStore(t.TempDir()), NewRegistry(), nil, log)

	_, err = svc.InvokeAgentTool(context.Background(), "missing", "echo", map[string]any{
		"secret": "do-not-log",
	}, AgentToolInvocationContext{
		InvocationID: "invoke-1", TaskID: "task-1", SessionID: "session-1",
	})
	require.Error(t, err)

	entries := observed.FilterMessage("plugin agent tool invocation").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "invoke-1", fields["invocation_id"])
	require.Equal(t, "missing", fields["plugin_id"])
	require.Equal(t, "echo", fields["local_name"])
	require.Equal(t, "task-1", fields["task_id"])
	require.Equal(t, "session-1", fields["session_id"])
	require.Equal(t, "error", fields["outcome"])
	require.NotContains(t, fields, "arguments")
	require.NotContains(t, fields, "result")
}
