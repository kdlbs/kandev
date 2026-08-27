package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

func TestDeleteInstanceUnknownReturnsNotFound(t *testing.T) {
	mgr := instance.NewManager(&config.Config{
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, logger.Default())
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	server := NewControlServer(&config.Config{}, mgr, logger.Default())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/missing", nil)
	resp := httptest.NewRecorder()

	server.Router().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("DELETE unknown instance status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}
