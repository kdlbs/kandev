package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestProviderReplacementRejectsStaleControlCatalog(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: "control", ProviderMCP: &config.ProviderMCPConfig{}}
	mgr := process.NewManager(cfg, logger.Default())
	defer func() { _ = mgr.StopForTeardown(context.Background()) }()
	s := NewServer(cfg, mgr, nil, nil, logger.Default())
	for _, header := range []string{"", "1"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer control")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(types.ProviderMCPReplacementHeader, header)
		response := httptest.NewRecorder()
		s.Router().ServeHTTP(response, req)
		if header == "" {
			require.Equal(t, http.StatusConflict, response.Code)
			require.Contains(t, response.Body.String(), "PROVIDER_MCP_PROTOCOL_REQUIRED")
		} else {
			require.NotContains(t, response.Body.String(), "PROVIDER_MCP_PROTOCOL_REQUIRED")
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shell/status", nil)
	req.Header.Set("Authorization", "Bearer control")
	response := httptest.NewRecorder()
	s.Router().ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code, "workspace clients must remain independent of provider protocol adoption")
}
