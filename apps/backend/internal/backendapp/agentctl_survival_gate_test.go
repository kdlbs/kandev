package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/ownershipproof"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	gateTestHomeDir    = "/home/kandev-gate-test"
	gateSeedCredential = "bootstrap-credential"
)

// recordingControlServer stands in for a control server left running by a
// prior launch and records which ownership operation this launch performed
// against it. Adoption rotates the credential; reclamation shuts the server
// down. The two are mutually exclusive, so which one arrives is the whole
// observation.
type recordingControlServer struct {
	*httptest.Server
	mu         sync.Mutex
	rotated    bool
	shutDown   bool
	identity   string
	challenges []string
}

func newRecordingControlServer(t *testing.T) *recordingControlServer {
	t.Helper()
	rec := &recordingControlServer{identity: "gate-survivor-identity"}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/identity":
			_ = json.NewEncoder(w).Encode(agentctlclient.IdentityInfo{
				ServerIdentity: rec.identity,
				Capabilities:   lifecycle.RequiredSurvivalCapabilities,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/ownership/prove":
			var req struct {
				Challenge string `json:"challenge"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			rec.mu.Lock()
			rec.challenges = append(rec.challenges, req.Challenge)
			rec.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string][]string{
				"proofs": {ownershipproof.Derive(gateSeedCredential, req.Challenge, gateTestHomeDir)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ownership/details":
			_ = json.NewEncoder(w).Encode(agentctlclient.ServerDetails{HomeDir: gateTestHomeDir})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/rotate":
			rec.mu.Lock()
			rec.rotated = true
			rec.mu.Unlock()
			_ = json.NewEncoder(w).Encode(agentctlclient.CredentialRotationResult{
				RotationID: 1,
				Credential: "gate-rotated-credential",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/confirm":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/shutdown":
			rec.mu.Lock()
			rec.shutDown = true
			rec.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recordingControlServer) observed() (rotated, shutDown bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rotated, r.shutDown
}

// newGateFixture wires a recorded surviving control server to a store and a
// secret store holding the credential that server was launched with.
func newGateFixture(t *testing.T) (*recordingControlServer, *fakeControlServerStore, secrets.SecretStore, *config.Config) {
	t.Helper()
	server := newRecordingControlServer(t)

	secretStore := newIDGeneratingSecretStore()
	seed := secrets.SecretWithValue{}
	seed.Name = "gate-seed"
	seed.Value = gateSeedCredential
	if err := secretStore.Create(context.Background(), &seed); err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	store := &fakeControlServerStore{record: &models.ControlServerRecord{
		Endpoint:           server.Listener.Addr().String(),
		ServerIdentity:     server.identity,
		CredentialSecretID: seed.ID,
	}}

	cfg := &config.Config{}
	cfg.HomeDir = gateTestHomeDir
	return server, store, secretStore, cfg
}

// TestResolveSurvivingAgentctlAdoptsWhenSurvivalEnabled pins the open side of
// the capability gate: with the capability on, a recorded server that proves
// its identity is adopted, and this launch takes it over by rotating its
// credential rather than starting a second one.
func TestResolveSurvivingAgentctlAdoptsWhenSurvivalEnabled(t *testing.T) {
	server, store, secretStore, cfg := newGateFixture(t)
	cfg.Features.AgentSurvival = true

	result, _ := resolveSurvivingAgentctl(context.Background(), cfg, newSurvivalTestLogger(t), store, secretStore)
	if result == nil {
		t.Fatal("result = nil, want the recorded server to be adopted")
	}
	t.Cleanup(func() { _ = result.cleanup() })

	rotated, shutDown := server.observed()
	if !rotated {
		t.Fatal("the adopted server's credential was never rotated")
	}
	if shutDown {
		t.Fatal("the adopted server was shut down")
	}
	if cfg.Agent.StandaloneAuthToken != "gate-rotated-credential" {
		t.Fatalf("StandaloneAuthToken = %q, want the rotated credential", cfg.Agent.StandaloneAuthToken)
	}
}

// TestResolveSurvivingAgentctlReclaimsDetachedServerWhenSurvivalDisabled pins
// the closed side. With the capability off, nothing may be adopted -- but a
// server a previous survival-enabled launch detached is still running with no
// backend attached to it, so this launch stops it instead of leaving it to
// time out on its own, and then falls through to a fresh spawn.
func TestResolveSurvivingAgentctlReclaimsDetachedServerWhenSurvivalDisabled(t *testing.T) {
	server, store, secretStore, cfg := newGateFixture(t)
	cfg.Features.AgentSurvival = false

	result, _ := resolveSurvivingAgentctl(context.Background(), cfg, newSurvivalTestLogger(t), store, secretStore)
	if result != nil {
		t.Fatalf("result = %+v, want nil: nothing may be adopted with the capability disabled", result)
	}

	rotated, shutDown := server.observed()
	if rotated {
		t.Fatal("an adoption rotation was attempted with the capability disabled")
	}
	if !shutDown {
		t.Fatal("the detached control server was left running with the capability disabled")
	}
	if cfg.Agent.StandaloneAuthToken != "" {
		t.Fatalf("StandaloneAuthToken = %q, want empty: no server was adopted", cfg.Agent.StandaloneAuthToken)
	}
}
