package agents

import (
	"context"
	"encoding/json"
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

func TestOpenCodeACPApplyFilesystemPolicyRendersNativePermissions(t *testing.T) {
	env := map[string]string{"NPM_CONFIG_CACHE": "/tmp/cache"}
	err := NewOpenCodeACP().ApplyFilesystemPolicy(env, FilesystemPolicy{
		Name: "kandev_task_git_metadata",
		Rules: []FilesystemPolicyRule{
			{Path: "/workspace/.git", Access: FilesystemAccessWrite},
			{Path: "/workspace/.git/worktrees", Access: FilesystemAccessDeny},
			{Path: "/workspace/.git/objects", Access: FilesystemAccessRead},
		},
	})
	if err != nil {
		t.Fatalf("ApplyFilesystemPolicy() error = %v", err)
	}
	var config struct {
		Permission map[string]map[string]string `json:"permission"`
	}
	if err := json.Unmarshal([]byte(env["OPENCODE_CONFIG_CONTENT"]), &config); err != nil {
		t.Fatalf("decode OPENCODE_CONFIG_CONTENT: %v", err)
	}
	if got := config.Permission["edit"]["/workspace/.git/**"]; got != "allow" {
		t.Errorf("edit GitDir permission = %q, want allow", got)
	}
	if got := config.Permission["read"]["/workspace/.git/objects/**"]; got != "allow" {
		t.Errorf("read objects permission = %q, want allow", got)
	}
	for _, action := range []string{"external_directory", "read", "edit"} {
		if got := config.Permission[action]["/workspace/.git/worktrees/**"]; got != "deny" {
			t.Errorf("%s worktrees permission = %q, want deny", action, got)
		}
	}
	if got := env["NPM_CONFIG_CACHE"]; got != "/tmp/cache" {
		t.Errorf("unrelated environment value = %q, want preserved", got)
	}
}

func TestOpenCodeACPApplyFilesystemPolicyFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		policy FilesystemPolicy
	}{
		{name: "missing name", policy: FilesystemPolicy{}},
		{name: "unsupported access", policy: FilesystemPolicy{
			Name:  "kandev_task_git_metadata",
			Rules: []FilesystemPolicyRule{{Path: "/workspace/.git", Access: FilesystemAccess("ask")}},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"OPENCODE_CONFIG_CONTENT": `{"permission":{"edit":{"*":"allow"}}}`}
			if err := NewOpenCodeACP().ApplyFilesystemPolicy(env, tc.policy); err == nil {
				t.Fatal("ApplyFilesystemPolicy() error = nil, want fail-closed rejection")
			}
			if got := env["OPENCODE_CONFIG_CONTENT"]; got != `{"permission":{"edit":{"*":"allow"}}}` {
				t.Errorf("failed policy replaced existing config: %q", got)
			}
		})
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
