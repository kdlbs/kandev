package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/sessionmcp"
)

func TestWithCodexSessionTokenSetsVariableOnlyForMarkedCommand(t *testing.T) {
	token := config.NewSessionMCPToken("tok-1")
	marked := []string{"codex", "-c", sessionmcp.CodexTokenArg}
	env := map[string]string{"A": "1", sessionmcp.CodexTokenEnv: "profile-value"}

	got := withCodexSessionToken(marked, env, token)
	if got[sessionmcp.CodexTokenEnv] != "Bearer tok-1" || got["A"] != "1" {
		t.Fatalf("env = %v", got)
	}
	if env[sessionmcp.CodexTokenEnv] != "profile-value" {
		t.Fatal("input env must not be mutated")
	}
	if out := withCodexSessionToken([]string{"codex"}, env, token); out[sessionmcp.CodexTokenEnv] != "profile-value" {
		t.Fatalf("unmarked command must keep env: %v", out)
	}
	if out := withCodexSessionToken(marked, env, config.NewSessionMCPToken("")); out[sessionmcp.CodexTokenEnv] != "profile-value" {
		t.Fatalf("instance without a token must keep env: %v", out)
	}
}
