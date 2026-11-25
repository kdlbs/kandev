package process

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func TestGenericWrapperSessionCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "independent wrapper")
	child := filepath.Join(dir, "child with spaces")
	receipt := filepath.Join(dir, "receipt")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$RECEIPT\"\nshift\nexec \"$@\"\n"), 0700))
	require.NoError(t, os.WriteFile(child, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700))
	cfg := &config.InstanceConfig{WorkDir: dir, AgentArgs: []string{child, "", "first value"}, ContinueArgs: []string{child, "follow up"},
		AgentEnv: []string{"RECEIPT=" + receipt}, LaunchWrapper: launchwrapper.Profile([]string{wrapper, launchwrapper.WorkDir}), LaunchWrapperRequired: true}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newStubAdapter()
	for range 2 {
		manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: cfg.AgentArgs, ContinueArgs: cfg.ContinueArgs}}
		require.NoError(t, manager.buildFinalCommand())
		output, err := manager.cmd.Output()
		require.NoError(t, err)
		require.Equal(t, "\nfirst value\n", string(output))
		require.NotContains(t, manager.GetFinalCommand(), wrapper)
		for _, args := range [][]string{manager.adapterCfg.OneShotConfig.InitialArgs, manager.adapterCfg.OneShotConfig.ContinueArgs} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Env = cfg.AgentEnv
			cmd.Dir = dir
			require.NoError(t, cmd.Run())
		}
	}
	got, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.NotContains(t, string(got), wrapper, "wrapper must never receive another copy of itself")
	require.Equal(t, []string{child, "", "first value"}, cfg.AgentArgs)
	require.Equal(t, launchwrapper.WorkDir, cfg.LaunchWrapper.Argv[1])
}

func TestGenericWrapperRejectsIncompleteRecoveryState(t *testing.T) {
	cfg := &config.InstanceConfig{AgentArgs: []string{"child"}, WorkDir: t.TempDir(), LaunchWrapperRequired: true}
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	var restored config.InstanceConfig
	require.NoError(t, json.Unmarshal(encoded, &restored))
	manager := NewManager(&restored, newTestLogger(t))
	manager.adapter = newStubAdapter()
	require.Error(t, manager.buildFinalCommand())
	require.Nil(t, manager.cmd)
}

func TestGenericWrapperOneShotStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	receipt := filepath.Join(dir, "one-shot-receipt")
	wrapper := filepath.Join(dir, "launcher")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf 'wrapped\\n' >> \"$RECEIPT\"\nexec \"$@\"\n"), 0700))
	cfg := &config.InstanceConfig{WorkDir: dir, AgentArgs: []string{"/bin/sh", "-c", "exit 0"}, ContinueArgs: []string{"/bin/sh", "-c", "exit 0"},
		AgentEnv: []string{"RECEIPT=" + receipt, "AGENTCTL_AUTH_TOKEN=control-only"}, LaunchWrapper: launchwrapper.Profile([]string{wrapper}), LaunchWrapperRequired: true}
	manager := NewManager(cfg, newTestLogger(t))
	manager.adapter = newOneShotStubAdapter()
	manager.adapterCfg = &adapter.Config{OneShotConfig: &adapter.OneShotConfig{InitialArgs: cfg.AgentArgs, ContinueArgs: cfg.ContinueArgs}}
	require.NoError(t, manager.startOneShot())
	t.Cleanup(func() { close(manager.stopCh); manager.wg.Wait(); manager.workspaceTracker.Stop() })
	for _, args := range [][]string{manager.adapterCfg.OneShotConfig.InitialArgs, manager.adapterCfg.OneShotConfig.ContinueArgs} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = manager.adapterCfg.OneShotConfig.Env
		cmd.Dir = dir
		require.NoError(t, cmd.Run())
	}
	got, err := os.ReadFile(receipt)
	require.NoError(t, err)
	require.Equal(t, "wrapped\nwrapped\n", string(got))
	require.NotContains(t, manager.adapterCfg.OneShotConfig.Env, "AGENTCTL_AUTH_TOKEN=control-only")
}
