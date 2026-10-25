package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/sessionmcp"
)

// A sandboxed execution reaches Kandev MCP only through its private relay, so
// the instance token is neither required nor written into its configuration,
// including for agents that have no instance-token carrier.
func TestSandboxedExecutionNeitherRequiresNorReceivesInstanceToken(t *testing.T) {
	m := newLaunchManager(&keyedStore{key: launchKey()})
	execution := carrierExecution("n1")
	execution.SandboxSelection = &sandbox.SelectionContext{OrgID: "org"}

	for _, strategy := range []mcpconfig.PassthroughMCPStrategy{mcpconfig.ClaudeStrategy{}, mcpconfig.CursorStrategy{}, mcpconfig.PiStrategy{}} {
		in := carrierServers()
		out, err := m.withSessionMCPAuth(t.Context(), execution, nil, strategy, in)
		if err != nil {
			t.Fatalf("%T: %v", strategy, err)
		}
		for _, server := range out {
			if len(server.Headers) != 0 {
				t.Fatalf("%T: %s received headers %v", strategy, server.Name, server.Headers)
			}
		}
		if err := m.requireSessionMCPCarrier(execution, nil, strategy); err != nil {
			t.Fatalf("%T: %v", strategy, err)
		}
	}

	var tui agents.Agent
	if err := m.requireSessionMCPCarrier(execution, tui, mcpconfig.CursorStrategy{}); err != nil {
		t.Fatal(err)
	}

	request := process.InteractiveStartRequest{Command: []string{"codex", sessionmcp.CodexTokenArg}}
	got, err := m.withCodexSessionTokenEnv(t.Context(), execution, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := got.Env[sessionmcp.CodexTokenEnv]; present {
		t.Fatal("sandboxed Codex start must not receive the instance token variable")
	}
	if err := m.checkCodexCarrier(t.Context(), execution, request.Command, process.InteractiveStartRequest{}.LaunchWrapper); err != nil {
		t.Fatal(err)
	}
}
