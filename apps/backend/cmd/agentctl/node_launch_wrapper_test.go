package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func nodeWrapperFile(t *testing.T, mode os.FileMode) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX ownership model")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir = filepath.Join(dir, "node")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o755))
	wrapper := filepath.Join(dir, "wrap")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nexec \"$@\"\n"), 0o700))
	data, err := json.Marshal(launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{wrapper, "--"}})
	require.NoError(t, err)
	path := filepath.Join(dir, "node.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return path, wrapper
}

// @covers AC-AGENTS-NODEWRAP-001.1 AC-AGENTS-NODEWRAP-004.1
func TestApplyNodeLaunchWrapperLoadsFile(t *testing.T) {
	path, wrapper := nodeWrapperFile(t, 0o600)
	cfg := &config.Config{}
	require.NoError(t, applyNodeLaunchWrapper(cfg, path, true, -1))
	require.Equal(t, []string{wrapper, "--"}, cfg.NodeLaunchWrapper.Spec.Argv)
	require.True(t, cfg.NodeLaunchWrapper.Required)
	inst := cfg.NewInstanceConfig(0, nil)
	require.Equal(t, cfg.NodeLaunchWrapper, inst.NodeLaunchWrapper)
}

// @covers AC-AGENTS-NODEWRAP-001.4
func TestApplyNodeLaunchWrapperRefusals(t *testing.T) {
	cfg := &config.Config{}
	require.Error(t, applyNodeLaunchWrapper(cfg, "", true, -1), "required without a file")
	require.False(t, cfg.NodeLaunchWrapper.Active() && cfg.NodeLaunchWrapper.Spec.IsZero())

	require.NoError(t, applyNodeLaunchWrapper(&config.Config{}, "", false, -1), "not required, not configured")

	require.Error(t, applyNodeLaunchWrapper(&config.Config{}, filepath.Join(t.TempDir(), "absent.json"), false, -1), "named but missing")

	path, _ := nodeWrapperFile(t, 0o666)
	require.Error(t, applyNodeLaunchWrapper(&config.Config{}, path, false, -1), "world writable")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
	require.NoError(t, os.Chmod(path, 0o600))
	require.Error(t, applyNodeLaunchWrapper(&config.Config{}, path, true, -1), "invalid json")
}
