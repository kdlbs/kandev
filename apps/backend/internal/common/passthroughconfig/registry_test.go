package passthroughconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareRollsBackWhenRegistrySaveFails(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "private", "mcp.json")
	files := []File{{Path: path, Content: []byte(`{}`)}, {Path: filepath.Join(workspace, registryPath, "obstruct"), Content: []byte(`{}`)}}
	require.Error(t, Prepare(context.Background(), workspace, workspace, "owner", files))
	require.NoFileExists(t, path, "a failed registry save must not leave an unclaimed private config")
	require.NoError(t, os.RemoveAll(filepath.Join(workspace, registryPath)))
	require.NoError(t, Prepare(context.Background(), workspace, workspace, "owner", files[:1]))
	require.NoError(t, Cleanup(context.Background(), workspace, "owner", false))
	require.NoFileExists(t, path)
}

func TestPrepareRollsBackEarlierFilesOnLaterFailure(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "mcp.json")
	original := []byte(`{"mcpServers":{"user":{"url":"http://user"}}}`)
	require.NoError(t, os.WriteFile(path, original, 0600))
	files := []File{{Path: path, Content: []byte(`{"mcpServers":{"agent":{}}}`), MergeKey: "mcpServers"}, {Path: filepath.Join(workspace, "..", "outside.json"), Content: []byte(`{}`)}}
	require.Error(t, Prepare(context.Background(), workspace, workspace, "owner", files))
	actual, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, actual)
}

func TestPrepareRollbackPreservesExternalReplacement(t *testing.T) {
	workspace := t.TempDir()
	root, err := os.OpenRoot(workspace)
	require.NoError(t, err)
	defer func() { require.NoError(t, root.Close()) }()
	change, err := captureRemoteFile(root, "mcp.json")
	require.NoError(t, err)
	file := File{Content: []byte("owned")}
	require.NoError(t, materializeRemoteFile(root, "mcp.json", "owner", file, registry{}, &change))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "user.json"), []byte("user replacement"), 0600))
	require.NoError(t, root.Rename("user.json", "mcp.json"))
	require.NoError(t, rollbackRemoteFiles(root, []remoteFileChange{change}))
	actual, err := root.ReadFile("mcp.json")
	require.NoError(t, err)
	require.Equal(t, "user replacement", string(actual))
}

func TestPrepareRejectsConflictingSiblingMCPContext(t *testing.T) {
	for _, mergeKey := range []string{"mcpServers", "mcp"} {
		t.Run(mergeKey, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "mcp.json")
			file := File{Path: path, MergeKey: mergeKey, Content: []byte(`{"` + mergeKey + `":{"kandev":{"url":"http://first/mcp"}}}`)}
			require.NoError(t, Prepare(context.Background(), workspace, workspace, "first", []File{file}))
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			other := file
			other.Content = []byte(`{"` + mergeKey + `":{"kandev":{"url":"http://second/mcp"}}}`)
			require.ErrorContains(t, Prepare(context.Background(), workspace, workspace, "second", []File{other}), "another session")
			actual, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, actual)
			require.NoError(t, Cleanup(context.Background(), workspace, "second", false))
			require.FileExists(t, path)
			require.NoError(t, Cleanup(context.Background(), workspace, "first", false))
			require.NoError(t, Prepare(context.Background(), workspace, workspace, "second", []File{other}))
		})
	}
}

func TestPrepareSharedIdenticalMCPContextCannotChangeUntilOtherOwnersRelease(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "mcp.json")
	file := File{Path: path, MergeKey: "mcpServers", Content: []byte(`{"mcpServers":{"kandev":{"url":"http://shared/mcp"}}}`)}
	require.NoError(t, Prepare(context.Background(), workspace, workspace, "first", []File{file}))
	require.NoError(t, Prepare(context.Background(), workspace, workspace, "second", []File{file}))
	file.Content = []byte(`{"mcpServers":{"kandev":{"url":"http://changed/mcp"}}}`)
	require.ErrorContains(t, Prepare(context.Background(), workspace, workspace, "first", []File{file}), "another session")
	require.NoError(t, Cleanup(context.Background(), workspace, "second", false))
	require.NoError(t, Prepare(context.Background(), workspace, workspace, "first", []File{file}))
}
