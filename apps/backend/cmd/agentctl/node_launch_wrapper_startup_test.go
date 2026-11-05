package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// TestApplyNodeLaunchWrapperDoesNotRunTheWrapper pins that agentctl startup
// only loads the node wrapper file; it never executes the wrapper. The loader is
// replaced by a stub that returns a wrapper recording every run in a sentinel.
func TestApplyNodeLaunchWrapperDoesNotRunTheWrapper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "wrapper-ran")
	wrapper := filepath.Join(dir, "wrap")
	require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\necho ran >> '"+sentinel+"'\nexec \"$@\"\n"), 0o700))

	previous := loadNodeLaunchWrapperFile
	t.Cleanup(func() { loadNodeLaunchWrapperFile = previous })
	loadNodeLaunchWrapperFile = func(string, int) (launchwrapper.Spec, error) {
		return launchwrapper.Spec{Version: launchwrapper.Version, Source: launchwrapper.SourceNode, Argv: []string{wrapper, "--"}}, nil
	}

	cfg := &config.Config{}
	require.NoError(t, applyNodeLaunchWrapper(cfg, filepath.Join(dir, "node.json"), true, -1))
	require.Equal(t, []string{wrapper, "--"}, cfg.NodeLaunchWrapper.Spec.Argv)
	require.NoFileExists(t, sentinel, "agentctl startup executed the node launch wrapper")
}

// TestAgentctlStartupReferencesOnlyTheNodeWrapperLoader pins, at the source
// level, that the startup files of cmd/agentctl reach the launchwrapper package
// only through LoadNodeFile, the Node and Spec types and the pure allocation
// directory checks (UsesAllocDir, ResolveAllocDir), and that
// node_launch_wrapper.go starts no process. The functions that compose and run a
// wrapper (Node.Prepare, Prepare) are used only by the process manager and the
// utility executors, which run after an authenticated instance create.
func TestAgentctlStartupReferencesOnlyTheNodeWrapperLoader(t *testing.T) {
	allowed := map[string]bool{"LoadNodeFile": true, "Node": true, "UsesAllocDir": true, "ResolveAllocDir": true}
	for _, file := range []string{"main.go", "node_launch_wrapper.go"} {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		require.NoError(t, err)
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			if file == "node_launch_wrapper.go" {
				require.NotEqual(t, "os/exec", path, "node_launch_wrapper.go must not start processes")
				require.NotEqual(t, "syscall", path, "node_launch_wrapper.go must not exec")
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name == "launchwrapper" {
				require.True(t, allowed[selector.Sel.Name], "%s uses launchwrapper.%s; startup may only load the wrapper", file, selector.Sel.Name)
			}
			if file == "node_launch_wrapper.go" {
				require.NotEqual(t, "Prepare", selector.Sel.Name, "node_launch_wrapper.go must not compose a wrapped command")
			}
			return true
		})
	}
}
