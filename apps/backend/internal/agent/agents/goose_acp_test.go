package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

func TestGooseACP_IDAndDisplay(t *testing.T) {
	a := NewGooseACP()
	if got := a.ID(); got != "goose-acp" {
		t.Errorf("ID() = %q, want goose-acp", got)
	}
	if got := a.Name(); got != "Goose ACP Agent" {
		t.Errorf("Name() = %q, want Goose ACP Agent", got)
	}
	if got := a.DisplayName(); got != "Goose" {
		t.Errorf("DisplayName() = %q, want Goose", got)
	}
	if !a.Enabled() {
		t.Error("Enabled() = false, want true")
	}
	if got := a.DisplayOrder(); got != 22 {
		t.Errorf("DisplayOrder() = %d, want 22", got)
	}
	if got := a.Description(); !strings.Contains(got, "goose acp") {
		t.Errorf("Description should mention the ACP launch command, got %q", got)
	}
}

func TestGooseACP_AllCommandSurfaces(t *testing.T) {
	a := NewGooseACP()
	want := []string{"goose", "acp"}

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
		t.Fatal("GooseACP must implement PassthroughAgent")
	}
	assertArgvEqual(t, "PassthroughCmd", pa.PassthroughConfig().PassthroughCmd.Args(), []string{"goose"})
}

func TestGooseACP_DoesNotForceSSE(t *testing.T) {
	rt := NewGooseACP().Runtime()
	// Goose supports Stdio + HTTP (StreamableHttp) MCP but rejects SSE; forcing
	// SSE would send a server Goose refuses to consume.
	if rt.AssumeMcpSse {
		t.Error("AssumeMcpSse = true, want false (Goose rejects SSE MCP)")
	}
}

func TestGooseACP_EnvAndStripEnv(t *testing.T) {
	rt := NewGooseACP().Runtime()
	// Goose authenticates via ~/.config/goose config files, not provider
	// API-key env vars, so there is no behavior-changing env to set or strip.
	if len(rt.Env) != 0 {
		t.Errorf("Runtime Env = %#v, want empty (no skip/env knob exists; global extensions still load)", rt.Env)
	}
	if len(rt.StripEnv) != 0 {
		t.Errorf("Runtime StripEnv = %#v, want empty (Goose reads config files, not provider env vars)", rt.StripEnv)
	}
}

func TestGooseACP_InstallScript(t *testing.T) {
	got := NewGooseACP().InstallScript()
	for _, needle := range []string{
		"set -eu",
		`tmp="$(mktemp)"`,
		`trap 'rm -f "$tmp"' EXIT`,
		"curl -fsSL https://github.com/aaif-goose/goose/releases/download/stable/download_cli.sh",
		`-o "$tmp"`,
		"install_dir=",
		`GOOSE_BIN_DIR="$install_dir" CONFIGURE=false bash "$tmp"`,
		`command -v goose`,
		"goose",
	} {
		if !strings.Contains(got, needle) {
			t.Errorf("InstallScript missing %q: %q", needle, got)
		}
	}
	if strings.HasPrefix(got, "npm install -g ") {
		t.Errorf("InstallScript should use the native Goose installer, got npm script: %q", got)
	}
	if !strings.Contains(got, "CONFIGURE=false") {
		t.Errorf("InstallScript should set CONFIGURE=false for deterministic non-interactive install, got %q", got)
	}
}

func TestGooseACPInstallScriptUsesFirstWritablePathDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}

	home := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}

	fakeCurl := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
