package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentruntime"
)

// TestPreferNativeBinary_NonNativeAgent verifies that an agent which does not
// implement NativeBinaryAgent never prefers a native binary, regardless of
// runtime or metadata.
func TestPreferNativeBinary_NonNativeAgent(t *testing.T) {
	m := &Manager{}
	ag := agents.NewClaudeACP() // does not implement NativeBinaryAgent
	meta := map[string]interface{}{MetadataKeyNativeBinary: "claude"}
	if m.preferNativeBinary(ag, agentruntime.RuntimeStandalone, meta) {
		t.Error("non-native agent should never prefer a native binary")
	}
}

// TestPreferNativeBinary_SSH verifies the SSH branch reads the preflight result
// from metadata: a recorded hit (matching the agent's binary name) prefers
// native; an absent or mismatched key falls back to npx.
func TestPreferNativeBinary_SSH(t *testing.T) {
	m := &Manager{}
	ag := agents.NewCopilotACP()

	hit := map[string]interface{}{MetadataKeyNativeBinary: "copilot"}
	if !m.preferNativeBinary(ag, agentruntime.RuntimeSSH, hit) {
		t.Error("ssh with recorded copilot hit should prefer native binary")
	}

	if m.preferNativeBinary(ag, agentruntime.RuntimeSSH, nil) {
		t.Error("ssh without a recorded hit should fall back to npx")
	}

	mismatch := map[string]interface{}{MetadataKeyNativeBinary: "something-else"}
	if m.preferNativeBinary(ag, agentruntime.RuntimeSSH, mismatch) {
		t.Error("ssh with a mismatched binary name should fall back to npx")
	}
}

// TestPreferNativeBinary_ContainerizedKeepsNpx verifies containerized runtimes
// keep npx even when metadata claims a hit — their controlled image pulls npx
// from the public registry, so the private-registry slowdown does not apply.
func TestPreferNativeBinary_ContainerizedKeepsNpx(t *testing.T) {
	m := &Manager{}
	ag := agents.NewCopilotACP()
	meta := map[string]interface{}{MetadataKeyNativeBinary: "copilot"}
	for _, rt := range []agentruntime.Runtime{
		agentruntime.RuntimeDocker,
		agentruntime.RuntimeSprites,
		agentruntime.RuntimeRemoteDocker,
	} {
		if m.preferNativeBinary(ag, rt, meta) {
			t.Errorf("runtime %q should keep npx", rt)
		}
	}
}

// TestPreferNativeBinary_StandaloneLooksUpPath verifies the standalone branch
// probes the backend host's PATH (which is the execution environment for
// local_pc / worktree). A binary on PATH prefers native; an empty PATH does not.
func TestPreferNativeBinary_StandaloneLooksUpPath(t *testing.T) {
	m := &Manager{}
	ag := agents.NewCopilotACP()

	dir := t.TempDir()
	bin := filepath.Join(dir, "copilot")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake copilot: %v", err)
	}

	t.Setenv("PATH", dir)
	if !m.preferNativeBinary(ag, agentruntime.RuntimeStandalone, nil) {
		t.Error("standalone with copilot on PATH should prefer native binary")
	}

	t.Setenv("PATH", t.TempDir()) // empty dir, no copilot
	if m.preferNativeBinary(ag, agentruntime.RuntimeStandalone, nil) {
		t.Error("standalone without copilot on PATH should fall back to npx")
	}
}
