package plugins

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/stretchr/testify/require"
)

func automationToolRecord() *store.Record {
	readOnly := true
	return &store.Record{
		Manifest: manifest.Manifest{
			ID:          "kandev-plugin-automation",
			DisplayName: "Automation fixture",
			Version:     "1.0.0",
			ConfigSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"token": map[string]any{"type": "string"}},
			},
			AgentTools: []manifest.AgentTool{
				{
					Name: "list_findings", Description: "List findings",
					Surfaces:    []string{manifest.AgentToolSurfaceAutomation},
					InputSchema: map[string]any{"type": "object"},
					Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
				},
				{
					Name: "complete_report", Description: "Complete a report",
					Surfaces:    []string{manifest.AgentToolSurfaceAutomation},
					InputSchema: map[string]any{"type": "object"},
				},
				{
					Name: "ordinary_task_tool", Description: "Not automation",
					Surfaces:    []string{manifest.AgentToolSurfaceKanban},
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
		Status:                 StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	}
}

func TestAgentToolPolicyVersionLifecycle(t *testing.T) {
	svc, _, fsStore, _ := newTestServiceWithDir(t)
	record := automationToolRecord()
	svc.registry.Add(record)
	require.NoError(t, fsStore.Save(record))

	options, err := svc.PluginToolOptions(context.Background(), "workspace-1")
	require.NoError(t, err)
	require.Len(t, options, 1)
	require.Equal(t, "policy-1", options[0].PolicyVersion)
	require.Len(t, options[0].Tools, 2, "only automation declarations are selectable")
	require.True(t, options[0].Tools[0].ReadOnlyHint)

	require.NoError(t, svc.UpdateConfig(context.Background(), record.ID, map[string]any{"token": "secret"}))
	updated, err := svc.Get(record.ID)
	require.NoError(t, err)
	require.NotEqual(t, "policy-1", updated.AgentToolPolicyVersion)
	require.NotEmpty(t, updated.AgentToolPolicyVersion)

	// A restart keeps the durable version; Load must not rotate a non-empty
	// value or admitted configuration changes would be forgotten.
	reloaded := NewRegistry()
	require.NoError(t, reloaded.Load(fsStore))
	persisted, ok := reloaded.Get(record.ID)
	require.True(t, ok)
	require.Equal(t, updated.AgentToolPolicyVersion, persisted.AgentToolPolicyVersion)
}

func TestAutomationAgentToolInvocationRequiresExactCurrentApproval(t *testing.T) {
	svc, _, fsStore, _ := newTestServiceWithDir(t)
	record := automationToolRecord()
	svc.registry.Add(record)
	require.NoError(t, fsStore.Save(record))

	base := AgentToolInvocationContext{
		InvocationID: "invocation-1", TaskID: "task-1", SessionID: "session-1",
		WorkspaceID: "workspace-1", Surface: manifest.AgentToolSurfaceAutomation, ExecutionID: "execution-1",
	}
	authorization := &plugintools.AutomationAuthorization{
		AutomationID: "automation-1", WorkspaceID: "workspace-1", PluginID: record.ID,
		InstallationID: record.InstallationID, PolicyVersion: record.AgentToolPolicyVersion,
		ToolNames: []string{"list_findings"},
	}

	_, err := svc.InvokeAgentTool(context.Background(), record.ID, "list_findings", map[string]any{}, base)
	require.ErrorContains(t, err, "approval denied")

	stale := *authorization
	stale.PolicyVersion = "old-policy"
	withStale := base
	withStale.AutomationToolAuthorization = &stale
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "list_findings", map[string]any{}, withStale)
	require.ErrorContains(t, err, "stale")

	unselected := base
	unselected.AutomationToolAuthorization = authorization
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "complete_report", map[string]any{}, unselected)
	require.ErrorContains(t, err, "approval denied")

	forgedWorkspace := base
	forgedWorkspace.WorkspaceID = "workspace-other"
	forgedWorkspace.AutomationToolAuthorization = authorization
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "list_findings", map[string]any{}, forgedWorkspace)
	require.ErrorContains(t, err, "approval denied")

	// The exact current approval reaches the normal dispatch boundary.
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "list_findings", map[string]any{}, unselected)
	require.ErrorContains(t, err, "is not running")
}
