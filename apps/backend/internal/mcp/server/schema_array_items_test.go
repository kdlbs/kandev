package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
)

// TestRegisteredToolSchemasDeclareArrayItems guards strict providers (Gemini
// rejects a request whose tool schema has an array parameter without items)
// across every Kandev-owned MCP surface.
func TestRegisteredToolSchemasDeclareArrayItems(t *testing.T) {
	log := newTestLogger(t)
	newModeServer := func(mode string) *Server {
		return New(&testBackend{}, "session", "task", 10005, log, "", false, mode)
	}
	newProfileServer := func(profile mcpprofile.Context) *Server {
		return NewWithProfile(&testBackend{}, "session", "task", 10005, log, "", false, profile)
	}
	// The managed-conversation surface registers no context tools by design;
	// its catalog is the plugin tools admitted by the managed policy.
	newManagedConversationServer := func(t *testing.T) *Server {
		t.Helper()
		policy := mcpprofile.ManagedToolPolicy{
			PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
			InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
			ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			AgentToolNames: []string{"read_task"},
		}
		profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
		s := NewWithProfile(&testBackend{}, "session-1", "task-1", 10005, log, "", false, profile)
		require.NoError(t, s.SetPluginTools(plugintools.Snapshot{Generation: "managed", Revision: 1, Tools: []plugintools.Definition{{
			PluginID: "coordinator.v1", LocalName: "read_task",
			ExposedName: plugintools.ExposedName("coordinator.v1", "read_task"),
			Description: "Read a task",
			InputSchema: []byte(`{"type":"object","properties":{"labels":{"type":"array","items":{"type":"string"}}}}`),
			Surfaces:    []string{plugintools.SurfaceManaged},
		}}}))
		return s
	}

	surfaces := map[string]*Server{}
	surfaces["task"] = newModeServer(ModeTask)
	surfaces["config"] = newModeServer(ModeConfig)
	surfaces["external"] = newModeServer(ModeExternal)
	surfaces["office"] = newModeServer(ModeOffice)
	surfaces["automation"] = newModeServer(ModeAutomation)
	surfaces["task_title_pending"] = newModeServer(ModeTaskTitlePending)
	surfaces["external_endpoint"] = NewExternal(&testBackend{}, log, "")
	surfaces["profile_automation"] = newProfileServer(mcpprofile.NewAutomation())
	surfaces["profile_coordinator"] = newProfileServer(mcpprofile.NewCoordinator())
	surfaces["profile_managed_conversation"] = newManagedConversationServer(t)

	packageProfile := NewWithProfile(
		&testBackend{}, "session", "task", 10005, log, "", false,
		packageTestProfile(mcpprofile.PackageCore, mcpprofile.PackageConfiguration),
	)
	t.Cleanup(func() { require.NoError(t, packageProfile.Close(context.Background())) })
	configurationPackage := packageProfile.builtinPackages[mcpprofile.PackageConfiguration]
	require.NotNil(t, configurationPackage)
	require.Contains(t, configurationPackage.registry.ListTools(), "reorder_workflow_steps_kandev")
	surfaces["package_manifest"] = packageProfile

	// The default profile of each surface leaves out tools gated by a
	// capability, provider, or coordinator binding. Register those groups too so
	// every Kandev-owned tool is guarded.
	allCapabilities := []mcpprofile.Capability{
		mcpprofile.CapabilityUserQuestion,
		mcpprofile.CapabilityParentQuestion,
		mcpprofile.CapabilityTaskTitle,
		mcpprofile.CapabilityGitHubPR,
		mcpprofile.CapabilityGitLabMR,
		mcpprofile.CapabilityCanvas,
		mcpprofile.CapabilityWorkspaceCollaborationReport,
	}
	allProviders := []string{"github", "gitlab"}
	for name, surface := range map[string]mcpprofile.Surface{
		"full_kanban":     mcpprofile.SurfaceKanbanTask,
		"full_office":     mcpprofile.SurfaceOfficeTask,
		"full_config":     mcpprofile.SurfaceConfiguration,
		"full_external":   mcpprofile.SurfaceExternal,
		"full_automation": mcpprofile.SurfaceAutomation,
	} {
		surfaces[name] = newProfileServer(mcpprofile.New(surface, allCapabilities, allProviders))
	}
	coordinatorProfile := mcpprofile.NewCoordinator()
	coordinatorProfile.CoordinatorToolPolicy = &mcpprofile.CoordinatorToolPolicy{
		Version:            mcpprofile.CoordinatorToolPolicyVersionV1,
		CoordinatorID:      "coordinator-1",
		WorkspaceID:        "workspace-1",
		ConversationTaskID: "task-1",
		ToolNames:          mcpprofile.CoordinatorToolUniverse(),
	}
	surfaces["full_coordinator"] = newProfileServer(coordinatorProfile)

	registered := make(map[string]struct{})
	for name, server := range surfaces {
		t.Run(name, func(t *testing.T) {
			assertRegistryArraySchemasDeclareItems(t, server.mcpServer, registered)
			for packageID, packageServer := range server.builtinPackages {
				t.Run(string(packageID), func(t *testing.T) {
					assertRegistryArraySchemasDeclareItems(t, packageServer.registry, registered)
				})
			}
		})
	}

	// A surface that registered nothing, or a profile that silently dropped a
	// capability group, must not pass. These names cover the capability,
	// provider, and coordinator-bound tools the default profiles omit.
	expected := []string{
		"list_canvases_kandev",
		"read_canvas_authoring_skill_kandev",
		"create_canvas_kandev",
		"get_canvas_kandev",
		"publish_canvas_kandev",
		"get_canvas_state_kandev",
		"set_canvas_state_kandev",
		"ask_parent_question_kandev",
		"get_task_change_requests_kandev",
		"manage_task_change_request_kandev",
		"report_change_request_auto_fix_outcome_kandev",
		"update_task_change_request_automation_kandev",
		"list_coordinator_activity_kandev",
		"list_coordinator_turns_kandev",
		"propose_message_kandev",
		"propose_move_kandev",
		"propose_resume_kandev",
		"propose_improvement_kandev",
	}
	for _, name := range expected {
		require.Contains(t, registered, name, "tool %q was never registered", name)
	}

	// The guard must actually detect a missing items declaration, otherwise a
	// surface that registers no tools (or a broken walk) would pass silently.
	t.Run("synthetic schema without items is detected", func(t *testing.T) {
		missing := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"labels": map[string]any{"type": "array"},
			},
		}
		require.NotEmpty(t, arraySchemasMissingItems("synthetic", "inputSchema", missing))
		require.Empty(t, arraySchemasMissingItems("synthetic", "inputSchema", map[string]any{
			"type":  "object",
			"items": map[string]any{"type": "string"},
		}))
		require.Empty(t, arraySchemasMissingItems("synthetic", "inputSchema", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":    "object",
					"default": map[string]any{"type": "array"},
				},
			},
		}))
		t.Run("schema-bearing keywords are traversed", func(t *testing.T) {
			arraySchema := func() map[string]any { return map[string]any{"type": "array"} }
			cases := []struct {
				name  string
				key   string
				value any
			}{
				{name: "properties", key: "properties", value: map[string]any{"value": arraySchema()}},
				{name: "patternProperties", key: "patternProperties", value: map[string]any{"^value$": arraySchema()}},
				{name: "definitions", key: "definitions", value: map[string]any{"value": arraySchema()}},
				{name: "dollar definitions", key: "$defs", value: map[string]any{"value": arraySchema()}},
				{name: "dependent schemas", key: "dependentSchemas", value: map[string]any{"value": arraySchema()}},
				{name: "dependencies", key: "dependencies", value: map[string]any{"value": arraySchema()}},
				{name: "items", key: "items", value: arraySchema()},
				{name: "tuple items", key: "items", value: []any{arraySchema()}},
				{name: "prefix items", key: "prefixItems", value: []any{arraySchema()}},
				{name: "additional items", key: "additionalItems", value: arraySchema()},
				{name: "additional properties", key: "additionalProperties", value: arraySchema()},
				{name: "contains", key: "contains", value: arraySchema()},
				{name: "property names", key: "propertyNames", value: arraySchema()},
				{name: "not", key: "not", value: arraySchema()},
				{name: "if", key: "if", value: arraySchema()},
				{name: "then", key: "then", value: arraySchema()},
				{name: "else", key: "else", value: arraySchema()},
				{name: "allOf", key: "allOf", value: []any{arraySchema()}},
				{name: "anyOf", key: "anyOf", value: []any{arraySchema()}},
				{name: "oneOf", key: "oneOf", value: []any{arraySchema()}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					missing := arraySchemasMissingItems("synthetic", "inputSchema", map[string]any{
						"type": "object",
						tc.key: tc.value,
					})
					require.Len(t, missing, 1)
					require.Contains(t, missing[0], "inputSchema."+tc.key)
				})
			}
		})
	})
}

