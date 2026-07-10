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

func TestCodexACP_PermissionSettings_CuratedConfigOverrides(t *testing.T) {
	settings := NewCodexACP().PermissionSettings()
	if len(settings) != 0 {
		t.Fatalf("PermissionSettings() = %#v, want no Codex ACP CLI flags", settings)
	}
}

func TestCodexACP_BuildCommand_NoCodexCLIFlags(t *testing.T) {
	want := []string{"npx", "-y", "@agentclientprotocol/codex-acp"}
	cmd := NewCodexACP().BuildCommand(CommandOptions{
		PermissionValues: map[string]bool{PermissionKeyAutoApprove: true},
	})
	if !slices.Equal(cmd.Args(), want) {
		t.Fatalf("BuildCommand = %#v, want %#v", cmd.Args(), want)
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
