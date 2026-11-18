package process

import (
	"context"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/shell"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-MCP-APPLY-001.6
func TestMCPProviderReplacementIsolatesWorkspaceEnvironment(t *testing.T) {
	cfg := persistentRestartConfig(t.TempDir())
	cfg.AgentEnv = append(cfg.AgentEnv, "KANDEV_WORKSPACE_SENTINEL=original")
	mgr := newRestartManager(t, cfg)
	startRestartManager(t, mgr)
	_, err := mgr.stopProviderForReplacement(context.Background(), mgr.ProcessGeneration())
	require.NoError(t, err)
	require.NoError(t, mgr.ConfigureWithEnvironment("", fixtureArgs(), true, map[string]string{"KANDEV_PROVIDER_SCOPE_SECRET": "provider-only", "KANDEV_WORKSPACE_SENTINEL": "changed"}, "", nil, false))
	commands, err := mgr.CommandEnvironment()
	require.NoError(t, err)
	require.Equal(t, "original", commands["KANDEV_WORKSPACE_SENTINEL"])
	require.NotContains(t, commands, "KANDEV_PROVIDER_SCOPE_SECRET")
	require.Equal(t, "original", environmentMapFromSlice(mgr.gitEnvironment())["KANDEV_WORKSPACE_SENTINEL"])
	require.NotContains(t, environmentMapFromSlice(mgr.trackerGitEnvironment()), "KANDEV_PROVIDER_SCOPE_SECRET")
	terminal, err := mgr.StartTerminalShell("after-config", shell.DefaultConfig(cfg.WorkDir))
	require.NoError(t, err)
	require.Equal(t, "original", terminal.Config().Env["KANDEV_WORKSPACE_SENTINEL"])
	require.NotContains(t, terminal.Config().Env, "KANDEV_PROVIDER_SCOPE_SECRET")
	require.Equal(t, "provider-only", environmentMapFromSlice(mgr.agentEnvSnapshot())["KANDEV_PROVIDER_SCOPE_SECRET"])
}

func TestMCPProviderReplacementConfigurationSnapshot(t *testing.T) {
	mgr := newRestartManager(t, persistentRestartConfig(t.TempDir()))
	startRestartManager(t, mgr)
	ctx := context.Background()
	source, err := mgr.stopProviderForReplacement(ctx, mgr.ProcessGeneration())
	require.NoError(t, err)
	env := environmentMapFromSlice(mgr.agentEnvSnapshot())
	env["KANDEV_PROVIDER_SCOPE_SECRET"] = "provider-only"
	args := fixtureArgs()
	updated, err := mgr.configureProviderForReplacement(source, args, env)
	require.NoError(t, err)
	args[0] = "mutated-by-caller"
	env["KANDEV_PROVIDER_SCOPE_SECRET"] = "mutated-by-caller"
	require.Equal(t, "provider-only", environmentMapFromSlice(mgr.agentEnvSnapshot())["KANDEV_PROVIDER_SCOPE_SECRET"])
	_, err = mgr.startProviderForReplacement(ctx, updated)
	require.NoError(t, err)
	commands, err := mgr.CommandEnvironment()
	require.NoError(t, err)
	require.NotContains(t, commands, "KANDEV_PROVIDER_SCOPE_SECRET")
	require.Equal(t, "provider-only", environmentMapFromSlice(mgr.agentEnvSnapshot())["KANDEV_PROVIDER_SCOPE_SECRET"])
	_, err = mgr.configureProviderForReplacement(updated, fixtureArgs(), nil)
	require.Error(t, err, "configuration cannot replace a live generation")
}

func TestMCPProviderReplacementPreservesLaunchWrapper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper fixture")
	}
	cfg := persistentRestartConfig(t.TempDir())
	wrapper := filepath.Join(cfg.WorkDir, "wrapper")
	receipt := filepath.Join(cfg.WorkDir, "receipt")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf 'launch\\n' >> \"$RECEIPT\"\nexec \"$@\"\n"), 0700))
	cfg.AgentEnv = append(cfg.AgentEnv, "RECEIPT="+receipt)
	cfg.LaunchWrapper = launchwrapper.Profile([]string{wrapper})
	cfg.LaunchWrapperRequired = true
	mgr := newRestartManager(t, cfg)
	startRestartManager(t, mgr)
	ctx := context.Background()
	source, err := mgr.stopProviderForReplacement(ctx, mgr.ProcessGeneration())
	require.NoError(t, err)
	source, err = mgr.configureProviderForReplacement(source, fixtureArgs(), environmentMapFromSlice(mgr.agentEnvSnapshot()))
	require.NoError(t, err)
	_, err = mgr.startProviderForReplacement(ctx, source)
	require.NoError(t, err)
	// Joining the provider proves that its wrapper executed before inspecting the receipt.
	_, err = mgr.stopProviderForReplacement(ctx, mgr.ProcessGeneration())
	require.NoError(t, err)
	got, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.Equal(t, "launch\nlaunch\n", string(got))
}
