package profile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuiltinBindingMetadataRejectsTamperingAndPreservesCanonicalSelection(t *testing.T) {
	ctx := New(SurfaceKanbanTask, []Capability{CapabilityTaskTitle}, nil)
	ctx.BuiltinPackages = &BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: ctx.Surface,
		Packages: []PackageID{PackageDiagnostics, PackageCore}, Origins: map[PackageID]string{PackageCore: "required", PackageDiagnostics: "workspace"}}
	encoded, err := MarshalBuiltinBinding(ctx)
	require.NoError(t, err)
	actual, err := ParseBuiltinBindingMetadata(encoded)
	require.NoError(t, err)
	require.NotNil(t, actual)
	require.ElementsMatch(t, ctx.BuiltinPackages.Packages, actual.BuiltinPackages.Packages)
	require.Equal(t, ctx.Capabilities, actual.Capabilities)
	ctx.BuiltinPackages.Packages = []PackageID{PackageCore, PackageDiagnostics}
	reordered, err := MarshalBuiltinBinding(ctx)
	require.NoError(t, err)
	require.Equal(t, encoded, reordered)
	var roundTrip map[string]any
	require.NoError(t, json.Unmarshal([]byte(encoded), &roundTrip))
	_, err = ParseBuiltinBindingMetadata(roundTrip)
	require.NoError(t, err)
	for _, invalid := range []any{nil, "null", `{}`, strings.ReplaceAll(encoded, "workspace", "foreign"), strings.Replace(encoded, `"digest":`, `"unknown":true,"digest":`, 1)} {
		_, err = ParseBuiltinBindingMetadata(invalid)
		require.Error(t, err, invalid)
	}
}
