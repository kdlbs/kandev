package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/stretchr/testify/require"
)

func TestExecuteProfileModelPromptRunsTheGivenModel(t *testing.T) {
	for _, tc := range []struct{ name, profileModel, model, want string }{
		{name: "explicit model replaces an empty profile model", model: "checked-model", want: "checked-model"},
		{name: "explicit model replaces the profile model", profileModel: "profile-model", model: "checked-model", want: "checked-model"},
		{name: "empty model keeps the profile model", profileModel: "profile-model", want: "profile-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := newTestLogger(t)
			reg := registry.NewRegistry(log)
			const agentType = "codex-acp"
			require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

			var received agentctlutil.PromptRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/health":
					w.WriteHeader(http.StatusOK)
				case "/api/v1/inference/prompt":
					require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.PromptResponse{Success: true, Response: "ok"}))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			host, port := serverHostPort(t, server)

			mgr := NewManager(reg, host, port, nil, log)
			mgr.SetProfileResolver(profilePromptResolver{profile: &settingsmodels.AgentProfile{
				ID: "profile-1", AgentID: agentType, Model: tc.profileModel,
			}})
			mgr.instances[agentType] = &instance{
				agentType: agentType,
				workDir:   t.TempDir(),
				client:    agentctlclient.NewClient(host, port, log),
			}

			_, err := mgr.ExecuteProfileModelPrompt(context.Background(), "profile-1", tc.model, "hello")
			require.NoError(t, err)
			require.Equal(t, tc.want, received.Model)
		})
	}
}
