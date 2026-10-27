package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxRuntimePackageIdentitySurvivesReloadAndUpgrade(t *testing.T) {
	svc, _, _ := newTestService(t)
	first := installTestPlugin(t, svc, "kandev-plugin-slack")
	require.Len(t, first.PackageDigest, 64)
	reloaded, err := svc.store.Get(first.ID)
	require.NoError(t, err)
	require.Equal(t, first.PackageDigest, reloaded.PackageDigest)
	second, err := svc.Install(context.Background(), testPackage(t, first.ID, "1.1.0", false))
	require.NoError(t, err)
	require.Len(t, second.PackageDigest, 64)
	require.NotEqual(t, first.PackageDigest, second.PackageDigest)
}

func TestSandboxRuntimeReadsSelectedPackageOnly(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.SetSandboxProvidersEnabled(true)
	content := []byte("fixture executable bytes")
	installed, err := svc.Install(t.Context(), sandboxFixturePackage(t, content, "1.0.0"))
	require.NoError(t, err)
	binding := sandboxFixtureBinding(installed, "fixture-alpha")
	digest := sha256.Sum256(content)
	artifact := &pluginsdk.SandboxRuntimeArtifact{RelativePath: "server/plugin", Sha256: hex.EncodeToString(digest[:])}
	got, err := svc.ReadSandboxRuntime(t.Context(), binding, artifact)
	require.NoError(t, err)
	require.Equal(t, content, got.Content)
	require.Equal(t, installed.PackageDigest, got.PackageDigest)
	stale := binding
	stale.PackageDigest = "stale"
	_, err = svc.ReadSandboxRuntime(t.Context(), stale, artifact)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	svc.SetSandboxProvidersEnabled(false)
	_, err = svc.ReadSandboxRuntime(t.Context(), binding, artifact)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	svc.SetSandboxProvidersEnabled(true)
	require.NoError(t, os.WriteFile(filepath.Join(installed.InstallPath, "server/plugin"), []byte("changed"), 0600))
	_, err = svc.ReadSandboxRuntime(t.Context(), binding, artifact)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
}
