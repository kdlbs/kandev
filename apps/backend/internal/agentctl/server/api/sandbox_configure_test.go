package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func apiSandboxFixture(t *testing.T) *sandbox.Launch {
	t.Helper()
	content := []byte("fixture runtime")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(context.Background(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	return &sandbox.Launch{Version: 1, AttemptID: "configure-attempt", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}
}

func TestSandboxConfigureRequiresControlAndPersistsAtomically(t *testing.T) {
	for _, kind := range []string{"valid", "unauthenticated", "invalid-envelope", "missing-argv"} {
		t.Run(kind, func(t *testing.T) {
			launch := apiSandboxFixture(t)
			cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AgentArgs: []string{"original"}, AuthToken: "fixture-control"}
			log := newTestLogger()
			body := map[string]any{"agent_args": []string{"mock-agent"}, "command": "mock-agent", "sandbox": launch}
			want := http.StatusOK
			switch kind {
			case "unauthenticated":
				cfg.AuthToken = ""
				want = http.StatusForbidden
			case "invalid-envelope":
				launch.PolicyDigest = "invalid"
				want = http.StatusBadRequest
			case "missing-argv":
				delete(body, "agent_args")
				want = http.StatusBadRequest
			}
			server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			if cfg.AuthToken != "" {
				req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			}
			response := httptest.NewRecorder()
			server.Router().ServeHTTP(response, req)
			require.Equal(t, want, response.Code, response.Body.String())
			if want != http.StatusOK {
				require.Equal(t, []string{"original"}, cfg.AgentArgs)
				return
			}
			retained, err := json.Marshal(cfg)
			require.NoError(t, err)
			var state struct {
				Sandbox  *sandbox.Launch `json:"sandbox"`
				Required bool            `json:"sandbox_required"`
			}
			require.NoError(t, json.Unmarshal(retained, &state))
			require.True(t, state.Required)
			require.Equal(t, launch, state.Sandbox)
			// Older callers cannot clear a selected sandbox by omitting the field.
			body = map[string]any{"agent_args": []string{"unconfined"}}
			raw, err = json.Marshal(body)
			require.NoError(t, err)
			req = httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
			response = httptest.NewRecorder()
			server.Router().ServeHTTP(response, req)
			require.GreaterOrEqual(t, response.Code, 400)
			require.Equal(t, []string{"mock-agent"}, cfg.AgentArgs)
		})
	}
}

func TestSandboxOwnershipEndpointRequiresAuthenticatedControl(t *testing.T) {
	for _, token := range []string{"", "fixture-control"} {
		cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: token}
		log := newTestLogger()
		server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
		request := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sandbox/owner", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		want := http.StatusForbidden
		if token != "" {
			want = http.StatusNotFound
		}
		require.Equal(t, want, response.Code)
		require.NotContains(t, response.Body.String(), cfg.WorkDir)
	}
}
