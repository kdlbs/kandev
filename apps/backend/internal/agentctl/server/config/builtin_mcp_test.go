package config

import (
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPackagesReachInstanceConfiguration(t *testing.T) {
	profile := builtinInstanceTestProfile()
	instance, err := (&Config{ListenHostOverride: "localhost"}).NewInstanceConfig(12345, &InstanceOverrides{TaskID: "task", SessionID: "session", McpProfile: &profile})
	if err != nil {
		t.Fatal(err)
	}
	require.Len(t, instance.McpServers, 6)
	for index, name := range []string{"kandev", "kandev-configuration", "kandev-rich-output"} {
		http, sse := instance.McpServers[index*2], instance.McpServers[index*2+1]
		require.Equal(t, name, http.Name)
		require.Equal(t, name, sse.Name)
		require.Equal(t, "http", http.Type)
		require.Equal(t, "sse", sse.Type)
	}
	require.Equal(t, "http://localhost:12345/mcp/packages/configuration", instance.McpServers[2].URL)
	require.Equal(t, "http://localhost:12345/sse/packages/rich-output", instance.McpServers[5].URL)
}

func builtinInstanceTestProfile() p.Context {
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{
		Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface,
		Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration, p.PackageRichOutput},
		Origins:  map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task", p.PackageRichOutput: "workspace"},
	}
	return profile
}

func TestBuiltinInstanceAdmissionRejectsInvalidAuthorityAndCollisions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*InstanceOverrides)
	}{
		{"foreign task", func(o *InstanceOverrides) { o.TaskID = "other" }},
		{"wrong surface", func(o *InstanceOverrides) { o.McpProfile.Surface = p.SurfaceOfficeTask }},
		{"missing core", func(o *InstanceOverrides) {
			o.McpProfile.BuiltinPackages.Packages = []p.PackageID{p.PackageConfiguration}
		}},
		{"core collision", func(o *InstanceOverrides) {
			o.McpServers = []McpServerConfig{{Name: "kandev", Type: "stdio", Command: "other"}}
		}},
		{"optional collision", func(o *InstanceOverrides) {
			o.McpServers = []McpServerConfig{{Name: "kandev-configuration", Type: "http", URL: "https://external.test/mcp"}}
		}},
		{"inactive collision", func(o *InstanceOverrides) {
			o.McpServers = []McpServerConfig{{Name: "KANDEV-WALKTHROUGHS", Type: "stdio", Command: "other"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := builtinInstanceTestProfile()
			overrides := &InstanceOverrides{TaskID: "task", SessionID: "session", McpProfile: &profile}
			tc.change(overrides)
			instance, err := (&Config{}).NewInstanceConfig(12345, overrides)
			require.Error(t, err)
			require.Nil(t, instance)
		})
	}
	profile := builtinInstanceTestProfile()
	instance, err := (&Config{}).NewInstanceConfig(0, &InstanceOverrides{TaskID: "task", McpProfile: &profile})
	require.Error(t, err, "package authority cannot start without a transport")
	require.Nil(t, instance)
}
