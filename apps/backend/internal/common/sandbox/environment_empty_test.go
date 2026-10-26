package sandbox

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsolatedEnvironmentEmptyApprovedValueIsNotEffectiveOnDarwin(t *testing.T) {
	dirs := runtimeDirectories(t.TempDir())
	for _, dir := range []string{dirs.Home, dirs.Config, dirs.State, dirs.Temp} {
		require.NoError(t, os.Mkdir(dir, 0700))
	}
	env, err := IsolatedEnvironment(map[string]string{"OPENAI_API_KEY": "", "ANTHROPIC_API_KEY": "k"}, []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "MISSING_KEY"}, dirs)
	require.NoError(t, err)
	require.Contains(t, env, "ANTHROPIC_API_KEY=k")
	require.NotContains(t, env, "MISSING_KEY=")
	if runtime.GOOS == "darwin" {
		require.NotContains(t, env, "OPENAI_API_KEY=")
		require.False(t, EffectiveEnvironmentValue(""))
	} else {
		require.Contains(t, env, "OPENAI_API_KEY=")
	}
	require.True(t, EffectiveEnvironmentValue("x"))
}
