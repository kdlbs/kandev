package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/stretchr/testify/require"
)

func prepareRemoteFile(t *testing.T, srv *Server, path, content string, merge bool) {
	t.Helper()
	file := map[string]any{"path": path, "content": []byte(content)}
	if merge {
		file["merge_key"] = "mcpServers"
	}
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/prepare", map[string]any{"generation": 1, "files": []any{file}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestRemotePassthroughConfigOwnership(t *testing.T) {
	first := newPassthroughAPIServer(t)
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: first.cfg.WorkDir, SessionID: "session-2", InstanceID: "instance-2"}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
	second := NewServer(cfg, manager, nil, nil, log)
	path := filepath.Join(first.cfg.WorkDir, ".cursor", "mcp.json")
	content := `{"mcpServers":{"kandev":{"url":"http://first"}}}`
	prepareRemoteFile(t, first, path, content, true)
	prepareRemoteFile(t, second, path, content, true)
	conflicting := map[string]any{"path": path, "content": []byte(`{"mcpServers":{"kandev":{"url":"http://second"}}}`), "merge_key": "mcpServers"}
	rejected := shellTerminalRequest(t, second, http.MethodPost, "/api/v1/passthrough/prepare", map[string]any{"generation": 1, "files": []any{conflicting}})
	require.Equal(t, http.StatusBadRequest, rejected.Code, rejected.Body.String())
	require.Contains(t, rejected.Body.String(), "another session")
	rec := shellTerminalRequest(t, first, http.MethodPost, "/api/v1/passthrough/cleanup", map[string]any{"generation": 1})
	require.Equal(t, http.StatusOK, rec.Code)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "first")
	require.NotContains(t, string(data), "http://second")
	rec = shellTerminalRequest(t, second, http.MethodPost, "/api/v1/passthrough/cleanup", map[string]any{"generation": 1})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoFileExists(t, path)
}

func TestRemotePassthroughConfigPreservesUserFile(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	path := filepath.Join(srv.cfg.WorkDir, ".cursor", "mcp.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"mcpServers":{"user":{"url":"http://user"}}}`), 0o600))
	prepareRemoteFile(t, srv, path, `{"mcpServers":{"kandev":{"url":"http://local"}}}`, true)
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/cleanup", map[string]any{"generation": 1})
	require.Equal(t, http.StatusOK, rec.Code)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "user")
}

func TestRemotePassthroughConfigSymlinks(t *testing.T) {
	srv := newPassthroughAPIServer(t)
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(srv.cfg.WorkDir, ".cursor")))
	file := map[string]any{"path": filepath.Join(srv.cfg.WorkDir, ".cursor", "mcp.json"), "content": []byte(`{}`), "merge_key": "mcpServers"}
	rec := shellTerminalRequest(t, srv, http.MethodPost, "/api/v1/passthrough/prepare", map[string]any{"generation": 1, "files": []any{file}})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoFileExists(t, filepath.Join(outside, "mcp.json"))
}
