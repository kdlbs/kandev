package plugintools

import "testing"

func TestNormalizeSelectionSortsAndDeduplicates(t *testing.T) {
	normalized := NormalizeSelection([]Selection{
		{PluginID: "b", InstallationID: "i-b", PolicyVersion: "p", ToolNames: []string{"z", "a", "z"}},
		{PluginID: "a", InstallationID: "i-a", PolicyVersion: "p", ToolNames: []string{"b"}},
		{PluginID: "b", InstallationID: "ignored", PolicyVersion: "p", ToolNames: []string{"c"}},
	})
	if len(normalized) != 2 {
		t.Fatalf("normalized = %#v", normalized)
	}
	if normalized[0].PluginID != "a" || normalized[1].PluginID != "b" {
		t.Fatalf("plugin order = %#v", normalized)
	}
	if got := normalized[1].ToolNames; len(got) != 2 || got[0] != "a" || got[1] != "z" {
		t.Fatalf("tool names = %#v", got)
	}
}

func TestSelectionValidateRejectsIncompleteIdentity(t *testing.T) {
	valid := Selection{PluginID: "p", InstallationID: "i", PolicyVersion: "v", ToolNames: []string{"tool"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid selection: %v", err)
	}
	for name, selection := range map[string]Selection{
		"missing installation": {PluginID: "p", PolicyVersion: "v", ToolNames: []string{"tool"}},
		"missing policy":       {PluginID: "p", InstallationID: "i", ToolNames: []string{"tool"}},
		"empty tools":          {PluginID: "p", InstallationID: "i", PolicyVersion: "v"},
		"duplicate tools":      {PluginID: "p", InstallationID: "i", PolicyVersion: "v", ToolNames: []string{"tool", "tool"}},
	} {
		if err := selection.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestProjectNonAutomationKeepsMixedSurfacesAndDropsAutomationOnly(t *testing.T) {
	snapshot := Snapshot{Tools: []Definition{
		{PluginID: "p", LocalName: "mixed", Surfaces: []string{SurfaceAutomation, SurfaceKanban}},
		{PluginID: "p", LocalName: "automation", Surfaces: []string{SurfaceAutomation}},
		{PluginID: "p", LocalName: "kanban", Surfaces: []string{SurfaceKanban}},
	}}
	projected := ProjectNonAutomation(snapshot)
	if len(projected.Tools) != 2 {
		t.Fatalf("projected tools = %#v", projected.Tools)
	}
	if got := projected.Tools[0].Surfaces; len(got) != 1 || got[0] != SurfaceKanban {
		t.Fatalf("mixed surfaces = %#v", got)
	}
	if projected.Tools[1].LocalName != "kanban" {
		t.Fatalf("projected order = %#v", projected.Tools)
	}
	if len(snapshot.Tools[0].Surfaces) != 2 {
		t.Fatal("projection mutated the source snapshot")
	}
}

func TestProjectLegacySafeDropsEveryNewContext(t *testing.T) {
	snapshot := Snapshot{Tools: []Definition{
		{PluginID: "p", LocalName: "mixed", Surfaces: []string{SurfaceKanban, SurfaceAutomation, SurfaceCoordinator, SurfaceExternal}},
		{PluginID: "p", LocalName: "coordinator", Surfaces: []string{SurfaceCoordinator}},
		{PluginID: "p", LocalName: "external", Surfaces: []string{SurfaceExternal}},
		{PluginID: "p", LocalName: "office", Surfaces: []string{SurfaceOffice}},
	}}
	projected := ProjectLegacySafe(snapshot)
	if len(projected.Tools) != 2 {
		t.Fatalf("projected tools = %#v", projected.Tools)
	}
	if got := projected.Tools[0].Surfaces; len(got) != 1 || got[0] != SurfaceKanban {
		t.Fatalf("mixed surfaces = %#v", got)
	}
	if projected.Tools[1].Surfaces[0] != SurfaceOffice {
		t.Fatalf("office tool = %#v", projected.Tools[1])
	}
	if len(snapshot.Tools[0].Surfaces) != 4 {
		t.Fatal("projection mutated the source snapshot")
	}
}

func TestProjectLegacySafeProjectsWorkspaceScopeToTask(t *testing.T) {
	snapshot := Snapshot{Tools: []Definition{
		{PluginID: "p", LocalName: "kanban", InvocationScope: InvocationScopeWorkspace,
			Surfaces: []string{SurfaceKanban, SurfaceExternal}},
		{PluginID: "p", LocalName: "office", InvocationScope: InvocationScopeWorkspace,
			Surfaces: []string{SurfaceOffice}},
		{PluginID: "p", LocalName: "external_only", InvocationScope: InvocationScopeWorkspace,
			Surfaces: []string{SurfaceExternal}},
		{PluginID: "p", LocalName: "instance", InvocationScope: InvocationScopeInstance,
			Surfaces: []string{SurfaceKanban, SurfaceExternal}},
	}}
	projected := ProjectLegacySafe(snapshot)
	if len(projected.Tools) != 3 {
		t.Fatalf("projected tools = %#v", projected.Tools)
	}
	if got := projected.Tools[0]; got.LocalName != "kanban" || got.InvocationScope != InvocationScopeTask ||
		len(got.Surfaces) != 1 || got.Surfaces[0] != SurfaceKanban {
		t.Fatalf("workspace kanban projection = %#v", got)
	}
	if got := projected.Tools[1]; got.LocalName != "office" || got.InvocationScope != InvocationScopeTask {
		t.Fatalf("workspace office projection = %#v", got)
	}
	if got := projected.Tools[2]; got.LocalName != "instance" || got.InvocationScope != InvocationScopeInstance {
		t.Fatalf("instance projection = %#v", got)
	}
	if got := snapshot.Tools[0].InvocationScope; got != InvocationScopeWorkspace {
		t.Fatalf("projection mutated the source scope to %q", got)
	}
}

func TestAutomationAuthorizationAllowsOnlyExactPluginAndTool(t *testing.T) {
	authorization := AutomationAuthorization{
		AutomationID: "a", WorkspaceID: "w", PluginID: "p",
		InstallationID: "i", PolicyVersion: "v", ToolNames: []string{"read"},
	}
	if err := authorization.Validate(); err != nil {
		t.Fatalf("valid authorization: %v", err)
	}
	if !authorization.Allows("p", "read") {
		t.Fatal("expected exact tool to be allowed")
	}
	for _, allowed := range []bool{
		authorization.Allows("other", "read"),
		authorization.Allows("p", "write"),
	} {
		if allowed {
			t.Fatal("authorization widened beyond the exact plugin and tool")
		}
	}
}

func TestProjectCoordinatorOnlyKeepsCoordinatorSurface(t *testing.T) {
	snapshot := Snapshot{Tools: []Definition{
		{PluginID: "p", LocalName: "mixed", Surfaces: []string{SurfaceKanban, SurfaceCoordinator, SurfaceExternal}},
		{PluginID: "p", LocalName: "task", Surfaces: []string{SurfaceKanban}},
	}}
	projected := ProjectCoordinatorOnly(snapshot)
	if len(projected.Tools) != 1 {
		t.Fatalf("projected tools = %#v", projected.Tools)
	}
	if surfaces := projected.Tools[0].Surfaces; len(surfaces) != 1 || surfaces[0] != SurfaceCoordinator {
		t.Fatalf("projected surfaces = %#v", surfaces)
	}
}
