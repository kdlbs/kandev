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

func TestBuiltinBindingDigestMatchesSealedMetadata(t *testing.T) {
	ctx := New(SurfaceKanbanTask, nil, nil)
	ctx.BuiltinPackages = &BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: ctx.Surface,
		Packages: []PackageID{PackageCore, PackageDiagnostics}, Origins: map[PackageID]string{PackageCore: "required", PackageDiagnostics: "task"}}
	digest, err := BuiltinBindingDigest(ctx)
	require.NoError(t, err)
	encoded, err := MarshalBuiltinBinding(ctx)
	require.NoError(t, err)
	var binding struct {
		Digest string `json:"digest"`
	}
	require.NoError(t, json.Unmarshal([]byte(encoded), &binding))
	require.Equal(t, binding.Digest, digest)

	reordered := ctx
	reordered.BuiltinPackages = ctx.BuiltinPackages.Clone()
	reordered.BuiltinPackages.Packages = []PackageID{PackageDiagnostics, PackageCore}
	canonical, err := BuiltinBindingDigest(reordered)
	require.NoError(t, err)
	require.Equal(t, digest, canonical, "package order must not change the canonical digest")

	changed := ctx
	changed.BuiltinPackages = ctx.BuiltinPackages.Clone()
	changed.BuiltinPackages.Packages = []PackageID{PackageCore}
	changed.BuiltinPackages.Origins = map[PackageID]string{PackageCore: "required"}
	other, err := BuiltinBindingDigest(changed)
	require.NoError(t, err)
	require.NotEqual(t, digest, other)
}
