package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOpenCodeACPUsesInstalledBinaryAndPinnedInstaller(t *testing.T) {
	a := NewOpenCodeACP()
	want := []string{"opencode", "acp"}

	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, want)
	}
	if got := a.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, want)
	}
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("Inference Command = %#v, want %#v", got, want)
	}
	if got, wantInstall := a.InstallScript(), "npm install -g opencode-ai@1.18.4"; got != wantInstall {
		t.Fatalf("InstallScript = %q, want %q", got, wantInstall)
	}
}

func TestOpenCodeACPDiscoveryMatchesRuntimeExecutable(t *testing.T) {
	binaryPath := writeOpenCodeTestBinary(t, "printf '1.18.4\\n'")

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
	if got := a.BuildCommand(CommandOptions{}).Args()[0]; got != filepath.Base(binaryPath) {
		t.Fatalf("runtime executable = %q, discovered executable = %q", got, filepath.Base(binaryPath))
	}
}

func TestOpenCodeACPDiscoveryAcceptsSupportedVersionOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "exact", output: "1.18.4"},
		{name: "prefixed and whitespace", output: "  OpenCode version v1.18.4  "},
		{name: "slash prefix", output: "opencode/1.18.4 linux-x64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeOpenCodeTestBinary(t, "printf '%s\\n' \"$OPEN_CODE_VERSION_OUTPUT\"")
			t.Setenv("OPEN_CODE_VERSION_OUTPUT", tt.output)

			result, err := NewOpenCodeACP().IsInstalled(context.Background())
			if err != nil {
				t.Fatalf("IsInstalled() error = %v", err)
			}
			if !result.Available {
				t.Fatal("IsInstalled() Available = false, want true")
			}
		})
	}
}

func TestOpenCodeACPDiscoveryRejectsUnsupportedVersion(t *testing.T) {
	binaryPath := writeOpenCodeTestBinary(t, "printf '1.16.2\\n'")

	result, err := NewOpenCodeACP().IsInstalled(context.Background())
	if err == nil {
		t.Fatal("IsInstalled() error = nil, want version mismatch")
	}
	if result.Available {
		t.Fatal("IsInstalled() Available = true, want false")
	}
	if result.MatchedPath != binaryPath {
		t.Fatalf("MatchedPath = %q, want %q", result.MatchedPath, binaryPath)
	}
	for _, want := range []string{"1.16.2", "requires 1.18.4", "npm install -g opencode-ai@1.18.4"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestOpenCodeACPDiscoveryRejectsMalformedVersion(t *testing.T) {
	writeOpenCodeTestBinary(t, "printf 'OpenCode development build\\n'")

	result, err := NewOpenCodeACP().IsInstalled(context.Background())
	if err == nil {
		t.Fatal("IsInstalled() error = nil, want parse failure")
	}
	if result.Available {
		t.Fatal("IsInstalled() Available = true, want false")
	}
	if !strings.Contains(err.Error(), "npm install -g opencode-ai@1.18.4") {
		t.Fatalf("error %q does not contain pinned remediation", err)
	}
}

func TestOpenCodeACPDiscoveryHandlesVersionCommandFailure(t *testing.T) {
	writeOpenCodeTestBinary(t, "exit 7")

	result, err := NewOpenCodeACP().IsInstalled(context.Background())
	if err == nil {
		t.Fatal("IsInstalled() error = nil, want command failure")
	}
	if result.Available {
		t.Fatal("IsInstalled() Available = true, want false")
	}
	if !strings.Contains(err.Error(), "npm install -g opencode-ai@1.18.4") {
		t.Fatalf("error %q does not contain pinned remediation", err)
	}
}

func TestOpenCodeACPDiscoveryHonorsCancellation(t *testing.T) {
	writeOpenCodeTestBinary(t, "/bin/sleep 10")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := NewOpenCodeACP().IsInstalled(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IsInstalled() error = %v, want context.Canceled", err)
	}
	if result.Available {
		t.Fatal("IsInstalled() Available = true, want false")
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
