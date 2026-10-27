package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/sessionmcp"
)

func carrierServers() []types.McpServer {
	return []types.McpServer{
		{Name: kandevMCPServerName, Type: "http", URL: "http://127.0.0.1:1/mcp"},
		{Name: "other", Type: "http", URL: "http://example.invalid/mcp"},
	}
}

func carrierExecution(nonce string) *AgentExecution {
	e := &AgentExecution{ID: "exec-1"}
	if nonce != "" {
		e.setMetadataValue(MetadataKeyMCPTokenNonce, nonce)
	}
	return e
}

type unknownStrategy struct{ mcpconfig.ClaudeStrategy }

func TestWithSessionMCPAuthCarrierClasses(t *testing.T) {
	tui := agents.NewTUIAgent(agents.TUIAgentConfig{AgentID: "custom-tui", AgentName: "Custom", Command: "tool", MCPStrategy: mcpconfig.ClaudeStrategy{}})
	cases := []struct {
		name     string
		agent    agents.Agent
		strategy mcpconfig.PassthroughMCPStrategy
		cause    string // empty means the token is carried
	}{
		{"claude", nil, mcpconfig.ClaudeStrategy{}, ""},
		{"opencode", nil, mcpconfig.OpenCodeStrategy{}, ""},
		{"codex", nil, mcpconfig.CodexStrategy{}, ""},
		{"cursor", nil, mcpconfig.CursorStrategy{}, CauseWorkspaceFileCarrierDeferred},
		{"pi", nil, mcpconfig.PiStrategy{}, CauseWorkspaceFileCarrierDeferred},
		{"custom tui whatever the strategy", tui, mcpconfig.ClaudeStrategy{}, CauseCustomTUIDeferred},
		{"unverified strategy", nil, unknownStrategy{}, CauseAgentNotVerified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newLaunchManager(&keyedStore{key: launchKey()})
			in := carrierServers()
			out, err := m.withSessionMCPAuth(t.Context(), carrierExecution("n1"), tc.agent, tc.strategy, in)
			if tc.cause != "" {
				var refusal *LaunchRefusal
				if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenCarrierUnavailable || refusal.Cause != tc.cause {
					t.Fatalf("err = %v, want carrier_unavailable cause=%s", err, tc.cause)
				}
				if !strings.HasPrefix(err.Error(), CodeMCPTokenCarrierUnavailable+": ") {
					t.Fatalf("text must start with the code: %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "Bearer " + sessionmcp.Token(launchKey(), "exec-1", "n1")
			if got := out[0].Headers["Authorization"]; got != want {
				t.Fatalf("kandev entry Authorization = %q, want %q", got, want)
			}
			if len(out[1].Headers) != 0 {
				t.Fatalf("operator entry must not receive the token: %v", out[1].Headers)
			}
			if len(in[0].Headers) != 0 {
				t.Fatalf("input list must stay token free: %v", in[0].Headers)
			}
		})
	}
}

func TestWithSessionMCPAuthWithoutKeyStoreLeavesServersUnchanged(t *testing.T) {
	m := newLaunchManager(nil)
	out, err := m.withSessionMCPAuth(t.Context(), carrierExecution(""), nil, mcpconfig.CursorStrategy{}, carrierServers())
	if err != nil || len(out[0].Headers) != 0 {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestWithSessionMCPAuthKeyFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		store *keyedStore
		nonce string
	}{
		"store failure": {&keyedStore{err: errors.New("disk")}, "n1"},
		"missing nonce": {&keyedStore{key: launchKey()}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			m := newLaunchManager(tc.store)
			_, err := m.withSessionMCPAuth(t.Context(), carrierExecution(tc.nonce), nil, mcpconfig.ClaudeStrategy{}, carrierServers())
			var refusal *LaunchRefusal
			if !errors.As(err, &refusal) || refusal.Code != CodeMCPTokenKeyUnavailable {
				t.Fatalf("err = %v, want %s", err, CodeMCPTokenKeyUnavailable)
			}
		})
	}
}

func TestWithCodexSessionTokenEnv(t *testing.T) {
	m := newLaunchManager(&keyedStore{key: launchKey()})
	exec := carrierExecution("n1")
	marked := process.InteractiveStartRequest{Command: []string{"codex", "-c", sessionmcp.CodexTokenArg}, Env: map[string]string{sessionmcp.CodexTokenEnv: "profile"}}

	got, err := m.withCodexSessionTokenEnv(t.Context(), exec, marked)
	if err != nil {
		t.Fatal(err)
	}
	want := "Bearer " + sessionmcp.Token(launchKey(), "exec-1", "n1")
	if got.Env[sessionmcp.CodexTokenEnv] != want {
		t.Fatalf("env = %v", got.Env)
	}
	if marked.Env[sessionmcp.CodexTokenEnv] != "profile" {
		t.Fatal("request env must not be mutated")
	}
	if _, ok := exec.metadataValue("passthrough_mcp_env"); ok {
		t.Fatal("token variable must not be recorded in execution metadata")
	}
	plain, _ := m.withCodexSessionTokenEnv(t.Context(), exec, process.InteractiveStartRequest{Command: []string{"claude"}})
	if _, ok := plain.Env[sessionmcp.CodexTokenEnv]; ok {
		t.Fatal("unmarked command must not receive the variable")
	}
}
