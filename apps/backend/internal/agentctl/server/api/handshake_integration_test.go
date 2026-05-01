package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestHandshakeIntegration_FullFlow exercises the complete client↔server
// handshake flow: nonce-based auth → token retrieval → authenticated requests.
// No Docker, no agent binaries — runs in CI as a regular test.
func TestHandshakeIntegration_FullFlow(t *testing.T) {
	// 1. Set up a ControlServer with a bootstrap nonce (simulating Docker/Sprites mode)
	cfg := &config.Config{
		AuthToken:      "test-generated-token-xyz",
		BootstrapNonce: "integration-test-nonce-abc123",
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	cs := NewControlServer(cfg, &instance.Manager{}, log)

	server := httptest.NewServer(cs.Router())
	defer server.Close()

	// Parse host:port from test server URL
	host, port := parseHostPort(t, server.URL)

	// 2. Create a ControlClient with NO auth token (simulating fresh backend)
	client := agentctl.NewControlClient(host, port, log)

	ctx := context.Background()

	// 3. Health check should work without auth
	if err := client.Health(ctx); err != nil {
		t.Fatalf("health check should work without auth: %v", err)
	}

	// 4. Authenticated endpoint should be REJECTED (no token yet)
	_, err := client.ListInstances(ctx)
	if err == nil {
		t.Fatal("expected error for unauthenticated request, got nil")
	}

	// 5. Handshake with correct nonce — should return token
	token, err := client.Handshake(ctx, "integration-test-nonce-abc123")
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}
	if token == "" {
		t.Fatal("handshake returned empty token")
	}

	// 6. After handshake, authenticated requests should WORK
	//    (ListInstances may return an endpoint/client error, but it should
	//     NOT be a 401 — it should get past auth.)
	_, err = client.ListInstances(ctx)
	if err != nil {
		// If the error mentions "unauthorized" or "auth", auth is still failing
		if containsAuthError(err.Error()) {
			t.Fatalf("request after handshake still rejected by auth: %v", err)
		}
		// Any other error (e.g., nil instMgr panic recovery) is fine —
		// it means auth passed and we hit the handler
	}

	// 7. Second handshake with same nonce should FAIL (burned)
	_, err = client.Handshake(ctx, "integration-test-nonce-abc123")
	if err == nil {
		t.Fatal("second handshake should fail (nonce burned)")
	}

	// 8. Handshake with wrong nonce should also fail
	_, err = client.Handshake(ctx, "wrong-nonce")
	if err == nil {
		t.Fatal("handshake with wrong nonce should fail")
	}
}

// TestHandshakeIntegration_StandaloneMode verifies that when AuthToken is set
// directly (standalone mode, no nonce), normal Bearer auth works.
func TestHandshakeIntegration_StandaloneMode(t *testing.T) {
	cfg := &config.Config{
		AuthToken: "standalone-secret-token",
		// No BootstrapNonce — standalone mode
	}
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	cs := NewControlServer(cfg, &instance.Manager{}, log)

	server := httptest.NewServer(cs.Router())
	defer server.Close()

	host, port := parseHostPort(t, server.URL)
	ctx := context.Background()

	// Without auth token: rejected
	noAuthClient := agentctl.NewControlClient(host, port, log)
	_, err := noAuthClient.ListInstances(ctx)
	if err == nil {
		t.Fatal("expected rejection without auth token")
	}

	// With correct auth token: passes auth
	authClient := agentctl.NewControlClient(host, port, log,
		agentctl.WithControlAuthToken("standalone-secret-token"))
	_, err = authClient.ListInstances(ctx)
	if err != nil && containsAuthError(err.Error()) {
		t.Fatalf("request with correct token rejected: %v", err)
	}

	// Handshake should fail (no nonce configured)
	_, err = noAuthClient.Handshake(ctx, "any-nonce")
	if err == nil {
		t.Fatal("handshake should fail when no nonce is configured")
	}
}

func parseHostPort(t *testing.T, url string) (string, int) {
	t.Helper()
	var host string
	var port int
	// httptest URLs look like "http://127.0.0.1:PORT"
	n, err := fmt.Sscanf(url, "http://%s", &host)
	if err != nil || n != 1 {
		t.Fatalf("failed to parse test server URL %q", url)
	}
	// Split host:port
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			_, _ = fmt.Sscanf(host[i+1:], "%d", &port)
			host = host[:i]
			break
		}
	}
	return host, port
}

func containsAuthError(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "unauthorized") || strings.Contains(lower, "status 401")
}
