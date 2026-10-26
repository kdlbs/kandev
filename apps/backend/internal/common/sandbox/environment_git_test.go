package sandbox

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsolatedEnvironmentGitSystemConfigByHost(t *testing.T) {
	dirs := runtimeDirectories(t.TempDir())
	for _, dir := range []string{dirs.Home, dirs.Config, dirs.State, dirs.Temp} {
		require.NoError(t, os.Mkdir(dir, 0700))
	}
	env, err := IsolatedEnvironment(map[string]string{"GIT_CONFIG_NOSYSTEM": "0", "GIT_CONFIG_GLOBAL": "/host"}, nil, dirs)
	require.NoError(t, err)
	if runtime.GOOS == "darwin" {
		require.Contains(t, env, "GIT_CONFIG_NOSYSTEM=1")
	} else {
		require.NotContains(t, env, "GIT_CONFIG_NOSYSTEM=1")
	}
	for _, entry := range env {
		require.NotContains(t, entry, "GIT_CONFIG_GLOBAL")
	}
}
