package agents

import (
	"reflect"
	"testing"
)

func TestManagedNPMRuntimeSpecMatchRuntimeCommand(t *testing.T) {
	spec := newManagedNPMRuntimeSpec("@agentclientprotocol/claude-agent-acp")
	command := spec.ACPCommand("0.100.0").Args()
	command = append(command, "--dangerously-skip-permissions")
	invocation, ok := spec.MatchRuntimeCommand(command)
	if !ok {
		t.Fatal("registered launcher with valid profile arguments was rejected")
	}
	if got, want := invocation.AgentArgs(), []string{"--dangerously-skip-permissions"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("agent args = %#v, want %#v", got, want)
	}
	if invocation.PackageSpec != "@agentclientprotocol/claude-agent-acp@0.100.0" {
		t.Fatalf("package spec = %q", invocation.PackageSpec)
	}

	codex := newManagedNPMRuntimeSpec("@openai/codex", "app-server")
	codexCommand := append(codex.RuntimeCommand("1.2.3").Args(), "--profile-flag", "value")
	if _, ok := codex.MatchRuntimeCommand(codexCommand); !ok {
		t.Fatal("managed native Codex command with valid trailing arguments was rejected")
	}
	wrongSubcommand := codex.RuntimeCommand("1.2.3").Args()
	wrongSubcommand[6] = "acp"
	if _, ok := codex.MatchRuntimeCommand(wrongSubcommand); ok {
		t.Fatal("wrong required runtime argument was accepted")
	}

	wrapped := append([]string{"wrapper"}, command...)
	if _, ok := spec.MatchRuntimeCommand(wrapped); ok {
		t.Fatal("wrapper prefix was accepted as installation authority")
	}
}
