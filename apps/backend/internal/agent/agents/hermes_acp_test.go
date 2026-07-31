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

func TestHermesACP_IDAndDisplay(t *testing.T) {
	a := NewHermesACP()
	if got := a.ID(); got != "hermes-acp" {
		t.Errorf("ID() = %q, want hermes-acp", got)
	}
	if got := a.DisplayName(); got != "Hermes" {
		t.Errorf("DisplayName() = %q, want Hermes", got)
	}
	if !a.Enabled() {
		t.Error("Enabled() = false, want true")
	}
	if got := a.DisplayOrder(); got != 21 {
		t.Errorf("DisplayOrder() = %d, want 21", got)
	}
}

func TestHermesACP_AllCommandSurfaces(t *testing.T) {
	a := NewHermesACP()
	want := []string{"hermes", "acp"}

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

	pa, ok := any(a).(PassthroughAgent)
	if !ok {
		t.Fatal("HermesACP must implement PassthroughAgent")
	}
	assertArgvEqual(t, "PassthroughCmd", pa.PassthroughConfig().PassthroughCmd.Args(), []string{"hermes"})
}

// TestHermesACP_SkipsGlobalMCPStartup pins the host-owned MCP marker. Kandev
// passes session MCP servers through ACP session/new; without the marker
// Hermes would ALSO start every globally configured server from
// ~/.hermes/config.yaml before initialize, duplicating work and slowing the
// handshake. The marker must stay exactly "1" — any other value keeps the
// default behavior.
func TestHermesACP_SkipsGlobalMCPStartup(t *testing.T) {
	rt := NewHermesACP().Runtime()
	if got := rt.Env["HERMES_ACP_SKIP_CONFIGURED_MCP"]; got != "1" {
		t.Errorf("HERMES_ACP_SKIP_CONFIGURED_MCP = %q, want exactly \"1\"", got)
	}
}

func TestHermesACP_InstallScript(t *testing.T) {
	got := NewHermesACP().InstallScript()
	for _, needle := range []string{
		"curl -fsSL https://hermes-agent.nousresearch.com/install.sh",
		"hermes",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("InstallScript missing %q: %q", needle, got)
		}
	}
	if strings.Contains(got, "Install Hermes") {
		t.Errorf("InstallScript must be executable shell, got prose: %q", got)
	}
	if strings.HasPrefix(got, "npm install -g ") {
		t.Errorf("InstallScript should use the native Hermes installer, got npm script: %q", got)
	}
}

func TestHermesACP_DetectionRequiresGlobalBinary(t *testing.T) {
	if _, err := exec.LookPath("hermes"); err == nil {
		t.Skip("detection binary \"hermes\" is on PATH; can't verify availability requirement")
	}
	result, err := NewHermesACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true without hermes on PATH; discovery must not imply install")
	}
}

func TestHermesACP_LogosNonEmpty(t *testing.T) {
	a := NewHermesACP()
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

func TestHermesACP_RemoteAuth(t *testing.T) {
	auth := NewHermesACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}

	files := auth.Methods[0]
	if files.Type != "files" {
		t.Errorf("Methods[0].Type = %q, want files", files.Type)
	}
	if files.TargetRelDir != ".hermes" {
		t.Errorf("TargetRelDir = %q, want .hermes", files.TargetRelDir)
	}
	for _, osName := range []string{"darwin", "linux"} {
		paths := files.SourceFiles[osName]
		want := []string{".hermes/.env", ".hermes/config.yaml"}
		if !slices.Equal(paths, want) {
			t.Errorf("SourceFiles[%s] = %#v, want %#v", osName, paths, want)
		}
		for _, p := range paths {
			if strings.Contains(p, "state.db") || strings.Contains(p, "skills") {
				t.Errorf("must not copy non-auth state: %q", p)
			}
		}
	}
}

func TestHermesACP_LoginCommand(t *testing.T) {
	cmd := NewHermesACP().LoginCommand()
	if cmd == nil {
		t.Fatal("LoginCommand() returned nil")
	}
	want := []string{"hermes", "model"}
	if !slices.Equal(cmd.Cmd, want) {
		t.Errorf("LoginCommand.Cmd = %#v, want %#v", cmd.Cmd, want)
	}
	if cmd.Description == "" {
		t.Error("LoginCommand.Description is empty")
	}
}

func TestHermesACP_SessionAndSkills(t *testing.T) {
	rt := NewHermesACP().Runtime()
	if rt.WorkingDir != "{workspace}" {
		t.Errorf("WorkingDir = %q, want {workspace}", rt.WorkingDir)
	}
	if rt.UserSkillDir != ".hermes/skills" {
		t.Errorf("UserSkillDir = %q, want .hermes/skills", rt.UserSkillDir)
	}
	sc := rt.SessionConfig
	if !sc.NativeSessionResume {
		t.Error("NativeSessionResume = false, want true")
	}
	if sc.NewSessionOnWorkspaceRebind {
		t.Error("NewSessionOnWorkspaceRebind = true, want false (Hermes reloads sessions with the new cwd)")
	}
	if sc.CanRecover == nil || !*sc.CanRecover {
		t.Error("CanRecover must be true")
	}
	if sc.SessionDirTemplate != "{home}/.hermes" {
		t.Errorf("SessionDirTemplate = %q, want {home}/.hermes", sc.SessionDirTemplate)
	}
	if sc.SessionDirTarget != "/root/.hermes" {
		t.Errorf("SessionDirTarget = %q, want /root/.hermes", sc.SessionDirTarget)
	}
}

func TestHermesACP_PermissionAndBillingDefaults(t *testing.T) {
	a := NewHermesACP()
	if len(a.PermissionSettings()) != 0 {
		t.Errorf("PermissionSettings() = %#v, want empty (agentctl auto-approve is authoritative)", a.PermissionSettings())
	}
	if got := a.BillingType(); got != usage.BillingTypeAPIKey {
		t.Errorf("BillingType() = %q, want %q", got, usage.BillingTypeAPIKey)
	}
	catalog := CatalogPermissionSettings(a)
	auto, ok := catalog[PermissionKeyAutoApprove]
	if !ok {
		t.Fatal("catalog missing auto_approve")
	}
	if auto.ApplyMethod != PermissionApplyMethodAgentctlAutoApprove {
		t.Errorf("auto_approve ApplyMethod = %q, want agentctl auto-approve", auto.ApplyMethod)
	}
}
