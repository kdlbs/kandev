package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxFinalEnvironmentKeepsOnlyApprovedValuesAndOwnedRoots(t *testing.T) {
	root := t.TempDir()
	dirs := RuntimeDirectories{Home: filepath.Join(root, "home"), Config: filepath.Join(root, "config"), State: filepath.Join(root, "state"), Temp: filepath.Join(root, "tmp")}
	for _, dir := range []string{dirs.Home, dirs.Config, dirs.State, dirs.Temp} {
		require.NoError(t, os.Mkdir(dir, 0700))
	}
	values := map[string]string{"PATH": "/bin", "LANG": "C.UTF-8", "HOME": "/real/home", "XDG_CONFIG_HOME": "/real/config", "OPENAI_API_KEY": "approved-fixture", "OTHER_API_KEY": "private-fixture", "KANDEV_TOKEN": "control-fixture", "LD_PRELOAD": "/malicious/library"}
	env, err := IsolatedEnvironment(values, []string{"OPENAI_API_KEY"}, dirs)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"PATH=/bin", "LANG=C.UTF-8", "OPENAI_API_KEY=approved-fixture", "HOME=" + dirs.Home, "XDG_CONFIG_HOME=" + dirs.Config, "XDG_STATE_HOME=" + dirs.State, "TMPDIR=" + dirs.Temp}, env)
	require.Equal(t, "/real/home", values["HOME"])
	require.NotContains(t, strings.Join(env, "\n"), "private-fixture")
	require.NotContains(t, strings.Join(env, "\n"), "control-fixture")
}

func TestSandboxFinalEnvironmentRejectsUnsafeRootsAndValues(t *testing.T) {
	root := t.TempDir()
	dirs := RuntimeDirectories{Home: root, Config: root, State: root, Temp: root}
	_, err := IsolatedEnvironment(map[string]string{"OPENAI_API_KEY": "bad\x00value"}, []string{"OPENAI_API_KEY"}, dirs)
	require.ErrorIs(t, err, ErrInvalidPolicy)
	dirs.Home = "relative"
	_, err = IsolatedEnvironment(nil, nil, dirs)
	require.ErrorIs(t, err, ErrInvalidPolicy)
	dirs.Home = filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(root, dirs.Home))
	_, err = IsolatedEnvironment(nil, nil, dirs)
	require.ErrorIs(t, err, ErrInvalidPolicy)
}
