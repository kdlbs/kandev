package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestSandboxPersistentReceiptRequiresAuthenticatedControl(t *testing.T) {
	for _, token := range []string{"", "fixture-control"} {
		launch := apiSandboxFixture(t)
		cfg := &config.InstanceConfig{WorkDir: t.TempDir(), Protocol: agent.ProtocolACP, AuthToken: token, Sandbox: launch, AgentArgs: []string{filepath.Join(t.TempDir(), "missing")}}
		log := newTestLogger()
		manager := process.NewManager(cfg, log)
		t.Cleanup(func() { _ = manager.Stop(context.Background()) })
		_, err := manager.StartWithGeneration(t.Context())
		require.Error(t, err)
		server := NewServer(cfg, manager, nil, nil, log)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sandbox/"+launch.AttemptID+"/receipt", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		if token == "" {
			require.Equal(t, http.StatusForbidden, response.Code)
			continue
		}
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var receipt sandbox.StartReceipt
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &receipt))
		require.NoError(t, receipt.ValidateStopped(launch))
		require.Equal(t, sandbox.StartNotStarted, receipt.Outcome)
		require.Nil(t, receipt.Owner)
	}
}
