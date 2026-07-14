package agents

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

func TestGrokACP_IDAndDisplay(t *testing.T) {
	a := NewGrokACP()
	if got := a.ID(); got != "grok-acp" {
		t.Errorf("ID() = %q, want grok-acp", got)
	}
	if got := a.DisplayName(); got != "Grok" {
		t.Errorf("DisplayName() = %q, want Grok", got)
	}
	if !a.Enabled() {
		t.Error("Enabled() = false, want true")
	}
	if got := a.DisplayOrder(); got != 20 {
		t.Errorf("DisplayOrder() = %d, want 20", got)
	}
}

func TestGrokACP_AllCommandSurfaces(t *testing.T) {
	a := NewGrokACP()
	want := []string{"grok", "--no-auto-update", "agent", "stdio"}

	assertArgvEqual(t, "BuildCommand", a.BuildCommand(CommandOptions{}).Args(), want)

	rt := a.Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if rt.Protocol != agent.ProtocolACP {
		t.Errorf("Runtime.Protocol = %q, want ACP", rt.Protocol)
	}
	assertArgvEqual(t, "Runtime.Cmd", rt.Cmd.Args(), want)

	ic := a.InferenceConfig()
	if ic == nil || !ic.Supported {
		t.Fatalf("InferenceConfig() = %+v, want Supported=true", ic)
	}
	assertArgvEqual(t, "InferenceConfig.Command", ic.Command.Args(), want)

	// ACP-only slice: no PassthroughAgent.
	if _, ok := any(a).(PassthroughAgent); ok {
		t.Error("GrokACP must not implement PassthroughAgent in this slice")
	}
}

func TestGrokACP_InstallScript(t *testing.T) {
	got := NewGrokACP().InstallScript()
	want := "npm install -g @xai-official/grok"
	if got != want {
		t.Errorf("InstallScript() = %q, want %q", got, want)
	}
}

func TestGrokACP_DetectionRequiresGlobalBinary(t *testing.T) {
	if _, err := exec.LookPath("grok"); err == nil {
		t.Skip("detection binary \"grok\" is on PATH; can't verify availability requirement")
	}
	result, err := NewGrokACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true without grok on PATH; discovery must not imply install")
	}
}

func TestGrokACP_LogosNonEmpty(t *testing.T) {
	a := NewGrokACP()
	if len(a.Logo(LogoLight)) == 0 {
		t.Error("Logo(LogoLight) is empty")
	}
	if len(a.Logo(LogoDark)) == 0 {
		t.Error("Logo(LogoDark) is empty")
	}
	if !strings.Contains(string(a.Logo(LogoLight)), "<svg") {
		t.Error("Logo(LogoLight) is not SVG")
	}
}

func TestGrokACP_DisplayOrderUnique(t *testing.T) {
	all := []Agent{
		NewClaudeACP(), NewCodexACP(), NewAuggie(), NewOpenCodeACP(),
		NewGemini(), NewCopilotACP(), NewAmpACP(), NewQwenACP(),
		NewIFlowACP(), NewDroidACP(), NewKilocodeACP(), NewPiACP(),
		NewCursorACP(), NewKimiACP(), NewKiroACP(), NewQoderACP(),
		NewTraeACP(), NewOmpACP(), NewDevinACP(), NewGrokACP(),
		NewMockAgent(),
	}
	seen := map[int]string{}
	for _, ag := range all {
		order := ag.DisplayOrder()
		if other, exists := seen[order]; exists {
			t.Errorf("DisplayOrder %d collision: %s and %s", order, other, ag.ID())
		}
		seen[order] = ag.ID()
	}
}

func TestGrokACP_RemoteAuth(t *testing.T) {
	auth := NewGrokACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil")
	}
	if len(auth.Methods) != 2 {
		t.Fatalf("Methods len = %d, want 2", len(auth.Methods))
	}

	files := auth.Methods[0]
	if files.Type != "files" {
		t.Errorf("Methods[0].Type = %q, want files", files.Type)
	}
	if files.TargetRelDir != ".grok" {
		t.Errorf("TargetRelDir = %q, want .grok", files.TargetRelDir)
	}
	for _, osName := range []string{"darwin", "linux"} {
		paths := files.SourceFiles[osName]
		if !slices.Equal(paths, []string{".grok/auth.json"}) {
			t.Errorf("SourceFiles[%s] = %#v, want [.grok/auth.json] only", osName, paths)
		}
		for _, p := range paths {
			if strings.Contains(p, "config.toml") || strings.Contains(p, "session") {
				t.Errorf("must not copy non-auth state: %q", p)
			}
		}
	}

	env := auth.Methods[1]
	if env.Type != "env" || env.EnvVar != "XAI_API_KEY" {
		t.Errorf("Methods[1] = %+v, want env XAI_API_KEY", env)
	}
}

func TestGrokACP_LoginCommand(t *testing.T) {
	cmd := NewGrokACP().LoginCommand()
	if cmd == nil {
		t.Fatal("LoginCommand() returned nil")
	}
	want := []string{"grok", "login", "--device-auth"}
	if !slices.Equal(cmd.Cmd, want) {
		t.Errorf("LoginCommand.Cmd = %#v, want %#v", cmd.Cmd, want)
	}
	if cmd.Description == "" {
		t.Error("LoginCommand.Description is empty")
	}
}

func TestGrokACP_SessionAndSkills(t *testing.T) {
	rt := NewGrokACP().Runtime()
	if rt.WorkingDir != "{workspace}" {
		t.Errorf("WorkingDir = %q, want {workspace}", rt.WorkingDir)
	}
	if len(rt.RequiredEnv) != 0 {
		t.Errorf("RequiredEnv = %#v, want empty (cached OAuth is valid without XAI_API_KEY)", rt.RequiredEnv)
	}
	if rt.ProjectSkillDir != ".grok/skills" {
		t.Errorf("ProjectSkillDir = %q, want .grok/skills", rt.ProjectSkillDir)
	}
	if rt.UserSkillDir != ".grok/skills" {
		t.Errorf("UserSkillDir = %q, want .grok/skills", rt.UserSkillDir)
	}
	sc := rt.SessionConfig
	if !sc.NativeSessionResume {
		t.Error("NativeSessionResume = false, want true")
	}
	if sc.CanRecover == nil || !*sc.CanRecover {
		t.Error("CanRecover must be true")
	}
	if sc.SessionDirTemplate != "{home}/.grok" {
		t.Errorf("SessionDirTemplate = %q, want {home}/.grok", sc.SessionDirTemplate)
	}
}

func TestGrokACP_PermissionAndBillingDefaults(t *testing.T) {
	a := NewGrokACP()
	if len(a.PermissionSettings()) != 0 {
		t.Errorf("PermissionSettings() = %#v, want empty (agentctl auto-approve is authoritative)", a.PermissionSettings())
	}
	if got := a.BillingType(); got != usage.BillingTypeAPIKey {
		t.Errorf("BillingType() = %q, want %q", got, usage.BillingTypeAPIKey)
	}
	// Catalog still injects agentctl auto-approve.
	catalog := CatalogPermissionSettings(a)
	auto, ok := catalog[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("catalog missing auto_approve")
	}
	if auto.ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Errorf("auto_approve ApplyMethod = %q, want agentctl auto-approve", auto.ApplyMethod)
	}
}