func assertRegistryArraySchemasDeclareItems(
	t *testing.T,
	registry *mcpserver.MCPServer,
	registered map[string]struct{},
) {
	t.Helper()
	tools := registry.ListTools()
	require.NotEmpty(t, tools, "registry must advertise tools")
	for toolName, tool := range tools {
		registered[toolName] = struct{}{}
		raw, err := json.Marshal(tool.Tool)
		require.NoError(t, err, "marshal tool %q", toolName)
		var advertised map[string]any
		require.NoError(t, json.Unmarshal(raw, &advertised), "unmarshal tool %q", toolName)
		schema, ok := advertised["inputSchema"].(map[string]any)
		require.True(t, ok, "tool %q must advertise an input schema", toolName)
		assertArraySchemasDeclareItems(t, toolName, "inputSchema", schema)
	}
}

// arraySchemasMissingItems walks JSON Schema keywords that contain schemas and
// reports each array node that does not declare items.
func arraySchemasMissingItems(toolName, path string, node any) []string {
	var missing []string
	var walk func(path string, node any)
	walk = func(path string, node any) {
		switch value := node.(type) {
		case map[string]any:
			if declaresArrayType(value["type"]) {
				items, ok := value["items"]
				if !ok || items == nil {
					missing = append(missing, fmt.Sprintf("tool %q schema %s declares type array without items", toolName, path))
				}
			}
			for key, child := range value {
				switch key {
				case "properties", "patternProperties", "definitions", "$defs", "dependencies", "dependentSchemas":
					entries, ok := child.(map[string]any)
					if !ok {
						continue
					}
					for name, entry := range entries {
						walk(fmt.Sprintf("%s.%s.%s", path, key, name), entry)
					}
				case "items", "prefixItems", "additionalItems", "additionalProperties",
					"contains", "propertyNames", "not", "if", "then", "else",
					"allOf", "anyOf", "oneOf":
					walk(fmt.Sprintf("%s.%s", path, key), child)
				}
			}
		case []any:
			for index, child := range value {
				walk(fmt.Sprintf("%s[%d]", path, index), child)
			}
		}
	}
	walk(path, node)
	return missing
}

func assertArraySchemasDeclareItems(t *testing.T, toolName, path string, node any) {
	t.Helper()
	for _, violation := range arraySchemasMissingItems(toolName, path, node) {
		t.Error(violation)
	}
}

func declaresArrayType(node any) bool {
	switch value := node.(type) {
	case string:
		return value == "array"
	case []any:
		for _, item := range value {
			if declaresArrayType(item) {
				return true
			}
		}
	}
	return false
}
