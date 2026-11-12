package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
)

func newProjectMCPFilesTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	workspace := t.TempDir()
	manager := NewManager(&config.InstanceConfig{
		InstanceID: "instance-1",
		SessionID:  "session-1",
		Port:       43210,
		WorkDir:    workspace,
	}, newTestLogger(t))
	t.Cleanup(func() { manager.lifetimeCancel() })
	return manager, workspace
}

func projectMCPTestFile(t *testing.T, workspace, url string) passthroughconfig.File {
	t.Helper()
	content, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"kandev": map[string]any{"transport": "streamable-http", "url": url, "lifecycle": "eager"},
		},
	})
	require.NoError(t, err)
	return passthroughconfig.File{
		Path: filepath.Join(workspace, ".pi", "mcp.json"), Content: content, MergeKey: "mcpServers",
	}
}

// @covers AC-AGENTS-MCP-PREP-005.1, AC-AGENTS-MCP-PREP-005.2
func TestProjectMCPFilesOwnerPrepareMergeAndCleanup(t *testing.T) {
	manager, workspace := newProjectMCPFilesTestManager(t)
	owner := manager.ProjectMCPFiles()
	require.NotNil(t, owner)
	target := owner.Target()
	require.Equal(t, "instance-1", target.InstanceID)
	require.Equal(t, "session-1", target.SessionID)
	require.Equal(t, workspace, target.WorkspacePath)
	require.Equal(t, "http://127.0.0.1:43210/mcp", target.MCPURL)
	require.Zero(t, target.Generation)

	first := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp")
	response, err := owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1, Files: []passthroughconfig.File{first},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(1), response.Generation)
	require.Len(t, response.Files, 1)
	require.Len(t, response.Files[0].Fingerprint, 64)
	require.FileExists(t, first.Path)

	second := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp/packages/diagnostics")
	_, err = owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 2, Files: []passthroughconfig.File{second},
	})
	require.NoError(t, err)
	var merged struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	content, err := os.ReadFile(first.Path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &merged))
	require.Equal(t, "http://127.0.0.1:43210/mcp/packages/diagnostics", merged.MCPServers["kandev"]["url"])
	require.Equal(t, "eager", merged.MCPServers["kandev"]["lifecycle"])
	require.Equal(t, "streamable-http", merged.MCPServers["kandev"]["transport"])

	require.NoError(t, owner.Cleanup(context.Background()))
	_, statErr := os.Stat(first.Path)
	require.True(t, os.IsNotExist(statErr), "an owned project file is removed with its final claim")
	require.Greater(t, owner.Target().Generation, uint64(2), "cleanup advances the generation fence")
}

func TestProjectMCPFilesOwnerPreservesUserFile(t *testing.T) {
	manager, workspace := newProjectMCPFilesTestManager(t)
	path := filepath.Join(workspace, ".pi", "mcp.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"user":{"command":"user-tool"}}}`), 0o600))

	file := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp")
	_, err := manager.ProjectMCPFiles().Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1, Files: []passthroughconfig.File{file},
	})
	require.NoError(t, err)
	var merged struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &merged))
	require.Equal(t, "user-tool", merged.MCPServers["user"]["command"])
	require.Equal(t, "http://127.0.0.1:43210/mcp", merged.MCPServers["kandev"]["url"])

	require.NoError(t, manager.ProjectMCPFiles().Cleanup(context.Background()))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "user-tool", "a user-owned file survives cleanup")
}

func TestProjectMCPFilesOwnerMapsOwnershipConflict(t *testing.T) {
	manager, workspace := newProjectMCPFilesTestManager(t)
	path := filepath.Join(workspace, ".pi", "mcp.json")
	content, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{"kandev": map[string]any{"url": "http://other.example/mcp"}},
	})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, content, 0o600))
	require.NoError(t, passthroughconfig.Prepare(context.Background(), workspace, filepath.Join(workspace, ".kandev", "private"),
		"other-execution:session:project", []passthroughconfig.File{{Path: path, Content: content, MergeKey: "mcpServers"}}))

	file := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp")
	_, err = manager.ProjectMCPFiles().Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1, Files: []passthroughconfig.File{file},
	})
	require.ErrorIs(t, err, ErrProjectMCPConflict)
}

func TestProjectMCPFilesOwnerVerifiesReadBackEntries(t *testing.T) {
	manager, workspace := newProjectMCPFilesTestManager(t)
	path := filepath.Join(workspace, ".pi", "mcp.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"kandev":{"url":"http://stale.example/mcp"}}}`), 0o600))
	requested := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp")
	_, err := manager.ProjectMCPFiles().readBack([]passthroughconfig.File{requested})
	require.ErrorContains(t, err, "does not contain the requested entries")
}

func TestProjectMCPFilesOwnerFencesGenerationAcrossCleanup(t *testing.T) {
	manager, workspace := newProjectMCPFilesTestManager(t)
	owner := manager.ProjectMCPFiles()
	file := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp")
	_, err := owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1, Files: []passthroughconfig.File{file},
	})
	require.NoError(t, err)
	require.NoError(t, owner.Cleanup(context.Background()))
	fenced := owner.Target().Generation
	require.Equal(t, uint64(2), fenced)
	_, err = owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1, Files: []passthroughconfig.File{file},
	})
	require.ErrorIs(t, err, ErrProjectMCPConflict, "a stale generation must not reinstall after cleanup")
	_, err = owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: fenced + 1, Files: []passthroughconfig.File{file},
	})
	require.NoError(t, err)
}

func TestProjectMCPFilesOwnerRejectsStaleGenerationAndEscape(t *testing.T) {
	manager, workspace := newProjectMCPFilesTestManager(t)
	owner := manager.ProjectMCPFiles()
	file := projectMCPTestFile(t, workspace, "http://127.0.0.1:43210/mcp")
	for _, generation := range []uint64{0, 2, 7} {
		_, err := owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
			Version: types.ProjectMCPFilesVersion, Generation: generation, Files: []passthroughconfig.File{file},
		})
		require.Error(t, err, "generation %d", generation)
	}

	escape := passthroughconfig.File{Path: filepath.Join(workspace, ".pi", "..", "..", "outside.json"), Content: []byte(`{}`)}
	_, err := owner.Prepare(context.Background(), types.ProjectMCPPrepareRequest{
		Version: types.ProjectMCPFilesVersion, Generation: 1, Files: []passthroughconfig.File{escape},
	})
	require.Error(t, err)
}
