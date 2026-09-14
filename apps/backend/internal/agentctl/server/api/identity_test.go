package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestHandleIdentityIsReachableWithoutAuthAndReportsIdentityAndCapabilities
// pins design 01's "Identity retrieval ... is what decides compatibility, so
// it cannot itself be gated on the answer": /identity must answer even when
// AuthToken is configured and the caller presents no bearer token at all,
// and must report this launch's opaque server identity and the advertised
// capability set.
func TestHandleIdentityIsReachableWithoutAuthAndReportsIdentityAndCapabilities(t *testing.T) {
	log := logger.Default()
	cfg := &config.Config{
		AuthToken:         "some-configured-token",
		HomeDir:           "/home/kandev-test/.kandev",
		ServerIdentity:    "server-identity-abc",
		DiagnosticLogPath: "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log",
	}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })

	cs := NewControlServer(cfg, mgr, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()

	resp, err := http.Get(server.URL + "/identity") //nolint:noctx // test-only, hits an ephemeral httptest server
	if err != nil {
		t.Fatalf("GET /identity: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (identity must not be gated by auth)", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		ServerIdentity  string   `json:"server_identity"`
		Capabilities    []string `json:"capabilities"`
		UnownedPeriodMS int64    `json:"unowned_period_ms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ServerIdentity != cfg.ServerIdentity {
		t.Errorf("ServerIdentity = %q, want %q", body.ServerIdentity, cfg.ServerIdentity)
	}
	if len(body.Capabilities) == 0 {
		t.Error("Capabilities is empty, want the advertised capability set")
	}
	// AC-EXECUTORS-CONTROL-OWNERSHIP-003.2 (Review round 3, finding 5): an
	// adopting backend must be able to renew ownership on the cadence this
	// server actually enforces, not on whatever its own local config
	// resolves to -- the two can disagree across a restart that changed
	// agentctl.unownedPeriod. /identity is the only channel that value ever
	// crosses back to an adopting backend.
	if body.UnownedPeriodMS != cs.unownedPeriod.Milliseconds() {
		t.Errorf("UnownedPeriodMS = %d, want %d (this server's own resolved unowned period)", body.UnownedPeriodMS, cs.unownedPeriod.Milliseconds())
	}
}

// TestGetIdentityRoundTripsThroughTheRealClient drives GET /identity through
// the real handler and the real agentctl.ControlClient, matching the
// end-to-end convention established for ListInstances (a hand-fabricated
// client fixture could never have caught that envelope mismatch).
func TestGetIdentityRoundTripsThroughTheRealClient(t *testing.T) {
	log := logger.Default()
	cfg := &config.Config{
		HomeDir:           "/home/kandev-test/.kandev",
		ServerIdentity:    "server-identity-xyz",
		DiagnosticLogPath: "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log",
	}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })

	cs := NewControlServer(cfg, mgr, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()
	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log)

	identity, err := client.GetIdentity(t.Context())
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if identity.ServerIdentity != cfg.ServerIdentity {
		t.Errorf("ServerIdentity = %q, want %q", identity.ServerIdentity, cfg.ServerIdentity)
	}
	if len(identity.Capabilities) == 0 {
		t.Error("Capabilities is empty, want the advertised capability set")
	}
	if identity.UnownedPeriodMS != cs.unownedPeriod.Milliseconds() {
		t.Errorf("UnownedPeriodMS = %d, want %d", identity.UnownedPeriodMS, cs.unownedPeriod.Milliseconds())
	}
}

// TestGetServerDetailsRoundTripsThroughTheRealClientAndRequiresAuth pins the
// endpoint the paths moved to. The filesystem values an adopting backend
// records are still reachable, but only once it has authenticated, so a
// caller that never proved possession of the credential learns nothing about
// this machine's layout.
func TestGetServerDetailsRoundTripsThroughTheRealClientAndRequiresAuth(t *testing.T) {
	log := logger.Default()
	cfg := &config.Config{
		AuthToken:         "details-credential",
		HomeDir:           "/home/kandev-test/.kandev",
		ServerIdentity:    "server-identity-details",
		DiagnosticLogPath: "/home/kandev-test/.kandev/logs/agentctl-diagnostic.log",
	}
	mgr := instance.NewManager(cfg, log)
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })

	cs := NewControlServer(cfg, mgr, log)
	server := httptest.NewServer(cs.Router())
	defer server.Close()

	unauthenticated, err := http.Get(server.URL + "/api/v1/ownership/details") //nolint:noctx // test-only, hits an ephemeral httptest server
	if err != nil {
		t.Fatalf("GET /api/v1/ownership/details: %v", err)
	}
	defer func() { _ = unauthenticated.Body.Close() }()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticated.StatusCode, http.StatusUnauthorized)
	}

	host, port := parseHostPort(t, server.URL)
	client := agentctl.NewControlClient(host, port, log)
	client.SetAuthToken(cfg.AuthToken)

	details, err := client.GetServerDetails(t.Context())
	if err != nil {
		t.Fatalf("GetServerDetails: %v", err)
	}
	if details.HomeDir != cfg.HomeDir {
		t.Errorf("HomeDir = %q, want %q", details.HomeDir, cfg.HomeDir)
	}
	if details.DiagnosticLogPath != cfg.DiagnosticLogPath {
		t.Errorf("DiagnosticLogPath = %q, want %q", details.DiagnosticLogPath, cfg.DiagnosticLogPath)
	}
}
