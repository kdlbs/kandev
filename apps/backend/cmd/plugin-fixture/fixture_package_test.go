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
	require.Equal(t, manifest.CurrentAPIVersion, m.APIVersion)
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
	actions := make(map[string]manifest.Action, len(m.Actions))
	for _, action := range m.Actions {
		actions[action.Key] = action
	}
	require.Equal(t, "workspace", actions[repositoryInspectActionKey].ResourceScope)
	require.Equal(t, "workspace", actions[repositoryBranchesActionKey].ResourceScope)
	require.Equal(t, "task", actions["link-pull-request"].ResourceScope)
	require.Len(t, m.ReferenceSources, 1)
	require.Equal(t, "fixture-pull-requests", m.ReferenceSources[0].Source)
	require.Equal(t, "fixture-source-control", m.ReferenceSources[0].Provider)
	require.Equal(t, "pull_request", m.ReferenceSources[0].Kind)
	require.Len(t, m.AgentTools, 1)
	require.Equal(t, "test_echo", m.AgentTools[0].Name)

	require.Len(t, m.Webhooks, 2)
	require.Equal(t, "test-hook", m.Webhooks[0].Key)
	require.Equal(t, "POST", m.Webhooks[0].Method)
	require.Equal(t, manifest.WebhookAccessAuthenticated, m.Webhooks[0].EffectiveAccess(m.APIVersion), "test-hook exercises the private (auth-gated) webhook path")
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
