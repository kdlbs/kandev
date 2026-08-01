package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/stretchr/testify/require"
)

// TestStandalonePluginPack_InstallCompatibility runs the standalone module's
// packer, then sends its archive through the real backend installer. Keeping
// this bridge in the backend module catches format drift while the old
// backend-owned packer remains in place for the staged migration.
func TestStandalonePluginPack_InstallCompatibility(t *testing.T) {
	dir := writeTestPackageDir(t)
	out := filepath.Join(t.TempDir(), "plugin.tar.gz")

	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must locate the test source")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../../.."))
	cmd := exec.Command("go", "run", "./cmd/plugin-pack",
		"-dir", dir,
		"-out", out,
		"-platform-only",
		"-version", "pr-compatibility",
	)
	cmd.Dir = filepath.Join(repoRoot, "pluginsdk")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "standalone plugin-pack output: %s", output)

	archive, err := os.ReadFile(out)
	require.NoError(t, err)
	result, err := pkgtar.Install(bytes.NewReader(archive), t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "kandev-plugin-pack-test", result.Manifest.ID)
	require.Equal(t, "pr-compatibility", result.Version)
}
