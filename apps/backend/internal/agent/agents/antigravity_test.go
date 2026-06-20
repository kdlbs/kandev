package agents

import (
	"context"
	"os/exec"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/pkg/agent"
)

func TestAntigravity_CommandSurfaces(t *testing.T) {
	a := NewAntigravity()
	if got := a.ID(); got != "antigravity" {
		t.Fatalf("ID() = %q, want antigravity", got)
	}
	if got := a.DisplayName(); got != "Antigravity" {
		t.Fatalf("DisplayName() = %q, want Antigravity", got)
	}
	if got := a.BuildCommand(CommandOptions{}).Args(); !reflect.DeepEqual(got, []string{"agy"}) {
		t.Fatalf("BuildCommand() = %v, want [agy]", got)
	}

	rt := a.Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if rt.Protocol != agent.ProtocolACP {
		t.Fatalf("Runtime.Protocol = %q, want acp", rt.Protocol)
	}
	if got := rt.Cmd.Args(); !reflect.DeepEqual(got, []string{"agy"}) {
		t.Fatalf("Runtime.Cmd = %v, want [agy]", got)
	}
	if rt.SessionConfig.SessionDirTemplate != "{home}/.gemini/antigravity-cli" {
		t.Fatalf("SessionDirTemplate = %q", rt.SessionConfig.SessionDirTemplate)
	}
	if !IsPassthroughOnly(a) {
		t.Fatal("Antigravity should seed profiles as passthrough-only")
	}
}

func TestAntigravity_PassthroughConfig(t *testing.T) {
	pt := NewAntigravity().PassthroughConfig()
	if !pt.Supported {
		t.Fatal("passthrough should be supported")
	}
	if got := pt.PassthroughCmd.Args(); !reflect.DeepEqual(got, []string{"agy"}) {
		t.Fatalf("PassthroughCmd = %v, want [agy]", got)
	}
	if got := pt.ModelFlag.Args(); !reflect.DeepEqual(got, []string{"--model", "{model}"}) {
		t.Fatalf("ModelFlag = %v", got)
	}
	if !pt.PromptFlag.IsEmpty() {
		t.Fatalf("PromptFlag = %v, want empty so prompt is auto-injected into PTY", pt.PromptFlag.Args())
	}
	if !pt.AutoInjectPrompt || pt.SubmitSequence != "\r" {
		t.Fatalf("auto-inject config = AutoInjectPrompt:%v SubmitSequence:%q", pt.AutoInjectPrompt, pt.SubmitSequence)
	}
	if got := pt.ResumeFlag.Args(); !reflect.DeepEqual(got, []string{"--continue"}) {
		t.Fatalf("ResumeFlag = %v", got)
	}
	if got := pt.SessionResumeFlag.Args(); !reflect.DeepEqual(got, []string{"--conversation"}) {
		t.Fatalf("SessionResumeFlag = %v", got)
	}
	if _, ok := pt.MCPStrategy.(mcpconfig.AntigravityStrategy); !ok {
		t.Fatalf("MCPStrategy = %T, want AntigravityStrategy", pt.MCPStrategy)
	}
}

func TestAntigravity_PermissionSettings(t *testing.T) {
	settings := NewAntigravity().PermissionSettings()
	skip := settings[PermissionKeyDangerouslySkipPermissions]
	if !skip.Supported || skip.ApplyMethod != PermissionApplyMethodCLIFlag || skip.CLIFlag != "--dangerously-skip-permissions" {
		t.Fatalf("skip permissions setting = %+v", skip)
	}
	sandbox := settings["enable_sandbox"]
	if !sandbox.Supported || sandbox.ApplyMethod != PermissionApplyMethodCLIFlag || sandbox.CLIFlag != "--sandbox" {
		t.Fatalf("sandbox setting = %+v", sandbox)
	}
}

func TestAntigravity_RemoteAuth(t *testing.T) {
	auth := NewAntigravity().RemoteAuth()
	if auth == nil || len(auth.Methods) != 2 {
		t.Fatalf("RemoteAuth() = %+v, want two file methods", auth)
	}
	if auth.Methods[0].TargetRelDir != ".gemini" {
		t.Fatalf("auth method 0 TargetRelDir = %q", auth.Methods[0].TargetRelDir)
	}
	if auth.Methods[1].TargetRelDir != ".gemini/config" {
		t.Fatalf("auth method 1 TargetRelDir = %q", auth.Methods[1].TargetRelDir)
	}
}

func TestAntigravity_IsInstalledRequiresAgy(t *testing.T) {
	if _, err := exec.LookPath("agy"); err == nil {
		t.Skip("agy is on PATH; can't verify unavailable detection")
	}
	result, err := NewAntigravity().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Fatal("Available=true without agy on PATH")
	}
}
