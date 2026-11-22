package managedruntime

import (
	"reflect"
	"testing"
)

func TestPreparedRuntimeRewritePreservesOpaqueAgentArguments(t *testing.T) {
	packageSpec := "@agentclientprotocol/claude-agent-acp@0.100.0"
	prepared := &PreparedRuntime{
		packageSpec: packageSpec,
		project:     "/owned/project",
		cache:       "/owned/cache",
	}
	command := []string{
		"npx", "--yes", "--prefer-offline", "--prefix", NPMProjectPrefix, packageSpec,
		"--dangerously-skip-permissions", packageSpec, "--cache", "/agent/value",
	}
	rewritten, err := prepared.RewriteNpxCommand(command)
	if err != nil {
		t.Fatalf("RewriteNpxCommand: %v", err)
	}
	if rewritten[4] != prepared.project {
		t.Fatalf("rewritten prefix = %q, want %q", rewritten[4], prepared.project)
	}
	if !containsArgumentPair(rewritten[5:], "--cache", prepared.cache) {
		t.Fatalf("rewritten command = %#v, want owned cache", rewritten)
	}
	if got, want := rewritten[len(rewritten)-3:], command[len(command)-3:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("agent arguments = %#v, want unchanged %#v", got, want)
	}
	if command[4] != NPMProjectPrefix {
		t.Fatal("RewriteNpxCommand mutated the logical input")
	}
}
