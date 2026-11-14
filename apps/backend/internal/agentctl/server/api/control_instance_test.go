package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
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

func TestCreateInstanceRejectsInstallContextWithoutControlAuth(t *testing.T) {
	mgr := instance.NewManager(&config.Config{
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, logger.Default())
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	server := NewControlServer(&config.Config{}, mgr, logger.Default())
	body, err := json.Marshal(instance.CreateRequest{
		WorkspacePath: t.TempDir(),
		ManagedRuntimeInstallContext: &agentctltypes.ManagedRuntimeInstallContext{
			InstallerEnvDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			OwnerScope:         "task:owner-a",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Router().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("create with unauthenticated install context = %d (%s), want 403", resp.Code, resp.Body.String())
	}
}
