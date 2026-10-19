package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/stretchr/testify/require"
)

func TestRemotePassthroughStoppedGenerationReleasesOnlyItsClaims(t *testing.T) {
	for _, sibling := range []bool{false, true} {
		t.Run(strconv.FormatBool(sibling), func(t *testing.T) {
			first := newPassthroughAPIServer(t)
			path := filepath.Join(first.cfg.WorkDir, ".cursor", "mcp.json")
			before := `{"mcpServers":{"profile":{"command":"before"}}}`
			prepareRemoteFile(t, first, path, before, true)
			var second *Server
			if sibling {
				log := newTestLogger()
				cfg := &config.InstanceConfig{WorkDir: first.cfg.WorkDir, SessionID: "session-2", InstanceID: "instance-2"}
				manager := process.NewManager(cfg, log)
				t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
				second = NewServer(cfg, manager, nil, nil, log)
				prepareRemoteFile(t, second, path, before, true)
			}
			snapshot := remoteStart(t, first, "cat")
			file := map[string]any{"path": path, "content": []byte(`{"mcpServers":{"profile":{"command":"after"}}}`), "merge_key": "mcpServers"}
			request := map[string]any{"generation": 2, "files": []any{file}}
			live := shellTerminalRequest(t, first, http.MethodPost, "/api/v1/passthrough/prepare", request)
			require.Equal(t, http.StatusBadRequest, live.Code, live.Body.String())
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.JSONEq(t, before, string(data))
			stop := shellTerminalRequest(t, first, http.MethodDelete, "/api/v1/passthrough/"+snapshot["process_id"].(string)+"?generation=1", nil)
			require.Equal(t, http.StatusOK, stop.Code, stop.Body.String())
			stale := shellTerminalRequest(t, first, http.MethodPost, "/api/v1/passthrough/prepare", map[string]any{"generation": 1, "files": []any{file}})
			require.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
			data, err = os.ReadFile(path)
			require.NoError(t, err)
			require.JSONEq(t, before, string(data))
			replacement := shellTerminalRequest(t, first, http.MethodPost, "/api/v1/passthrough/prepare", request)
			if sibling {
				require.Equal(t, http.StatusBadRequest, replacement.Code, replacement.Body.String())
				data, err = os.ReadFile(path)
				require.NoError(t, err)
				require.JSONEq(t, before, string(data))
				cleanup := shellTerminalRequest(t, second, http.MethodPost, "/api/v1/passthrough/cleanup", map[string]any{"generation": 1})
				require.Equal(t, http.StatusOK, cleanup.Code, cleanup.Body.String())
				replacement = shellTerminalRequest(t, first, http.MethodPost, "/api/v1/passthrough/prepare", request)
			}
			require.Equal(t, http.StatusOK, replacement.Code, replacement.Body.String())
			cleanup := shellTerminalRequest(t, first, http.MethodPost, "/api/v1/passthrough/cleanup", map[string]any{"generation": 1})
			require.Equal(t, http.StatusOK, cleanup.Code, cleanup.Body.String())
			data, err = os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), "after")
			require.NotContains(t, string(data), "before")
		})
	}
}
