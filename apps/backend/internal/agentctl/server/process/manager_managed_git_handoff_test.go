package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-KUBERNETES-TASK-POD-001.10
func TestManagerManagedGitConfigurationReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "overlay", true: "complete"}[replace], func(t *testing.T) {
			cfg := &config.InstanceConfig{WorkDir: t.TempDir()}
			mgr := NewManager(cfg, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
			configure := mgr.Configure
			if replace {
				configure = mgr.ConfigureWithEnvironment
			}
			env := map[string]string{
				githubauth.CredentialBrokerURLEnv:  "https://broker.example/resolve",
				githubauth.CredentialLeaseEnv:      "current",
				githubauth.CredentialHelperPathEnv: "/worker/agentctl",
				"GIT_CONFIG_COUNT":                 "4",
				"GIT_CONFIG_KEY_0":                 "core.hooksPath", "GIT_CONFIG_VALUE_0": "/user/hooks",
				"GIT_CONFIG_KEY_1": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_1": "",
				"GIT_CONFIG_KEY_2": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_2": githubauth.ManagedGitCredentialHelper,
				"GIT_CONFIG_KEY_3": "credential.useHttpPath", "GIT_CONFIG_VALUE_3": "true",
			}
			require.NoError(t, configure("echo", nil, false, env, "", nil, false))
			require.Equal(t, "current", envValue(cfg.AgentEnv, githubauth.CredentialLeaseEnv))
			require.NoError(t, configure("echo", nil, false, nil, "", nil, false))
			require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialLeaseEnv))
			require.Empty(t, envValue(cfg.AgentEnv, githubauth.CredentialHelperPathEnv))
			require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), githubauth.ManagedGitCredentialHelper)
			require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "credential.useHttpPath")
			if !replace {
				require.Equal(t, "core.hooksPath", envValue(cfg.AgentEnv, "GIT_CONFIG_KEY_0"))
			}
		})
	}
}

// @covers AC-EXECUTORS-KUBERNETES-TASK-POD-001.9
func TestManagerConfiguredManagedGitSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX executable helper")
	}
	git, err := exec.LookPath("git")
	require.NoError(t, err)
	dir := t.TempDir()
	helper := filepath.Join(dir, "agentctl")
	require.NoError(t, os.WriteFile(helper, []byte(`#!/bin/sh
[ "$1" = git-credential ] || exit 1
[ "$KANDEV_GITHUB_CREDENTIAL_LEASE" = current ] || exit 1
allowed=
while IFS= read -r line; do
 case "$line" in path=owner/allowed.git) allowed=yes;; esac
done
[ "$allowed" = yes ] || exit 1
printf 'username=synthetic\npassword=synthetic\n'
`), 0700))
	cfg := &config.InstanceConfig{WorkDir: dir}
	mgr := NewManager(cfg, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	for _, configure := range []func(string, []string, bool, map[string]string, string, []string, bool) error{mgr.Configure, mgr.ConfigureWithEnvironment} {
		env := map[string]string{
			"PATH": os.Getenv("PATH"), "HOME": dir, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull,
			"GIT_TERMINAL_PROMPT": "0", "GIT_ASKPASS": "", "SSH_ASKPASS": "",
			githubauth.CredentialBrokerURLEnv: "https://broker.example/resolve",
			githubauth.CredentialLeaseEnv:     "current", githubauth.CredentialHelperPathEnv: helper,
			"GIT_CONFIG_COUNT": "3",
			"GIT_CONFIG_KEY_0": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_0": "",
			"GIT_CONFIG_KEY_1": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_1": githubauth.ManagedGitCredentialHelper,
			"GIT_CONFIG_KEY_2": "credential.useHttpPath", "GIT_CONFIG_VALUE_2": "true",
		}
		require.NoError(t, configure("echo", nil, false, env, "", nil, false))
		for _, repo := range []string{"allowed", "foreign"} {
			cmd := exec.Command(git, "credential", "fill")
			cmd.Dir, cmd.Env = dir, append([]string(nil), cfg.AgentEnv...)
			cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\npath=owner/" + repo + ".git\n\n")
			output, err := cmd.Output()
			if repo == "allowed" {
				require.NoError(t, err)
				require.Contains(t, string(output), "username=synthetic")
			} else {
				require.Error(t, err)
				require.Empty(t, output)
			}
		}
	}
}

func TestManagerConfigurePreservesUnmanagedLegacyHelper(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentEnv: []string{
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0=" + githubauth.LegacyGitCredentialHelper,
	}}
	mgr := NewManager(cfg, newTestLogger(t))
	t.Cleanup(func() { require.NoError(t, mgr.StopForTeardown(context.Background())) })
	require.NoError(t, mgr.Configure("echo", nil, false, nil, "", nil, false))
	require.Equal(t, githubauth.LegacyGitCredentialHelper, envValue(cfg.AgentEnv, "GIT_CONFIG_VALUE_0"))
}
