package agents

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenCodeACPUsesManagedRuntime(t *testing.T) {
	a := NewOpenCodeACP()
	want := a.ManagedNPMRuntime().CachedACPCommand().Args()

	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, want)
	}
	if got := a.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, want)
	}
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("Inference Command = %#v, want %#v", got, want)
	}
	if got, wantInstall := a.InstallScript(), "npm install -g opencode-ai"; got != wantInstall {
		t.Fatalf("InstallScript = %q, want %q", got, wantInstall)
	}
}

func TestOpenCodeACPDiscoveryRecognizesAuthenticationHelper(t *testing.T) {
	binaryPath := writeOpenCodeTestBinary(t, "exit 7")

	a := NewOpenCodeACP()
	result, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !result.Available {
		t.Fatal("IsInstalled() Available = false, want true")
	}
	if result.MatchedPath != binaryPath {
		t.Fatalf("IsInstalled() MatchedPath = %q, want %q", result.MatchedPath, binaryPath)
	}
}

func TestOpenCodeACPDiscoveryDoesNotRunVersionCommand(t *testing.T) {
	writeOpenCodeTestBinary(t, "exit 7")

	result, err := NewOpenCodeACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !result.Available {
		t.Fatal("IsInstalled() Available = false, want true")
	}
}

func writeOpenCodeTestBinary(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "opencode")
	contents := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(binaryPath, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}
	t.Setenv("PATH", dir)
	return binaryPath
}

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
