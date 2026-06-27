package agents

import (
	"slices"
	"testing"
)

// TestOpenCodeACPRuntime_RequiresProcessKill is the regression test for GH
// issue #1247: opencode acp keeps its HTTP server + MCP child tree alive
// when stdin closes, so its RuntimeConfig must signal that the process
// group should be reaped immediately. Without this flag the ACP adapter
// returns RequiresProcessKill=false and the process manager waits for the
// graceful EOF path before it falls back to process-group cleanup.
func TestOpenCodeACPRuntime_RequiresProcessKill(t *testing.T) {
	rt := NewOpenCodeACP().Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if !rt.RequiresProcessKill {
		t.Error("RequiresProcessKill = false; opencode acp must opt into process-group kill")
	}
}

// TestACPAgents_DefaultProcessKill confirms the rest of the ACP agents
// stick with the default (false). They communicate over plain stdin/stdout
// and should get a short graceful EOF path before the process manager reaps
// any remaining process-group descendants.
func TestACPAgents_DefaultProcessKill(t *testing.T) {
	cases := []struct {
		name  string
		agent Agent
	}{
		{"claude", NewClaudeACP()},
		{"codex", NewCodexACP()},
		{"cursor", NewCursorACP()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := tc.agent.Runtime()
			if rt == nil {
				t.Fatalf("%s Runtime() returned nil", tc.name)
			}
			if rt.RequiresProcessKill {
				t.Errorf("%s RequiresProcessKill = true; expected default false", tc.name)
			}
		})
	}
}

func TestOpenCodeACPRemoteAuth(t *testing.T) {
	auth := NewOpenCodeACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil; expected files-based auth method")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}
	m := auth.Methods[0]
	if m.Type != "files" {
		t.Errorf("Type = %q, want %q", m.Type, "files")
	}
	if m.TargetRelDir != ".local/share/opencode" {
		t.Errorf("TargetRelDir = %q, want %q", m.TargetRelDir, ".local/share/opencode")
	}
	want := []string{".local/share/opencode/auth.json"}
	for _, os := range []string{"darwin", "linux"} {
		got := m.SourceFiles[os]
		if !slices.Equal(got, want) {
			t.Errorf("SourceFiles[%q] = %v, want %v", os, got, want)
		}
	}
}
