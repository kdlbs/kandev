package acp

import (
	"context"
	"testing"

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
