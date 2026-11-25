package types

import (
	"strings"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func testProjectMCPDelivery() *ProjectMCPDelivery {
	return &ProjectMCPDelivery{
		Version: ProjectMCPDeliveryVersion, Strategy: ProjectMCPDeliveryStrategyPi,
		Workspace: "/workspace", TaskID: "task-1", InstanceID: "instance-1",
		BindingDigest: strings.Repeat("a", 64), Packages: []string{"core", "diagnostics"},
	}
}

func TestProjectMCPDeliveryValidation(t *testing.T) {
	require.NoError(t, testProjectMCPDelivery().Validate())
	require.True(t, testProjectMCPDelivery().Delivers(mcpprofile.PackageCore))
	require.False(t, testProjectMCPDelivery().Delivers(mcpprofile.PackageConfiguration))
	require.False(t, (*ProjectMCPDelivery)(nil).Delivers(mcpprofile.PackageCore))

	for name, mutate := range map[string]func(*ProjectMCPDelivery){
		"nil":               func(*ProjectMCPDelivery) {},
		"future version":    func(d *ProjectMCPDelivery) { d.Version = 2 },
		"foreign strategy":  func(d *ProjectMCPDelivery) { d.Strategy = "cursor" },
		"missing instance":  func(d *ProjectMCPDelivery) { d.InstanceID = "" },
		"padded task":       func(d *ProjectMCPDelivery) { d.TaskID = " task-1" },
		"uppercase digest":  func(d *ProjectMCPDelivery) { d.BindingDigest = strings.ToUpper(d.BindingDigest) },
		"short digest":      func(d *ProjectMCPDelivery) { d.BindingDigest = "abc" },
		"no packages":       func(d *ProjectMCPDelivery) { d.Packages = nil },
		"unknown package":   func(d *ProjectMCPDelivery) { d.Packages = []string{"unknown"} },
		"duplicate package": func(d *ProjectMCPDelivery) { d.Packages = []string{"core", "core"} },
	} {
		t.Run(name, func(t *testing.T) {
			delivery := testProjectMCPDelivery()
			if name == "nil" {
				require.Error(t, (*ProjectMCPDelivery)(nil).Validate())
				return
			}
			mutate(delivery)
			require.Error(t, delivery.Validate())
		})
	}
}

func TestProjectMCPDeliveryAcceptsNestedWorkspaceBeyondIdentityLimit(t *testing.T) {
	delivery := testProjectMCPDelivery()
	delivery.Workspace = "/workspace/" + strings.Repeat("nested/", 40) + "task"
	require.Greater(t, len(delivery.Workspace), 256)
	require.NoError(t, delivery.Validate())

	delivery.Workspace = "/workspace/" + strings.Repeat("nested/", 700)
	require.Error(t, delivery.Validate(), "the path-sized workspace cap still applies")
}

func TestProjectMCPDeliveryCloneIsolation(t *testing.T) {
	delivery := testProjectMCPDelivery()
	clone := delivery.Clone()
	clone.Packages[0] = "diagnostics"
	require.Equal(t, []string{"core", "diagnostics"}, delivery.Packages)
	require.Nil(t, (*ProjectMCPDelivery)(nil).Clone())
}

func TestCanonicalProjectMCPDeliveryWorkspace(t *testing.T) {
	for input, want := range map[string]string{
		"/workspace":         "/workspace",
		"/workspace/./repo/": "/workspace/repo",
		"  /workspace  ":     "/workspace",
		"":                   "",
	} {
		require.Equal(t, want, CanonicalProjectMCPDeliveryWorkspace(input), input)
	}
}
