package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func launchFixture(t *testing.T) *Launch {
	t.Helper()
	binary := []byte("fixture runtime")
	files, err := MaterializeRuntime(context.Background(), t.TempDir(), binary, fixtureRuntimeDigest(binary), []RuntimeArtifact{{Name: "profile.json", Content: `{}`}}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	return &Launch{Version: 1, AttemptID: "attempt-1", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable, "--profile", files.Artifacts["profile.json"], "--"}, Digests: files.Digests, EnvironmentKeys: []string{"OPENAI_API_KEY"}}
}

func TestSandboxLaunchWrapsAdmittedCommandAndFiltersFinalEnvironment(t *testing.T) {
	launch := launchFixture(t)
	inner := []string{"/managed/agent", "argument with spaces"}
	argv, env, err := PrepareLaunch(launch, inner, []string{"PATH=/bin", "OPENAI_API_KEY=approved", "KANDEV_TOKEN=control"})
	require.NoError(t, err)
	require.Equal(t, append(append([]string(nil), launch.Prefix...), inner...), argv)
	require.Contains(t, env, "OPENAI_API_KEY=approved")
	require.NotContains(t, strings.Join(env, "\n"), "control")
	argv[0] = "mutated"
	require.NotEqual(t, argv[0], launch.Prefix[0])
}

func TestSandboxLaunchRefusesMissingOrChangedOwnedArtifacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Launch)
	}{
		{"invalid child home", func(l *Launch) { l.ChildHome = "relative/child" }},
		{"old schema", func(l *Launch) { l.Version = 0 }},
		{"missing attempt", func(l *Launch) { l.AttemptID = "" }},
		{"missing policy", func(l *Launch) { l.PolicyDigest = "" }},
		{"replacement prefix", func(l *Launch) { l.Prefix[0] = "/bin/sh" }},
		{"missing runtime digest", func(l *Launch) { delete(l.Digests, "runtime") }},
		{"traversal manifest", func(l *Launch) { l.Digests["../foreign"] = strings.Repeat("a", 64) }},
		{"changed private profile", func(l *Launch) {
			require.NoError(t, os.Chmod(l.Prefix[2], 0600))
			require.NoError(t, os.WriteFile(l.Prefix[2], []byte(`{"broadened":true}`), 0600))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			launch := launchFixture(t)
			tc.mutate(launch)
			argv, env, err := PrepareLaunch(launch, []string{"/agent"}, nil)
			require.ErrorIs(t, err, ErrInvalidPolicy)
			require.Nil(t, argv)
			require.Nil(t, env)
		})
	}
}

func TestSandboxAbsentPreservesOrdinaryLaunch(t *testing.T) {
	args, env, err := PrepareLaunch(nil, []string{"agent"}, []string{"KEY=value"})
	require.NoError(t, err)
	require.Equal(t, []string{"agent"}, args)
	require.Equal(t, []string{"KEY=value"}, env)
}

func TestSandboxLaunchRestrictsMCPRoute(t *testing.T) {
	launch := launchFixture(t)
	for _, route := range []string{"http://localhost:123/mcp", "http://127.0.0.1:123/mcp", "https://127.0.0.1:123/" + strings.Repeat("a", 64) + "/mcp", "http://127.0.0.1:123/" + strings.Repeat("a", 64) + "/mcp?task=other"} {
		launch.MCPURL = route
		require.Error(t, launch.Validate())
	}
	launch.MCPURL = "http://127.0.0.1:123/" + strings.Repeat("a", 64) + "/mcp"
	require.NoError(t, launch.Validate())
}
