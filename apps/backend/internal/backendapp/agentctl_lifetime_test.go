package backendapp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/agentctl/launcher"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ownershipproof"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

const lifetimeTestHomeDir = "/home/kandev-lifetime-test"

// lifetimeControlServer stands in for an agentctl control server and counts
// the ownership claims and health probes the backend keeps sending it.
type lifetimeControlServer struct {
	*httptest.Server
	claims atomic.Int64
	probes atomic.Int64
}

func newLifetimeControlServer(t *testing.T) *lifetimeControlServer {
	t.Helper()
	srv := &lifetimeControlServer{}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			srv.probes.Add(1)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/identity":
			_ = json.NewEncoder(w).Encode(agentctlclient.IdentityInfo{
				ServerIdentity:  "lifetime-identity",
				Capabilities:    lifecycle.RequiredSurvivalCapabilities,
				UnownedPeriodMS: 200,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/ownership/prove":
			var req struct {
				Challenge string `json:"challenge"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(map[string][]string{
				"proofs": {ownershipproof.Derive("bootstrap-credential", req.Challenge, lifetimeTestHomeDir)},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ownership/details":
			_ = json.NewEncoder(w).Encode(agentctlclient.ServerDetails{HomeDir: lifetimeTestHomeDir})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/rotate":
			_ = json.NewEncoder(w).Encode(agentctlclient.CredentialRotationResult{RotationID: 1, Credential: "rotated-credential"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/confirm":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ownership/claim":
			srv.claims.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *lifetimeControlServer) hostPort(t *testing.T) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(s.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split server address: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}
	return host, port
}

func waitForCount(t *testing.T, counter *atomic.Int64, atLeast int64, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for counter.Load() < atLeast && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := counter.Load(); got < atLeast {
		t.Fatalf("%s = %d, want >= %d", what, got, atLeast)
	}
}

// expectCountStops fails when counter keeps growing after the loops that feed
// it were supposed to have been stopped.
func expectCountStops(t *testing.T, counter *atomic.Int64, what string) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	settled := counter.Load()
	time.Sleep(300 * time.Millisecond)
	if got := counter.Load(); got != settled {
		t.Fatalf("%s kept growing after cleanup: %d -> %d", what, settled, got)
	}
}

func shortenLoopIntervals(t *testing.T) {
	t.Helper()
	prevRenewal, prevMonitor := ownershipRenewalInterval, adoptedMonitorInterval
	ownershipRenewalInterval = func(time.Duration) time.Duration { return 50 * time.Millisecond }
	adoptedMonitorInterval = 50 * time.Millisecond
	t.Cleanup(func() {
		ownershipRenewalInterval, adoptedMonitorInterval = prevRenewal, prevMonitor
	})
}

// TestProvideAgentctlLauncherAdoptedLoopsOutliveAttemptCtx drives the full
// provider for an adopted server: a runtime replacement cancels the attempt
// ctx as soon as provisioning returns, and both the ownership renewer and the
// adopted runtime monitor must keep running on the lifetime ctx until the
// result's cleanup stops them.
func TestProvideAgentctlLauncherAdoptedLoopsOutliveAttemptCtx(t *testing.T) {
	shortenLoopIntervals(t)
	srv := newLifetimeControlServer(t)

	secretStore := newIDGeneratingSecretStore()
	var seed secrets.SecretWithValue
	seed.Name = "seed"
	seed.Value = "bootstrap-credential"
	if err := secretStore.Create(context.Background(), &seed); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	store := &fakeControlServerStore{record: &models.ControlServerRecord{
		Endpoint:           srv.Listener.Addr().String(),
		ServerIdentity:     "lifetime-identity",
		CredentialSecretID: seed.ID,
	}}
	cfg := &config.Config{}
	cfg.HomeDir = lifetimeTestHomeDir
	cfg.Features.AgentSurvival = true

	var lost atomic.Int64
	lifetimeCtx, stopLifetime := context.WithCancel(context.Background())
	t.Cleanup(stopLifetime)
	attemptCtx, cancelAttempt := context.WithCancel(context.Background())
	log := newSurvivalTestLogger(t)
	result, err := provideAgentctlLauncher(attemptCtx, cfg, log, agentctlclient.NewAvailability(nil, log),
		store, secretStore, agentctlProvisionOptions{
			lifetimeCtx:   lifetimeCtx,
			onRuntimeLoss: func(uint64, string, bool) { lost.Add(1) },
		})
	cancelAttempt()
	if err != nil || result == nil || !result.adopted {
		t.Fatalf("provideAgentctlLauncher = %+v, %v; want an adopted result", result, err)
	}
	t.Cleanup(func() { _ = result.cleanup() })

	waitForCount(t, &srv.claims, 3, "ownership claims after attempt ctx cancelled")
	waitForCount(t, &srv.probes, 3, "monitor health probes after attempt ctx cancelled")
	if got := lost.Load(); got != 0 {
		t.Fatalf("runtime loss reported %d times while the server was healthy", got)
	}

	if err := result.cleanup(); err != nil {
		t.Fatalf("cleanup() = %v", err)
	}
	expectCountStops(t, &srv.claims, "ownership claims")
	expectCountStops(t, &srv.probes, "monitor health probes")
}

type stubManagedAgentctl struct {
	port int
}

func (s stubManagedAgentctl) BinaryPath() string { return "/stub/agentctl" }
func (s stubManagedAgentctl) Port() int          { return s.port }
func (s stubManagedAgentctl) Pid() int           { return 0 }
func (s stubManagedAgentctl) ProcessIdentity() processidentity.Identity {
	return processidentity.Identity{}
}
func (s stubManagedAgentctl) AuthToken() string { return "fresh-credential" }

// TestProvideAgentctlLauncherFreshSpawnRenewalOutlivesAttemptCtx covers the
// fresh-spawn path with a stubbed launcher: the renewer started for a newly
// spawned server must run on the lifetime ctx, not the attempt ctx, and stop
// when the result's cleanup runs.
func TestProvideAgentctlLauncherFreshSpawnRenewalOutlivesAttemptCtx(t *testing.T) {
	shortenLoopIntervals(t)
	srv := newLifetimeControlServer(t)
	host, port := srv.hostPort(t)

	prevStart := startManagedAgentctl
	var stopped atomic.Bool
	startManagedAgentctl = func(context.Context, launcher.Config, *logger.Logger) (managedAgentctl, func() error, error) {
		return stubManagedAgentctl{port: port}, func() error { stopped.Store(true); return nil }, nil
	}
	t.Cleanup(func() { startManagedAgentctl = prevStart })

	cfg := &config.Config{}
	cfg.HomeDir = lifetimeTestHomeDir
	cfg.Features.AgentSurvival = true
	cfg.Agent.StandaloneHost = host
	cfg.Agent.StandalonePort = port

	lifetimeCtx, stopLifetime := context.WithCancel(context.Background())
	t.Cleanup(stopLifetime)
	attemptCtx, cancelAttempt := context.WithCancel(context.Background())
	log := newSurvivalTestLogger(t)
	result, err := provideAgentctlLauncher(attemptCtx, cfg, log, agentctlclient.NewAvailability(nil, log),
		&fakeControlServerStore{}, newIDGeneratingSecretStore(), agentctlProvisionOptions{lifetimeCtx: lifetimeCtx})
	cancelAttempt()
	if err != nil || result == nil || result.adopted {
		t.Fatalf("provideAgentctlLauncher = %+v, %v; want a fresh result", result, err)
	}
	t.Cleanup(func() { _ = result.cleanup() })

	waitForCount(t, &srv.claims, 3, "ownership claims after attempt ctx cancelled")

	if err := result.cleanup(); err != nil {
		t.Fatalf("cleanup() = %v", err)
	}
	if !stopped.Load() {
		t.Fatal("cleanup did not stop the launched agentctl")
	}
	expectCountStops(t, &srv.claims, "ownership claims")
}
