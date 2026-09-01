package agents

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCodexACPRuntimeNoLongerBindMountsHostHome(t *testing.T) {
	a := NewCodexACP()
	rt := a.Runtime()

	for _, m := range rt.Mounts {
		if strings.Contains(m.Source, "{home}") {
			t.Fatalf("codex Mounts unexpectedly references {home}: %+v", m)
		}
	}
}

func TestCodexACP_PermissionSettings_NoBridgeCLIFlags(t *testing.T) {
	settings := NewCodexACP().PermissionSettings()
	if len(settings) != 0 {
		t.Fatalf("PermissionSettings() = %#v, want no Codex ACP bridge CLI flags", settings)
	}
}

func TestCodexACP_BuildCommand_NoCodexCLIFlags(t *testing.T) {
	agent := NewCodexACP()
	want := agent.ManagedNPMRuntime().CachedACPCommand().Args()
	cmd := agent.BuildCommand(CommandOptions{
		PermissionValues: map[string]bool{PermissionKeyAutoApprove: true},
	})
	if !slices.Equal(cmd.Args(), want) {
		t.Fatalf("BuildCommand = %#v, want %#v", cmd.Args(), want)
	}
}

func TestCodexACP_UsesAgentClientProtocolBridge(t *testing.T) {
	a := NewCodexACP()
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
	wantInstall := "npm install -g @openai/codex @agentclientprotocol/codex-acp"
	if got := a.InstallScript(); got != wantInstall {
		t.Fatalf("InstallScript = %q, want %q", got, wantInstall)
	}
}

func TestCodexACPSessionDirTemplate(t *testing.T) {
	a := NewCodexACP()
	cfg := a.Runtime().SessionConfig

	if cfg.SessionDirTemplate != "{home}/.codex" {
		t.Fatalf("SessionDirTemplate = %q, want %q", cfg.SessionDirTemplate, "{home}/.codex")
	}
	if cfg.SessionDirTarget != "/root/.codex" {
		t.Fatalf("SessionDirTarget = %q, want %q", cfg.SessionDirTarget, "/root/.codex")
	}
}

func TestCodexACPRendersNarrowFilesystemPolicy(t *testing.T) {
	env := map[string]string{"CODEX_CONFIG": `{"approval_policy":"never"}`}
	err := NewCodexACP().ApplyFilesystemPolicy(env, FilesystemPolicy{
		Name: "kandev_task_git_metadata",
		Rules: []FilesystemPolicyRule{
			{Path: ":minimal", Access: FilesystemAccessRead},
			{Path: "/task", Access: FilesystemAccessWrite},
			{Path: "/source/.git", Access: FilesystemAccessRead},
			{Path: "/source/.git/worktrees", Access: FilesystemAccessDeny},
			{Path: "/source/.git/worktrees/task", Access: FilesystemAccessWrite},
			{Path: "/source/.git/objects", Access: FilesystemAccessWrite},
		},
	})
	if err != nil {
		t.Fatalf("ApplyFilesystemPolicy() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(env["CODEX_CONFIG"]), &decoded); err != nil {
		t.Fatalf("unmarshal rendered policy: %v", err)
	}
	if got := decoded["default_permissions"]; got != "kandev_task_git_metadata" {
		t.Fatalf("default_permissions = %#v, want task profile", got)
	}
	permissions := decoded["permissions"].(map[string]any)
	profile := permissions["kandev_task_git_metadata"].(map[string]any)
	if got := profile["extends"]; got != ":workspace" {
		t.Fatalf("profile extends = %#v, want :workspace", got)
	}
	rules := profile["filesystem"].(map[string]any)
	if got := rules["/source/.git"]; got != "read" {
		t.Fatalf("common Git root = %#v, want read", got)
	}
	if got := rules["/source/.git/worktrees"]; got != "deny" {
		t.Fatalf("worktrees parent = %#v, want deny", got)
	}
	if got := rules["/source/.git/worktrees/task"]; got != "write" {
		t.Fatalf("owned worktree metadata = %#v, want write", got)
	}
}
