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

// TestHandleIdentityIsReachableWithoutAuthAndReportsHomeIdentityAndCapabilities
// pins design 01's "Identity retrieval ... is what decides compatibility, so
// it cannot itself be gated on the answer": /identity must answer even when
// AuthToken is configured and the caller presents no bearer token at all,
// and must report the resolved Kandev home directory, this launch's opaque
// server identity, and the advertised capability set.
func TestHandleIdentityIsReachableWithoutAuthAndReportsHomeIdentityAndCapabilities(t *testing.T) {
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
		HomeDir           string   `json:"home_dir"`
		ServerIdentity    string   `json:"server_identity"`
		Capabilities      []string `json:"capabilities"`
		DiagnosticLogPath string   `json:"diagnostic_log_path"`
		UnownedPeriodMS   int64    `json:"unowned_period_ms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.HomeDir != cfg.HomeDir {
		t.Errorf("HomeDir = %q, want %q", body.HomeDir, cfg.HomeDir)
	}
	if body.ServerIdentity != cfg.ServerIdentity {
		t.Errorf("ServerIdentity = %q, want %q", body.ServerIdentity, cfg.ServerIdentity)
	}
	if len(body.Capabilities) == 0 {
		t.Error("Capabilities is empty, want the advertised capability set")
	}
	if body.DiagnosticLogPath != cfg.DiagnosticLogPath {
		t.Errorf("DiagnosticLogPath = %q, want %q", body.DiagnosticLogPath, cfg.DiagnosticLogPath)
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
	if identity.HomeDir != cfg.HomeDir {
		t.Errorf("HomeDir = %q, want %q", identity.HomeDir, cfg.HomeDir)
	}
	if identity.ServerIdentity != cfg.ServerIdentity {
		t.Errorf("ServerIdentity = %q, want %q", identity.ServerIdentity, cfg.ServerIdentity)
	}
	if len(identity.Capabilities) == 0 {
		t.Error("Capabilities is empty, want the advertised capability set")
	}
	if identity.DiagnosticLogPath != cfg.DiagnosticLogPath {
		t.Errorf("DiagnosticLogPath = %q, want %q", identity.DiagnosticLogPath, cfg.DiagnosticLogPath)
	}
	if identity.UnownedPeriodMS != cs.unownedPeriod.Milliseconds() {
		t.Errorf("UnownedPeriodMS = %d, want %d", identity.UnownedPeriodMS, cs.unownedPeriod.Milliseconds())
	}
}
