package acp

import (
	"context"
	"errors"
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
)

func TestAntigravitySessionAuthentication(t *testing.T) {
	a := &Adapter{
		agentID: acpcompat.AntigravityAgentID,
		cfg: &shared.Config{AuthPolicy: &acpcompat.AuthPolicy{
			MethodID:  acpcompat.AntigravityPersonalAuthMethod,
			AttemptID: "attempt-1",
		}},
	}
	if err := a.Authenticate(context.Background(), "other-method"); err == nil {
		t.Fatal("unsupported auth method was accepted")
	}
	empty := &Adapter{agentID: acpcompat.AntigravityAgentID, cfg: &shared.Config{}}
	if err := empty.Authenticate(context.Background(), acpcompat.AntigravityPersonalAuthMethod); err == nil {
		t.Fatal("authenticate without a connection succeeded")
	}
}

func TestAntigravityCLIAuthenticationIsDeniedBeforeACP(t *testing.T) {
	a := &Adapter{
		agentID: acpcompat.AntigravityAgentID,
		cfg: &shared.Config{AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
			acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
		)},
	}
	if err := a.Authenticate(context.Background(), acpcompat.AntigravityPersonalAuthMethod); !errors.Is(err, acpcompat.ErrTaskAuthenticationDenied) {
		t.Fatalf("CLI task authentication error = %v, want task-authentication denial before ACP", err)
	}
}

func TestAntigravityBackgroundConsent(t *testing.T) {
	a := &Adapter{
		agentID: acpcompat.AntigravityAgentID,
		cfg: &shared.Config{AuthPolicy: &acpcompat.AuthPolicy{
			MethodID:  acpcompat.AntigravityPersonalAuthMethod,
			AttemptID: "attempt-1",
		}},
	}
	line := "Open https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&code_challenge=c&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb"
	a.ConsumeStderrLine(line)
	status := a.AuthStatus()
	if status.AuthorizationURL == "" || status.State != "waiting_for_browser" {
		t.Fatalf("status = %+v", status)
	}
	if _, keep := a.SanitizeStderrLine(line); !keep {
		t.Fatal("consent line was dropped without a redacted marker")
	}
}

func TestAntigravityCLIUsesDefaultWireIdentityAndCachedAuth(t *testing.T) {
	policy := acpcompat.AntigravityAuthPolicyForRuntime(acpcompat.AntigravityRuntimeCLI, "1.5.15", false)
	if policy.MethodID != "" {
		t.Fatalf("CLI auth method = %q, want no standalone auth method", policy.MethodID)
	}
	if info := clientInfoForAgent(acpcompat.AntigravityAgentID, policy); info.Name != "kandev-agentctl" {
		t.Fatalf("CLI clientInfo = %+v, want default agentctl identity", info)
	}
	a := &Adapter{agentID: acpcompat.AntigravityAgentID, cfg: &shared.Config{AuthPolicy: policy}}
	if err := a.applyAntigravityPersonalAuth(context.Background(), acp.InitializeResponse{}); err != nil {
		t.Fatalf("CLI attempted standalone auth: %v", err)
	}
}
