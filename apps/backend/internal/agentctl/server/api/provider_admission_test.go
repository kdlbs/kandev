package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestMCPProviderReplacementRejectsNewProviderRequests(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: "control"}
	mgr := process.NewManager(cfg, log)
	defer func() { _ = mgr.StopForTeardown(context.Background()) }()
	server := NewServer(cfg, mgr, nil, nil, log)
	require.NoError(t, mgr.ReserveProviderReplacement("operation", time.Now().Add(time.Minute)))
	for _, path := range []string{"/api/v1/start", "/api/v1/agent/configure", "/api/v1/agent/submissions"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer control")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, req)
		require.Equal(t, http.StatusConflict, response.Code, path+": "+response.Body.String())
	}
	message, err := ws.NewRequest("prompt", "agent.prompt", map[string]any{"message": "held input"})
	require.NoError(t, err)
	response := server.handleAgentStreamRequest(context.Background(), message)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Contains(t, string(response.Payload), "provider_replacement_pending")
	_, release, err := mgr.BeginOwnedOperation(context.Background())
	require.NoError(t, err)
	release()
}
