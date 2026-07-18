// Guards fixture-package/manifest.yaml against drift: it must stay a valid,
// runtime-managed manifest that declares the id, webhook, and UI
// bundle path the e2e suite and `make e2e-plugin-package` depend on. See
// docs/plans/plugins/GRPC-CONTRACT.md §6.
package main

import (
	_ "embed"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
)

//go:embed fixture-package/manifest.yaml
var fixtureManifestYAML []byte

func TestFixtureManifest_ParsesAndValidates(t *testing.T) {
	m, err := manifest.Parse(fixtureManifestYAML)
	require.NoError(t, err)
	require.NoError(t, m.Validate())

	require.Equal(t, "kandev-plugin-e2e", m.ID)
	require.Equal(t, 1, m.APIVersion)
	require.Equal(t, "1.0.0", m.Version)
	require.True(t, m.IsManaged())
	require.Equal(t, "/ui/bundle.js", m.UI.Bundle)
	require.True(t, m.HasEvent("task.created"))
	require.True(t, m.Capabilities.State)

	require.Len(t, m.Webhooks, 1)
	require.Equal(t, "test-hook", m.Webhooks[0].Key)
	require.Equal(t, "POST", m.Webhooks[0].Method)
}

func TestFixtureManifest_DeclaresHostPlatformExecutable(t *testing.T) {
	m, err := manifest.Parse(fixtureManifestYAML)
	require.NoError(t, err)

	// The Makefile's `e2e-plugin-package` target only ever builds/packs for
	// the host platform, but the committed manifest lists every platform
	// the fixture might run on in CI (linux/darwin/windows, amd64/arm64).
	for platformKey, execPath := range map[string]string{
		"linux-amd64":   "server/plugin-linux-amd64",
		"linux-arm64":   "server/plugin-linux-arm64",
		"darwin-amd64":  "server/plugin-darwin-amd64",
		"darwin-arm64":  "server/plugin-darwin-arm64",
		"windows-amd64": "server/plugin-windows-amd64.exe",
	} {
		require.Equal(t, execPath, m.Runtime.Executables[platformKey], "platform %s", platformKey)
	}
}
