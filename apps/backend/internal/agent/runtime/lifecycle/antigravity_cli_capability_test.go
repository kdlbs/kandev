package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestAntigravityCLIStartRejectsRetainedPersonalAuthHelper(t *testing.T) {
	manager := newTestManager(t)
	configureCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"capabilities":["antigravity_personal_auth_v1"]}`))
		case "/api/v1/agent/configure":
			configureCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	execution := &AgentExecution{
		ID:            "exec-antigravity-cli",
		SessionID:     "session-antigravity-cli",
		AgentID:       acpcompat.AntigravityAgentID,
		WorkspacePath: t.TempDir(),
		AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
			acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
		),
	}
	execution.setRuntimeEnvironment(map[string]string{})
	execution.SetAgentCtlClientForTesting(agentctlclient.NewClient(host, port, newTestLogger()))
	_, err = manager.configureAndStartAgentWithPreparedArgsAndPreparation(
		context.Background(), execution, []string{"agy", "acp"}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), acpcompat.AntigravityCLIRuntimeCapability) {
		t.Fatalf("CLI start error = %v, want missing %s", err, acpcompat.AntigravityCLIRuntimeCapability)
	}
	if configureCalled {
		t.Fatal("task configuration reached a helper that lacks CLI runtime support")
	}
}

func TestAntigravityCLIStartHandlesLocalRuntimeEnvWithEmptyBaseEnv(t *testing.T) {
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID:              "exec-antigravity-cli-empty-env",
		SessionID:       "session-antigravity-cli-empty-env",
		AgentID:         acpcompat.AntigravityAgentID,
		WorkspacePath:   t.TempDir(),
		LocalRuntimeEnv: map[string]string{"PATH": t.TempDir()},
		AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
			acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
		),
	}
	execution.setRuntimeEnvironment(map[string]string{})
	_, err := manager.configureAndStartAgentWithPreparedArgsAndPreparation(
		context.Background(), execution, []string{"agy", "acp"}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "has no agentctl client") {
		t.Fatalf("CLI start error = %v, want missing agentctl client after environment setup", err)
	}
}
