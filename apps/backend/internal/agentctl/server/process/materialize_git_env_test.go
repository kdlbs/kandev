package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

func TestManagerMaterializationGitEnvironment(t *testing.T) {
	installed := filepath.Join(t.TempDir(), "installed agentctl")
	t.Setenv(githubauth.CredentialHelperPathEnv, installed)
	t.Setenv(githubauth.CredentialBrokerURLEnv, "https://stale.example")
	t.Setenv(githubauth.CredentialLeaseEnv, "stale-lease")
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{
		"PATH=/instance/bin", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=/instance/hooks",
		githubauth.CredentialBrokerURLEnv + "=https://current.example", githubauth.CredentialLeaseEnv + "=instance-lease",
	}}, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	// Startup tooling is captured once, and returned snapshots are detached.
	t.Setenv(githubauth.CredentialHelperPathEnv, "/later/ambient/tool")
	snapshot, err := mgr.MaterializationGitEnvironment()
	require.NoError(t, err)
	values := environmentMapFromSlice(snapshot)
	require.Equal(t, installed, values[githubauth.CredentialHelperPathEnv])
	require.Equal(t, "instance-lease", values[githubauth.CredentialLeaseEnv])
	require.Equal(t, "/instance/hooks", values["GIT_CONFIG_VALUE_0"])
	snapshot[0] = "MUTATED=1"
	again, err := mgr.MaterializationGitEnvironment()
	require.NoError(t, err)
	require.NotContains(t, environmentMapFromSlice(again), "MUTATED")
	// A complete reconfiguration removes authorization without reviving the startup lease.
	err = mgr.ConfigureWithEnvironment("agent", nil, false, map[string]string{"GIT_CONFIG_COUNT": "0"}, "", nil, false)
	require.NoError(t, err)
	revoked, err := mgr.MaterializationGitEnvironment()
	require.NoError(t, err)
	require.NotContains(t, environmentMapFromSlice(revoked), githubauth.CredentialLeaseEnv)
	require.NotContains(t, environmentMapFromSlice(revoked), githubauth.CredentialBrokerURLEnv)
	require.NotContains(t, environmentMapFromSlice(revoked), githubauth.CredentialHelperPathEnv)
	require.Equal(t, "instance-lease", values[githubauth.CredentialLeaseEnv])
}

func TestManagerMaterializationGitEnvironmentHelperSelection(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, test := range []struct{ name, installed, instance, want string }{
		{name: "own binary", want: executable},
		{name: "explicit instance helper", installed: "/startup/agentctl", instance: "/instance/agentctl", want: "/instance/agentctl"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(githubauth.CredentialHelperPathEnv, test.installed)
			env := []string{githubauth.CredentialBrokerURLEnv + "=https://broker.example", githubauth.CredentialLeaseEnv + "=lease"}
			if test.instance != "" {
				env = append(env, githubauth.CredentialHelperPathEnv+"="+test.instance)
			}
			mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: env}, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
			got, err := mgr.MaterializationGitEnvironment()
			require.NoError(t, err)
			require.Equal(t, test.want, environmentMapFromSlice(got)[githubauth.CredentialHelperPathEnv])
		})
	}
}

func TestManagerMaterializationGitEnvironmentRejectsMalformedConfig(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=secret-sentinel"}}, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	_, err := mgr.MaterializationGitEnvironment()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-sentinel")
}

func TestManagerMaterializationGitEnvironmentHelperUnavailable(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{githubauth.CredentialBrokerURLEnv + "=https://broker.example", githubauth.CredentialLeaseEnv + "=lease"}}, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	mgr.materializeGitHelperPath = ""
	_, err := mgr.MaterializationGitEnvironment()
	require.ErrorContains(t, err, "unavailable")
}
