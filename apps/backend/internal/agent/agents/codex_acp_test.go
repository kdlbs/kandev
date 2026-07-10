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
	want := map[string]struct {
		flagText  string
		defaultOn bool
	}{
		"config_approval_policy_never": {
			flagText: "-c approval_policy=never", defaultOn: false,
		},
		"config_sandbox_disk_full_read": {
			flagText: CodexACPSandboxDiskFullReadCLIFlag, defaultOn: false,
		},
	}
	if len(settings) != len(want) {
		t.Fatalf("PermissionSettings() len = %d, want %d: %#v", len(settings), len(want), settings)
	}
	for key, spec := range want {
		s, ok := settings[key]
		if !ok {
			t.Fatalf("missing %q in PermissionSettings()", key)
		}
		if !s.Supported || s.ApplyMethod != PermissionApplyMethodCLIFlag {
			t.Fatalf("%q: unsupported or wrong apply method: %+v", key, s)
		}
		if s.Default != spec.defaultOn {
			t.Fatalf("%q: Default=%v, want %v", key, s.Default, spec.defaultOn)
		}
		gotFlag := s.CLIFlag
		if s.CLIFlagValue != "" {
			gotFlag = s.CLIFlag + " " + s.CLIFlagValue
		}
		if gotFlag != spec.flagText {
			t.Fatalf("%q: flag text %q, want %q", key, gotFlag, spec.flagText)
		}
	}
}

func TestCodexACP_BuildCommand_NoCodexCLIFlags(t *testing.T) {
	want := []string{"npx", "-y", codexACPPkg}
	cmd := NewCodexACP().BuildCommand(CommandOptions{
		PermissionValues: map[string]bool{PermissionKeyAutoApprove: true},
	})
	if !slices.Equal(cmd.Args(), want) {
		t.Fatalf("BuildCommand = %#v, want %#v", cmd.Args(), want)
	}
}

func TestCodexACP_UsesAgentClientProtocolBridge(t *testing.T) {
	a := NewCodexACP()
	want := []string{"npx", "-y", "@agentclientprotocol/codex-acp"}

	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, want)
	}
	if got := a.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, want)
	}
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("Inference Command = %#v, want %#v", got, want)
	}
	if got := a.InstallScript(); !strings.Contains(got, "@agentclientprotocol/codex-acp") {
		t.Fatalf("InstallScript = %q, want agentclientprotocol bridge package", got)
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
