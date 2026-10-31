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
	// The ownership rules have their own tests; this one covers the plumbing,
	// and the OS temp directory is not a trustworthy ancestor chain everywhere.
	previous := loadNodeLaunchWrapperFile
	t.Cleanup(func() { loadNodeLaunchWrapperFile = previous })
	var gotPath string
	var gotOwner int
	loadNodeLaunchWrapperFile = func(p string, owner int) (launchwrapper.Spec, error) {
		gotPath, gotOwner = p, owner
		return launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{wrapper, "--"}}, nil
	}
	cfg := &config.Config{}
	require.NoError(t, applyNodeLaunchWrapper(cfg, path, true, 1234))
	require.Equal(t, path, gotPath)
	require.Equal(t, 1234, gotOwner)
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

func allocPolicy(t *testing.T, argv ...string) *config.Config {
	t.Helper()
	spec := launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: argv}
	return &config.Config{NodeLaunchWrapper: launchwrapper.Node{Spec: spec, Required: true}}
}

func allocDirFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX ownership model")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "alloc")
	require.NoError(t, os.Mkdir(dir, 0o700))
	return dir
}

// @covers AC-AGENTS-NODEWRAP-006.4
func TestApplyNodeAllocDirRefusesTokenWithoutValue(t *testing.T) {
	cfg := allocPolicy(t, "/opt/wrap", "--allocation", launchwrapper.AllocDir, "--")
	t.Setenv("NOMAD_ALLOC_DIR", allocDirFixture(t))
	err := applyNodeAllocDir(cfg, "")
	require.ErrorIs(t, err, launchwrapper.ErrAllocDir)
	require.Contains(t, err.Error(), "--node-launch-wrapper-alloc-dir")
	require.Empty(t, cfg.NodeLaunchWrapper.AllocDir, "the process environment is not a source")
}

// @covers AC-AGENTS-NODEWRAP-006.2 AC-AGENTS-NODEWRAP-006.4
func TestApplyNodeAllocDirChecksValueAtStartup(t *testing.T) {
	dir := allocDirFixture(t)
	cfg := allocPolicy(t, "/opt/wrap", launchwrapper.AllocDir)
	require.NoError(t, applyNodeAllocDir(cfg, dir))
	require.Equal(t, dir, cfg.NodeLaunchWrapper.AllocDir)
	require.Equal(t, dir, cfg.NewInstanceConfig(0, nil).NodeLaunchWrapper.AllocDir)

	for name, value := range map[string]string{"relative": "alloc", "missing": filepath.Join(dir, "absent")} {
		cfg := allocPolicy(t, "/opt/wrap", launchwrapper.AllocDir)
		require.ErrorIs(t, applyNodeAllocDir(cfg, value), launchwrapper.ErrAllocDir, name)
		require.Empty(t, cfg.NodeLaunchWrapper.AllocDir, name)
	}
	require.NoError(t, os.Chmod(dir, 0o777))
	require.ErrorIs(t, applyNodeAllocDir(allocPolicy(t, "/opt/wrap", launchwrapper.AllocDir), dir), launchwrapper.ErrAllocDir)
}

// @covers AC-AGENTS-NODEWRAP-006.4
func TestApplyNodeAllocDirIsInertWithoutTheToken(t *testing.T) {
	require.NoError(t, applyNodeAllocDir(allocPolicy(t, "/opt/wrap", launchwrapper.WorkDir), ""))
	require.NoError(t, applyNodeAllocDir(&config.Config{}, ""), "no node wrapper at all")
	cfg := allocPolicy(t, "/opt/wrap", launchwrapper.WorkDir)
	require.NoError(t, applyNodeAllocDir(cfg, "relative-and-unused"))
	require.Equal(t, "relative-and-unused", cfg.NodeLaunchWrapper.AllocDir, "kept for launch, which ignores it")
	require.Error(t, applyNodeAllocDir(&config.Config{}, "/some/dir"), "a value with no wrapper is a misconfiguration")
}
