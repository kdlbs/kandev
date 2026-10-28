package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/ownershipproof"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// controlServerAdvertising stands in for a surviving agentctl control server
// that advertises the given capabilities.
func controlServerAdvertising(t *testing.T, homeDir string, capabilities []string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/identity":
			_ = json.NewEncoder(w).Encode(agentctlclient.IdentityInfo{ServerIdentity: "survivor-identity", Capabilities: capabilities})
		case r.Method == http.MethodPost && r.URL.Path == "/ownership/prove":
			var req struct {
				Challenge string `json:"challenge"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(map[string][]string{"proofs": {ownershipproof.Derive("bootstrap-credential", req.Challenge, homeDir)}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ownership/details":
			_ = json.NewEncoder(w).Encode(agentctlclient.ServerDetails{HomeDir: homeDir, DiagnosticLogPath: homeDir + "/logs/agentctl-diagnostic.log"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/rotate":
			_ = json.NewEncoder(w).Encode(agentctlclient.CredentialRotationResult{RotationID: 1, Credential: "rotated-credential"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/confirm":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// The capability set passed to control-server adoption follows the flag: an
// agentctl that predates session_mcp_token is adopted while the flag is off
// and refused while it is on, and one that advertises it is adopted either way.
func TestAdoptSurvivingAgentctlRequiredCapabilitiesFollowTheFlag(t *testing.T) {
	const homeDir = "/home/kandev-test"
	cases := []struct {
		name         string
		flag         bool
		capabilities []string
		wantAdopted  bool
	}{
		{"flag off, server lacks the capability", false, []string{"agent-survival.v1"}, true},
		{"flag on, server lacks the capability", true, []string{"agent-survival.v1"}, false},
		{"flag on, server advertises it", true, []string{"agent-survival.v1", "session_mcp_token"}, true},
		{"flag off, server advertises it", false, []string{"agent-survival.v1", "session_mcp_token"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := controlServerAdvertising(t, homeDir, tc.capabilities)
			secretStore := newIDGeneratingSecretStore()
			var seed secrets.SecretWithValue
			seed.Name, seed.Value = "seed", "bootstrap-credential"
			if err := secretStore.Create(context.Background(), &seed); err != nil {
				t.Fatalf("seed secret: %v", err)
			}
			store := &fakeControlServerStore{record: &models.ControlServerRecord{
				Endpoint: server.Listener.Addr().String(), ServerIdentity: "survivor-identity", CredentialSecretID: seed.ID,
			}}
			cfg := &config.Config{}
			cfg.HomeDir = homeDir
			cfg.Features.SessionMCPToken = tc.flag

			result, _ := adoptSurvivingAgentctl(context.Background(), context.Background(), cfg, newSurvivalTestLogger(t), store, secretStore)
			if (result != nil) != tc.wantAdopted {
				t.Fatalf("adopted = %v, want %v", result != nil, tc.wantAdopted)
			}
		})
	}
}