cat > "$out" <<'INSTALL'
#!/bin/sh
set -eu
mkdir -p "$GOOSE_BIN_DIR"
cat > "$GOOSE_BIN_DIR/goose" <<'GOOSE'
#!/bin/sh
echo "Run goose as an ACP agent server on stdio"
GOOSE
chmod +x "$GOOSE_BIN_DIR/goose"
INSTALL
`
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	cmd := exec.Command("sh", "-c", NewGooseACP().InstallScript())
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + binDir + ":/usr/bin:/bin",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("InstallScript failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(binDir, "goose")); err != nil {
		t.Fatalf("expected goose binary in first writable PATH directory: %v", err)
	}
}

func TestGooseACPInstallScriptPropagatesDownloadFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install script is POSIX shell")
	}

	binDir := t.TempDir()
	fakeCurl := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}

	cmd := exec.Command("sh", "-c", NewGooseACP().InstallScript())
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + binDir + ":/usr/bin:/bin",
	}
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("InstallScript succeeded after curl failure; output: %s", out)
	}
}

func TestGooseACP_DetectionRequiresGlobalBinary(t *testing.T) {
	if _, err := exec.LookPath("goose"); err == nil {
		t.Skip("detection binary \"goose\" is on PATH; can't verify availability requirement")
	}
	result, err := NewGooseACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true without goose on PATH; discovery must not imply install")
	}
}

func TestGooseACP_DetectionRejectsUnrelatedGooseBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture is Unix-specific")
	}

	binDir := t.TempDir()
	goosePath := filepath.Join(binDir, gooseBin)
	// Pressly's database-migration CLI also installs a `goose` binary. It may
	// answer generic version requests successfully, but it has no Goose ACP
	// help surface.
	script := "#!/bin/sh\necho 'goose version v3.24.3'\n"
	if err := os.WriteFile(goosePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write unrelated goose executable: %v", err)
	}
	t.Setenv("PATH", binDir)

	result, err := NewGooseACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Fatal("Available=true for unrelated goose binary without ACP help")
	}
}

func TestGooseACP_DetectionAcceptsACPHelp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell executable fixture is Unix-specific")
	}

	binDir := t.TempDir()
	goosePath := filepath.Join(binDir, gooseBin)
	script := "#!/bin/sh\necho 'Run goose as an ACP agent server on stdio'\n"
	if err := os.WriteFile(goosePath, []byte(script), 0o755); err != nil {
		t.Fatalf("write Goose executable: %v", err)
	}
	t.Setenv("PATH", binDir)

	result, err := NewGooseACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if !result.Available {
		t.Fatal("Available=false for Goose binary with ACP help")
	}
	if result.MatchedPath != goosePath {
		t.Errorf("MatchedPath = %q, want %q", result.MatchedPath, goosePath)
	}
}

func TestGooseACP_SupportsMCPAndResume(t *testing.T) {
	if _, err := exec.LookPath("goose"); err != nil {
		t.Skip("goose not on PATH; MCP/resume capabilities surface only when detected")
	}
	result, err := NewGooseACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if !result.SupportsMCP {
		t.Error("SupportsMCP = false, want true (session/new mcpServers)")
	}
	if !result.Capabilities.SupportsSessionResume {
		t.Error("SupportsSessionResume = false, want true (session/load)")
	}
}

func TestGooseACP_LogosNonEmpty(t *testing.T) {
	a := NewGooseACP()
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

func TestGooseACP_RemoteAuth(t *testing.T) {
	auth := NewGooseACP().RemoteAuth()
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
	if files.TargetRelDir != ".config/goose" {
		t.Errorf("TargetRelDir = %q, want .config/goose", files.TargetRelDir)
	}
	for _, osName := range []string{"darwin", "linux"} {
		paths := files.SourceFiles[osName]
		want := []string{".config/goose/config.yaml", ".config/goose/extensions.yaml", ".config/goose/secrets.yaml"}
		if !slices.Equal(paths, want) {
			t.Errorf("SourceFiles[%s] = %#v, want %#v", osName, paths, want)
		}
	}
}

func TestGooseACP_LoginCommand(t *testing.T) {
	cmd := NewGooseACP().LoginCommand()
	if cmd == nil {
		t.Fatal("LoginCommand() returned nil")
	}
	want := []string{"goose", "configure"}
	if !slices.Equal(cmd.Cmd, want) {
		t.Errorf("LoginCommand.Cmd = %#v, want %#v", cmd.Cmd, want)
	}
	if cmd.Description == "" {
		t.Error("LoginCommand.Description is empty")
	}
}

func TestGooseACP_SessionConfig(t *testing.T) {
	rt := NewGooseACP().Runtime()
	if rt.WorkingDir != "{workspace}" {
		t.Errorf("WorkingDir = %q, want {workspace}", rt.WorkingDir)
	}
	sc := rt.SessionConfig
	if !sc.NativeSessionResume {
		t.Error("NativeSessionResume = false, want true")
	}
	if sc.NewSessionOnWorkspaceRebind {
		t.Error("NewSessionOnWorkspaceRebind = true, want false")
	}
	if sc.CanRecover == nil || !*sc.CanRecover {
		t.Error("CanRecover must be true")
	}
	if sc.SessionDirTemplate != "{home}/.local/share/goose" {
		t.Errorf("SessionDirTemplate = %q, want {home}/.local/share/goose", sc.SessionDirTemplate)
	}
	if sc.SessionDirTarget != "/root/.local/share/goose" {
		t.Errorf("SessionDirTarget = %q, want /root/.local/share/goose", sc.SessionDirTarget)
	}
}

func TestGooseACP_PermissionAndBillingDefaults(t *testing.T) {
	a := NewGooseACP()
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
