package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-SDK-DISTRIBUTION-004.1
func TestPreviewDockerPreservesStandaloneSDKLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake Docker command uses a POSIX shell")
	}
	root := t.TempDir()
	backend := filepath.Join(root, "apps", "backend")
	require.NoError(t, os.MkdirAll(backend, 0755))
	tools := filepath.Join(root, "tools")
	require.NoError(t, os.Mkdir(tools, 0755))
	capture := filepath.Join(root, "docker-args")
	require.NoError(t, os.WriteFile(filepath.Join(tools, "docker"), []byte("#!/usr/bin/env sh\nprintf '%s\\n' \"$@\" > \"$PREVIEW_DOCKER_ARGS\"\nprintf fixture > bin/kandev-preview-build\n"), 0755))
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PREVIEW_DOCKER_ARGS", capture)
	t.Chdir(backend)
	output := filepath.Join(root, "kandev")
	require.NoError(t, buildKandevDocker(context.Background(), output))
	args, err := os.ReadFile(capture)
	require.NoError(t, err)
	require.Contains(t, string(args), backend+":/work/apps/backend\n")
	require.Contains(t, string(args), filepath.Join(root, "pluginsdk")+":/work/pluginsdk:ro\n")
	require.Contains(t, string(args), "-w\n/work/apps/backend\n")
	content, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "fixture", string(content))
}
