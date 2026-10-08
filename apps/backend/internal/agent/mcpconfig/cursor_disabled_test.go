package mcpconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-CURSOR-PLUGIN-MCP-001.12
func TestCursorMCPDisablesUseSourceRepositoryOnly(t *testing.T) {
	root := t.TempDir()
	cursorHome := t.TempDir()
	sourceRepository := filepath.Join(root, "source-repository")
	taskWorkspace := filepath.Join(root, "task-workspace")
	unrelatedWorkspace := filepath.Join(root, "unrelated-workspace")
	for _, workspace := range []string{sourceRepository, taskWorkspace, unrelatedWorkspace} {
		require.NoError(t, os.MkdirAll(workspace, 0o700))
	}
	sourceBytes := []byte(`["plugin-harness-figma"]`)
	taskBytes := []byte(`["plugin-other-server"]`)
	unrelatedBytes := []byte(`["plugin-atlassian-atlassian"]`)
	sourceStore := writeCursorNativeDisabledStore(t, cursorHome, sourceRepository, sourceBytes)
	taskStore := writeCursorNativeDisabledStore(t, cursorHome, taskWorkspace, taskBytes)
	unrelatedStore := writeCursorNativeDisabledStore(t, cursorHome, unrelatedWorkspace, unrelatedBytes)
	wrongLocalStore := filepath.Join(sourceRepository, ".cursor", cursorMCPDisabledFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(wrongLocalStore), 0o700))
	require.NoError(t, os.WriteFile(wrongLocalStore, []byte(`["plugin-atlassian-atlassian"]`), 0o600))

	got, err := ReadCursorMCPDisabledServers(cursorHome, sourceRepository, taskWorkspace)
	require.NoError(t, err)
	require.Equal(t, map[string]struct{}{"plugin-harness-figma": {}, "plugin-other-server": {}}, got)
	for path, want := range map[string][]byte{
		sourceStore:     sourceBytes,
		taskStore:       taskBytes,
		unrelatedStore:  unrelatedBytes,
		wrongLocalStore: []byte(`["plugin-atlassian-atlassian"]`),
	} {
		actual, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, want, actual, "disable stores are read-only")
	}
}

func TestCursorMCPDisablesMissingFilesAreEmpty(t *testing.T) {
	root := t.TempDir()
	cursorHome := t.TempDir()
	source := filepath.Join(root, "source")
	task := filepath.Join(root, "task")
	require.NoError(t, os.MkdirAll(source, 0o700))
	require.NoError(t, os.MkdirAll(task, 0o700))
	got, err := ReadCursorMCPDisabledServers(cursorHome, source, task)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCursorMCPDisablesUnsafeOrInvalidSelectedStoreFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{name: "malformed JSON", write: func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(`["unterminated"`), 0o600))
		}},
		{name: "not an array", write: func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(`{"plugin-a":true}`), 0o600))
		}},
		{name: "non-string identifier", write: func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(`["plugin-a",5]`), 0o600))
		}},
		{name: "blank identifier", write: func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(`["   "]`), 0o600))
		}},
		{name: "oversized", write: func(t *testing.T, path string) {
			payload := `{"padding":"` + strings.Repeat("x", cursorMCPDisabledMaxBytes) + `"}`
			require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
		}},
		{name: "directory", write: func(t *testing.T, path string) {
			require.NoError(t, os.Mkdir(path, 0o700))
		}},
		{name: "symlink", write: func(t *testing.T, path string) {
			target := filepath.Join(filepath.Dir(path), "outside.json")
			require.NoError(t, os.WriteFile(target, []byte(`["plugin-a"]`), 0o600))
			require.NoError(t, os.Symlink(target, path))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cursorHome := t.TempDir()
			workspace := filepath.Join(t.TempDir(), "source")
			require.NoError(t, os.MkdirAll(workspace, 0o700))
			store := writeCursorNativeDisabledStore(t, cursorHome, workspace, nil)
			tt.write(t, store)

			got, err := ReadCursorMCPDisabledServers(cursorHome, workspace, "")
			require.Error(t, err)
			require.Contains(t, err.Error(), "cursor MCP disabled preferences unavailable")
			require.NotContains(t, err.Error(), "plugin-a")
			require.Empty(t, got)
		})
	}
	t.Run("symlinked project directory", func(t *testing.T) {
		cursorHome := t.TempDir()
		workspace := filepath.Join(t.TempDir(), "source")
		require.NoError(t, os.MkdirAll(workspace, 0o700))
		projects := filepath.Join(cursorHome, "projects")
		require.NoError(t, os.MkdirAll(projects, 0o700))
		outside := t.TempDir()
		outsideStore := filepath.Join(outside, cursorMCPDisabledFilename)
		require.NoError(t, os.WriteFile(outsideStore, []byte(`["plugin-a"]`), 0o600))
		canonical, canonicalErr := canonicalPathIncludingMissingSuffix(workspace)
		require.NoError(t, canonicalErr)
		slug := DeriveCursorProjectSlug(canonical)
		require.NoError(t, os.Symlink(outside, filepath.Join(projects, slug)))
		got, err := ReadCursorMCPDisabledServers(cursorHome, workspace, "")
		require.Error(t, err)
		require.Empty(t, got)
	})
}

func writeCursorNativeDisabledStore(t *testing.T, cursorHome, workspace string, data []byte) string {
	t.Helper()
	canonical, err := canonicalPathIncludingMissingSuffix(workspace)
	require.NoError(t, err)
	store := filepath.Join(cursorHome, "projects", DeriveCursorProjectSlug(canonical), cursorMCPDisabledFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(store), 0o700))
	if data != nil {
		require.NoError(t, os.WriteFile(store, data, 0o600))
	}
	return store
}
