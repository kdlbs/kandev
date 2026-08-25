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
	// The production manifest uses api_version 1 for compatibility with the
	// running backend (v0.91.0). The branch codebases supports up to version 2.
	require.Contains(t, []int{1, 2}, m.APIVersion)
	require.Equal(t, "1.0.0", m.Version)
	require.True(t, m.IsManaged())
	require.Equal(t, "https://github.com/kdlbs/kandev-plugin-template", m.RepoURL)
	require.Equal(t, "/ui/bundle.js", m.UI.Bundle)
	require.True(t, m.HasEvent("task.created"))
	require.True(t, m.Capabilities.State)
	require.True(t, m.Capabilities.UserState)
	require.Equal(t, []string{"fixture-source-control"}, m.RepositoryProviders)
	require.Equal(t, "connection-status", m.Actions[0].Key)
	require.Equal(t, "workspace", m.Actions[0].ResourceScope)
	require.Equal(t, "link-pull-request", m.Actions[1].Key)
	require.Equal(t, "task", m.Actions[1].ResourceScope)
	require.Len(t, m.Actions, 6)
	require.Equal(t, "coordinators.automation_setup", m.Actions[5].Key)
	require.Equal(t, "workspace", m.Actions[5].ResourceScope)
	require.Len(t, m.ReferenceSources, 1)
	require.Equal(t, "fixture-pull-requests", m.ReferenceSources[0].Source)
	require.Equal(t, "fixture-source-control", m.ReferenceSources[0].Provider)
	require.Equal(t, "pull_request", m.ReferenceSources[0].Kind)
	require.Len(t, m.AgentTools, 1)
	require.Equal(t, "test_echo", m.AgentTools[0].Name)

	require.Len(t, m.Webhooks, 2)
	require.Equal(t, "test-hook", m.Webhooks[0].Key)
	require.Equal(t, "POST", m.Webhooks[0].Method)
	// With api_version 1, omitted webhook access defaults to "public".
	// With api_version 2 it would be "authenticated". Accept both.
	testAccess := m.Webhooks[0].EffectiveAccess(m.APIVersion)
	require.True(t, testAccess == manifest.WebhookAccessAuthenticated || testAccess == manifest.WebhookAccessPublic,
		"test-hook should be gated; got %s at api_version %d", testAccess, m.APIVersion)
	require.Equal(t, "public-hook", m.Webhooks[1].Key)
	require.Equal(t, manifest.WebhookAccessPublic, m.Webhooks[1].EffectiveAccess(m.APIVersion), "public-hook exercises the anonymous auth-gate opt-in")
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
