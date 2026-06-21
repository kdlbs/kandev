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
	// Default (ACP) launch goes through the antigravity-acp shim, not agy.
	if got := a.BuildCommand(CommandOptions{}).Args(); !reflect.DeepEqual(got, []string{"antigravity-acp"}) {
		t.Fatalf("BuildCommand() = %v, want [antigravity-acp]", got)
	}
	// An explicit host binary path is preferred on standalone runtimes, and the
	// selected model is forwarded to the shim as its session default.
	a.SetBinaryPath("/opt/kandev/antigravity-acp")
	if got := a.BuildCommand(CommandOptions{Model: "Gemini 3.5 Flash (Medium)"}).Args(); !reflect.DeepEqual(got, []string{"/opt/kandev/antigravity-acp", "--model", "Gemini 3.5 Flash (Medium)"}) {
		t.Fatalf("BuildCommand(model) = %v", got)
	}

	rt := a.Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if rt.Protocol != agent.ProtocolACP {
		t.Fatalf("Runtime.Protocol = %q, want acp", rt.Protocol)
	}
	if got := rt.Cmd.Args(); !reflect.DeepEqual(got, []string{"antigravity-acp"}) {
		t.Fatalf("Runtime.Cmd = %v, want [antigravity-acp]", got)
	}
	if rt.SessionConfig.SessionDirTemplate != "{home}/.gemini/antigravity-cli" {
		t.Fatalf("SessionDirTemplate = %q", rt.SessionConfig.SessionDirTemplate)
	}
	// Antigravity is no longer passthrough-only: the default is the ACP chat
	// dialog, with the raw agy TUI available via the CLI-passthrough toggle.
	if IsPassthroughOnly(a) {
		t.Fatal("Antigravity should default to ACP (chat), not passthrough-only")
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

func TestParseAntigravityModels(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []DiscoveredModel
	}{
		{
			name: "real output",
			out:  "Gemini 3.5 Flash (Medium)\nGemini 3.1 Pro (High)\nClaude Sonnet 4.6 (Thinking)\n",
			want: []DiscoveredModel{
				{ID: "Gemini 3.5 Flash (Medium)", Name: "Gemini 3.5 Flash (Medium)"},
				{ID: "Gemini 3.1 Pro (High)", Name: "Gemini 3.1 Pro (High)"},
				{ID: "Claude Sonnet 4.6 (Thinking)", Name: "Claude Sonnet 4.6 (Thinking)"},
			},
		},
		{
			name: "skips blank and whitespace lines",
			out:  "\n  Gemini 3.5 Flash (Low)  \n\n   \nGPT-OSS 120B (Medium)\n",
			want: []DiscoveredModel{
				{ID: "Gemini 3.5 Flash (Low)", Name: "Gemini 3.5 Flash (Low)"},
				{ID: "GPT-OSS 120B (Medium)", Name: "GPT-OSS 120B (Medium)"},
			},
		},
		{
			name: "empty output",
			out:  "",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseAntigravityModels(tc.out); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseAntigravityModels() = %v, want %v", got, tc.want)
			}
		})
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
