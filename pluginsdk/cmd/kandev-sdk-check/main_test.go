package main

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.2
func TestFrontendOnlyCommand(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "node_modules", "@kdlbs", "plugin-sdk")
	require.NoError(t, os.MkdirAll(pkg, 0755))
	files := map[string]any{
		"package.json": map[string]any{"dependencies": map[string]string{"@kdlbs/plugin-sdk": "0.1.0"}},
		"node_modules/@kdlbs/plugin-sdk/package.json":     map[string]string{"name": "@kdlbs/plugin-sdk", "version": "0.1.0"},
		"node_modules/@kdlbs/plugin-sdk/sdk-release.json": map[string]any{"schema_version": 1, "sdk_version": "0.1.0", "host_release": "0.100.0", "min_kandev_version": "0.100.0", "api_versions": []int{1, 2}, "change_kind": "initial"},
		"package-lock.json":                               map[string]any{"lockfileVersion": 3, "packages": map[string]any{"": map[string]any{"dependencies": map[string]string{"@kdlbs/plugin-sdk": "0.1.0"}}, "node_modules/@kdlbs/plugin-sdk": map[string]any{"version": "0.1.0", "resolved": "https://registry.npmjs.org/@kdlbs/plugin-sdk/-/plugin-sdk-0.1.0.tgz", "integrity": "sha512-" + strings.Repeat("A", 86) + "=="}}},
	}
	for path, value := range files {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, path), data, 0600))
	}
	manifest := filepath.Join(root, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte("api_version: 2\nmin_kandev_version: 0.100.0\n"), 0600))
	var out, errOut bytes.Buffer
	args := []string{"--frontend-only", "--frontend-sdk", "node_modules/@kdlbs/plugin-sdk"}
	require.Equal(t, 0, run(args, root, &out, &errOut), errOut.String())
	require.Contains(t, out.String(), "0.1.0")
	require.NoError(t, os.WriteFile(manifest, []byte("api_version: 2\nmin_kandev_version: 0.99.0\n"), 0600))
	require.Equal(t, 1, run(args, root, &out, &errOut))
	require.Contains(t, errOut.String(), "requires min_kandev_version")
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	require.True(t, os.IsNotExist(err))
}
func TestCommandRejectsMissingFrontendSelection(t *testing.T) {
	var out, errOut bytes.Buffer
	require.Equal(t, 2, run([]string{"--frontend-only"}, t.TempDir(), &out, &errOut))
	require.Contains(t, errOut.String(), "--frontend-sdk")
}
