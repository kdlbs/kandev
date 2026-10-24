package mcpconfig

import (
	"encoding/json"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPolicyRejectsDuplicatePackageChoices(t *testing.T) {
	var policy BuiltinWorkspacePolicy
	err := json.Unmarshal([]byte(`{"workspace_id":"workspace","version":1,"packages":{"configuration":"blocked","configuration":"available"}}`), &policy)
	require.Error(t, err)
	var preferences BuiltinTaskPreferences
	err = json.Unmarshal([]byte(`{"workspace_id":"workspace","task_id":"task","version":1,"packages":{"configuration":"off","configuration":"on"}}`), &preferences)
	require.Error(t, err)
}

func TestBuiltinPolicyDefaultsAndTaskOptIn(t *testing.T) {
	manifest, err := ResolveBuiltinPackages("workspace", "task", p.SurfaceKanbanTask, nil, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []p.PackageID{p.PackageCore, p.PackageRichOutput, p.PackageWalkthroughs, p.PackageDiagnostics}, manifest.Packages)
	require.Equal(t, "required", manifest.Origins[p.PackageCore])
	preferences := &BuiltinTaskPreferences{WorkspaceID: "workspace", TaskID: "task", Version: 1, Revision: 3, Packages: map[p.PackageID]string{p.PackageConfiguration: "on", p.PackageDiagnostics: "off"}}
	manifest, err = ResolveBuiltinPackages("workspace", "task", p.SurfaceKanbanTask, nil, preferences)
	require.NoError(t, err)
	require.Contains(t, manifest.Packages, p.PackageConfiguration)
	require.NotContains(t, manifest.Packages, p.PackageDiagnostics)
	require.Equal(t, "task", manifest.Origins[p.PackageConfiguration])
	require.EqualValues(t, 3, manifest.TaskRevision)
	require.NoError(t, manifest.Validate())
}

func TestBuiltinPolicyBlockSuppressesStoredOptIn(t *testing.T) {
	policy := testBuiltinWorkspacePolicy()
	policy.Packages[p.PackageConfiguration] = "blocked"
	preferences := &BuiltinTaskPreferences{WorkspaceID: "workspace", TaskID: "task", Version: 1, Packages: map[p.PackageID]string{p.PackageConfiguration: "on"}}
	manifest, err := ResolveBuiltinPackages("workspace", "task", p.SurfaceKanbanTask, policy, preferences)
	require.NoError(t, err)
	require.NotContains(t, manifest.Packages, p.PackageConfiguration)
	require.Equal(t, "on", preferences.Packages[p.PackageConfiguration])
	policy.Packages[p.PackageConfiguration] = "available"
	manifest, err = ResolveBuiltinPackages("workspace", "task", p.SurfaceKanbanTask, policy, preferences)
	require.NoError(t, err)
	require.Contains(t, manifest.Packages, p.PackageConfiguration)
}

func TestBuiltinPolicySurfaceBoundaries(t *testing.T) {
	manifest, err := ResolveBuiltinPackages("workspace", "task", p.SurfaceOfficeTask, nil, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []p.PackageID{p.PackageCore, p.PackageRichOutput}, manifest.Packages)
	policy := testBuiltinWorkspacePolicy()
	policy.Packages[p.PackageConfiguration] = "blocked"
	manifest, err = ResolveBuiltinPackages("workspace", "task", p.SurfaceConfiguration, policy, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []p.PackageID{p.PackageCore, p.PackageConfiguration}, manifest.Packages)
	require.Equal(t, "required", manifest.Origins[p.PackageConfiguration])
	for _, surface := range []p.Surface{p.SurfaceExternal, p.SurfaceAutomation, p.SurfaceCoordinator, p.SurfaceManagedConversation} {
		manifest, err = ResolveBuiltinPackages("workspace", "task", surface, nil, nil)
		require.NoError(t, err)
		require.Nil(t, manifest, surface)
	}
}

func TestBuiltinPolicyRejectsInvalidStoredAuthority(t *testing.T) {
	for _, mutate := range []func(*BuiltinWorkspacePolicy){
		func(p *BuiltinWorkspacePolicy) { p.Version = 2 },
		func(p *BuiltinWorkspacePolicy) { p.Revision = -1 },
		func(p *BuiltinWorkspacePolicy) { p.WorkspaceID = "foreign" },
		func(v *BuiltinWorkspacePolicy) { v.Packages[p.PackageCore] = "blocked" },
		func(v *BuiltinWorkspacePolicy) { v.Packages[p.PackageConfiguration] = "default" },
		func(v *BuiltinWorkspacePolicy) { v.Packages[p.PackageDiagnostics] = "enabled" },
		func(v *BuiltinWorkspacePolicy) { v.Packages["unknown"] = "default" },
	} {
		policy := testBuiltinWorkspacePolicy()
		mutate(policy)
		manifest, err := ResolveBuiltinPackages("workspace", "task", p.SurfaceKanbanTask, policy, nil)
		require.ErrorIs(t, err, ErrBuiltinPolicyInvalid)
		require.Nil(t, manifest)
	}
	for _, id := range []p.PackageID{p.PackageCore, p.PackageConfiguration, "unknown"} {
		preferences := &BuiltinTaskPreferences{WorkspaceID: "workspace", TaskID: "task", Version: 1, Packages: map[p.PackageID]string{id: "on"}}
		manifest, err := ResolveBuiltinPackages("workspace", "task", p.SurfaceOfficeTask, nil, preferences)
		require.ErrorIs(t, err, ErrBuiltinPolicyInvalid)
		require.Nil(t, manifest)
	}
}

func testBuiltinWorkspacePolicy() *BuiltinWorkspacePolicy {
	return &BuiltinWorkspacePolicy{WorkspaceID: "workspace", Version: 1, Revision: 2, Packages: map[p.PackageID]string{
		p.PackageConfiguration: "available", p.PackageRichOutput: "default", p.PackageWalkthroughs: "default", p.PackageDiagnostics: "default",
	}}
}
