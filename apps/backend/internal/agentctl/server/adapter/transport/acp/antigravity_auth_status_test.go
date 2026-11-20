package acp

import (
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
)

func TestAntigravityAuthStatusWhilePending(t *testing.T) {
	a := &Adapter{
		agentID: acpcompat.AntigravityAgentID,
		cfg: &shared.Config{
			AuthPolicy: &acpcompat.AuthPolicy{
				MethodID:         acpcompat.AntigravityPersonalAuthMethod,
				AllowInteractive: true,
				AttemptID:        "attempt-1",
			},
		},
	}
	line := "Open https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&code_challenge=c&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb"
	a.ConsumeStderrLine(line)
	status := a.AuthStatus()
	if status.AttemptID != "attempt-1" || status.AuthorizationURL == "" {
		t.Fatalf("status = %+v", status)
	}
	safe, keep := a.SanitizeStderrLine(line)
	if !keep || strings.Contains(safe, "accounts.google.com") {
		t.Fatalf("sanitized line = %q, keep=%v", safe, keep)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			a.ConsumeStderrLine(line)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = a.AuthStatus()
		}
	}()
	wg.Wait()
}
