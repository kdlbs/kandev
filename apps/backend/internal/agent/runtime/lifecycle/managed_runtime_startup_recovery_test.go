package lifecycle

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestStartupAttemptGenerationRejectsStaleEventsAfterRecovery(t *testing.T) {
	execution := &AgentExecution{}
	first := execution.beginStartupAttempt()
	if !execution.acceptsStartupAttempt(first) {
		t.Fatal("initial startup generation should be current")
	}

	second, ok := execution.beginStartupRecovery()
	if !ok {
		t.Fatal("expected startup recovery generation")
	}
	if second == first || execution.acceptsStartupAttempt(first) {
		t.Fatal("first child generation remained current after recovery")
	}
	if !execution.acceptsStartupAttempt(second) {
		t.Fatal("retry startup generation should be current")
	}
	if _, ok := execution.beginStartupRecovery(); ok {
		t.Fatal("startup recovery should be attempted at most once")
	}
	execution.finishStartupRecovery()
}

func TestOnlineManagedRuntimeArgsPreserveTrustedLaunchIdentity(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{
		Package: "@scope/managed-acp",
		ACPArgs: []string{"--acp", "--model", "fast"},
	}
	initial := []string{"greywall", "--", "npx", "--yes", "--prefer-offline", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}

	got, packageSpec, ok := onlineManagedRuntimeArgs(initial, spec)
	if !ok {
		t.Fatal("expected managed runtime recovery command")
	}
	want := []string{"greywall", "--", "npx", "--yes", "--prefer-online", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("online args = %#v, want %#v", got, want)
	}
	if packageSpec != "@scope/managed-acp@1.2.3" {
		t.Fatalf("package spec = %q, want exact selected spec", packageSpec)
	}
}

func TestOnlineManagedRuntimeArgsRejectsNonManagedCommands(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	for _, args := range [][]string{
		{"native-agent", "--acp"},
		{"npx", "--yes", "--prefer-offline", "other-agent@1.2.3"},
		{"npx", "--yes", "--prefer-online", "managed-acp@1.2.3"},
	} {
		if _, _, ok := onlineManagedRuntimeArgs(args, spec); ok {
			t.Fatalf("command %#v should not be eligible for managed runtime recovery", args)
		}
	}
}
