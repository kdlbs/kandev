package manifest

import (
	"strings"
	"testing"
)

func TestAgentToolInvocationScope(t *testing.T) {
	readOnly := true
	notReadOnly := false

	tests := []struct {
		name    string
		tool    AgentTool
		wantErr string
	}{
		{
			name: "omitted scope defaults to task",
			tool: AgentTool{
				Name: "add_tag", Description: "Add a tag.",
				Surfaces: []string{AgentToolSurfaceKanban}, InputSchema: map[string]any{"type": "object"},
			},
		},
		{
			name: "explicit task scope preserves task behavior",
			tool: AgentTool{
				Name: "add_tag", Description: "Add a tag.", InvocationScope: AgentToolInvocationScopeTask,
				Surfaces: []string{AgentToolSurfaceKanban}, InputSchema: map[string]any{"type": "object"},
			},
		},
		{
			name: "instance scope keeps task surfaces",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.", InvocationScope: AgentToolInvocationScopeInstance,
				Surfaces:    []string{AgentToolSurfaceKanban, AgentToolSurfaceAutomation},
				InputSchema: map[string]any{"type": "object"},
			},
		},
		{
			name: "unknown scope rejected",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.", InvocationScope: "global",
				Surfaces: []string{AgentToolSurfaceKanban}, InputSchema: map[string]any{"type": "object"},
			},
			wantErr: "invocation_scope",
		},
		{
			name: "coordinator surface requires instance scope",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.",
				Surfaces:    []string{AgentToolSurfaceCoordinator},
				InputSchema: map[string]any{"type": "object"},
				Annotations: AgentToolAnnotations{ReadOnlyHint: &readOnly},
			},
			wantErr: "invocation_scope: instance",
		},
		{
			name: "external surface requires a true read-only hint",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.", InvocationScope: AgentToolInvocationScopeInstance,
				Surfaces:    []string{AgentToolSurfaceExternal},
				InputSchema: map[string]any{"type": "object"},
				Annotations: AgentToolAnnotations{ReadOnlyHint: &notReadOnly},
			},
			wantErr: "read_only_hint: true",
		},
		{
			name: "external surface requires the hint at all",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.", InvocationScope: AgentToolInvocationScopeInstance,
				Surfaces:    []string{AgentToolSurfaceExternal},
				InputSchema: map[string]any{"type": "object"},
			},
			wantErr: "read_only_hint: true",
		},
		{
			name: "instance-read external surface accepted",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.", InvocationScope: AgentToolInvocationScopeInstance,
				Surfaces:    []string{AgentToolSurfaceKanban, AgentToolSurfaceExternal},
				InputSchema: map[string]any{"type": "object"},
				Annotations: AgentToolAnnotations{ReadOnlyHint: &readOnly},
			},
		},
		{
			name: "configuration remains unsupported",
			tool: AgentTool{
				Name: "provider_usage", Description: "Read cached usage.", InvocationScope: AgentToolInvocationScopeInstance,
				Surfaces: []string{"configuration"}, InputSchema: map[string]any{"type": "object"},
			},
			wantErr: "unsupported surface",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := managedManifest(t)
			m.MinKandevVersion = "0.99.0"
			m.AgentTools = []AgentTool{tc.tool}
			err := m.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestAgentToolWorkspaceScopeValidation(t *testing.T) {
	readOnly := true
	workspaceSchema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{"type": "string"},
			},
		}
	}
	workspaceTool := func(surfaces ...string) AgentTool {
		return AgentTool{
			Name: "read_workspace_data", Description: "Read workspace data.",
			InvocationScope: AgentToolInvocationScopeWorkspace,
			Surfaces:        surfaces,
			InputSchema:     workspaceSchema(),
			Annotations:     AgentToolAnnotations{ReadOnlyHint: &readOnly},
		}
	}
	tests := []struct {
		name    string
		tool    AgentTool
		minimum string
		wantErr string
	}{
		{
			name:    "external workspace read accepted",
			tool:    workspaceTool(AgentToolSurfaceKanban, AgentToolSurfaceExternal),
			minimum: MinimumWorkspaceScopeVersion,
		},
		{
			name:    "office workspace read accepted",
			tool:    workspaceTool(AgentToolSurfaceOffice),
			minimum: MinimumWorkspaceScopeVersion,
		},
		{
			name: "kanban workspace tool does not require a read-only hint",
			tool: AgentTool{
				Name: "read_workspace_data", Description: "Read workspace data.",
				InvocationScope: AgentToolInvocationScopeWorkspace,
				Surfaces:        []string{AgentToolSurfaceKanban},
				InputSchema:     workspaceSchema(),
			},
			minimum: MinimumWorkspaceScopeVersion,
		},
		{
			name:    "workspace scope rejects automation",
			tool:    workspaceTool(AgentToolSurfaceAutomation),
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: `"automation", which is not supported`,
		},
		{
			name:    "workspace scope rejects coordinator",
			tool:    workspaceTool(AgentToolSurfaceCoordinator),
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: `"coordinator", which is not supported`,
		},
		{
			name:    "workspace scope rejects managed conversations",
			tool:    workspaceTool(AgentToolSurfaceManaged),
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: `"managed-conversation", which is not supported`,
		},
		{
			name: "external workspace read requires the read-only hint",
			tool: func() AgentTool {
				tool := workspaceTool(AgentToolSurfaceExternal)
				tool.Annotations = AgentToolAnnotations{}
				return tool
			}(),
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: "read_only_hint: true",
		},
		{
			name: "workspace input schema requires a top-level object",
			tool: func() AgentTool {
				tool := workspaceTool(AgentToolSurfaceKanban)
				tool.InputSchema = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
				return tool
			}(),
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: "top-level type: object",
		},
		{
			name: "workspace input schema rejects the reserved argument",
			tool: func() AgentTool {
				tool := workspaceTool(AgentToolSurfaceKanban)
				tool.InputSchema = map[string]any{
					"type":       "object",
					"properties": map[string]any{AgentToolWorkspaceArgument: map[string]any{"type": "string"}},
				}
				return tool
			}(),
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: "reserved property",
		},
		{
			name:    "workspace scope requires the compatibility floor",
			tool:    workspaceTool(AgentToolSurfaceKanban),
			minimum: "0.99.0",
			wantErr: "min_kandev_version >= " + MinimumWorkspaceScopeVersion,
		},
		{
			name: "task scope cannot claim the external surface",
			tool: AgentTool{
				Name: "read_workspace_data", Description: "Read workspace data.",
				InvocationScope: AgentToolInvocationScopeTask,
				Surfaces:        []string{AgentToolSurfaceExternal},
				InputSchema:     workspaceSchema(),
				Annotations:     AgentToolAnnotations{ReadOnlyHint: &readOnly},
			},
			minimum: MinimumWorkspaceScopeVersion,
			wantErr: "invocation_scope: instance or workspace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := managedManifest(t)
			m.MinKandevVersion = tc.minimum
			m.AgentTools = []AgentTool{tc.tool}
			err := m.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestAgentToolInvocationScopeIsNormalizedOnParse(t *testing.T) {
	m, err := Parse([]byte(managedManifestYAML + `
agent_tools:
  - name: provider_usage
    description: Read cached usage.
    surfaces: [kanban-task]
    input_schema:
      type: object
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if got := m.AgentTools[0].InvocationScope; got != AgentToolInvocationScopeTask {
		t.Fatalf("InvocationScope = %q, want normalized task scope", got)
	}
	if got := m.AgentTools[0].NormalizedInvocationScope(); got != AgentToolInvocationScopeTask {
		t.Fatalf("NormalizedInvocationScope() = %q, want task", got)
	}
}

func TestInstanceReadSurfacesRequireCompatibilityFloor(t *testing.T) {
	readOnly := true
	for _, surface := range []string{AgentToolSurfaceCoordinator, AgentToolSurfaceExternal} {
		t.Run(surface, func(t *testing.T) {
			m := managedManifest(t)
			m.AgentTools = []AgentTool{{
				Name: "provider_usage", Description: "Read cached usage.",
				Surfaces:        []string{surface},
				InvocationScope: AgentToolInvocationScopeInstance,
				InputSchema:     map[string]any{"type": "object"},
				Annotations:     AgentToolAnnotations{ReadOnlyHint: &readOnly},
			}}
			if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "min_kandev_version") {
				t.Fatalf("Validate() error = %v, want compatibility floor", err)
			}
			m.MinKandevVersion = "0.99.0"
			if err := m.Validate(); err != nil {
				t.Fatalf("Validate() with the compatibility floor: %v", err)
			}
		})
	}
}
