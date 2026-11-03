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
