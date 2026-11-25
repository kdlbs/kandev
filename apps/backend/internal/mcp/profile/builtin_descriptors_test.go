package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinDescriptorsOwnNamesDefaultsAndActions(t *testing.T) {
	catalog := BuiltinPackageCatalog()
	require.Len(t, catalog, 5)
	wantNames := map[PackageID]string{PackageCore: "kandev", PackageConfiguration: "kandev-configuration", PackageRichOutput: "kandev-rich-output", PackageWalkthroughs: "kandev-walkthroughs", PackageDiagnostics: "kandev-diagnostics"}
	seen := make(map[string]bool)
	for _, descriptor := range catalog {
		require.Equal(t, wantNames[descriptor.ID], descriptor.RuntimeName)
		require.NotEmpty(t, descriptor.LabelKey)
		require.NotEmpty(t, descriptor.DescriptionKey)
		require.NotEmpty(t, descriptor.Surfaces)
		require.NotEmpty(t, descriptor.Tools)
		for _, tool := range descriptor.Tools {
			require.False(t, seen[tool.Name], tool.Name)
			seen[tool.Name] = true
			require.Equal(t, descriptor.ID, BuiltinToolPackage(tool.Name))
			require.NotEmpty(t, tool.Actions, tool.Name)
		}
		switch descriptor.ID {
		case PackageCore:
			require.Equal(t, "required", descriptor.DefaultPolicy)
			require.ElementsMatch(t, descriptor.Surfaces, descriptor.RequiredSurfaces)
		case PackageConfiguration:
			require.Equal(t, "available", descriptor.DefaultPolicy)
			require.Equal(t, []Surface{SurfaceConfiguration}, descriptor.RequiredSurfaces)
		default:
			require.Equal(t, "default", descriptor.DefaultPolicy)
			require.Empty(t, descriptor.RequiredSurfaces)
		}
	}
	require.Len(t, seen, len(builtinToolPackages))
	catalog[0].Tools[0].Actions[0] = "corrupt"
	require.NotEqual(t, catalog[0].Tools, BuiltinPackageCatalog()[0].Tools)
}
