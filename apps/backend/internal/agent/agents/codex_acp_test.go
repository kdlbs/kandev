package agents

import (
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
	want := []string{"npx", "-y", codexACPPackageSpec}
	cmd := NewCodexACP().BuildCommand(CommandOptions{
		PermissionValues: map[string]bool{PermissionKeyAutoApprove: true},
	})
	if !slices.Equal(cmd.Args(), want) {
		t.Fatalf("BuildCommand = %#v, want %#v", cmd.Args(), want)
	}
}

func TestCodexACP_UsesAgentClientProtocolBridge(t *testing.T) {
	a := NewCodexACP()
	want := []string{"npx", "-y", "@agentclientprotocol/codex-acp@1.1.5"}

	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, want)
	}
	if got := a.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, want)
	}
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("Inference Command = %#v, want %#v", got, want)
	}
	wantInstall := "npm install -g @openai/codex @agentclientprotocol/codex-acp@1.1.5"
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
