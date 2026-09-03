package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/pkg/agent"
)

// wantAntigravityArgv returns the expected argument vector for the current
// host platform, pinned independently of the production switch in
// antigravity_acp.go (AC-AGENTS-ANTIGRAVITY-ACP-003.1/.2).
func wantAntigravityArgv() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"agy_acp_server.exe"}
	case "linux":
		return []string{"agy_acp_server.par", "--uid="}
	default:
		return []string{"agy_acp_server.par"}
	}
}

func wantAntigravityBinaryName() string {
	if runtime.GOOS == "windows" {
		return "agy_acp_server.exe"
	}
	return "agy_acp_server.par"
}

func wantAntigravityHarnessNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"localharness_external.exe", "localharness.exe"}
	}
	return []string{"localharness_external", "localharness"}
}

func TestAntigravityACP_IDAndDisplay(t *testing.T) {
	a := NewAntigravityACP()
	if got := a.ID(); got != "antigravity-acp" {
		t.Errorf("ID() = %q, want antigravity-acp", got)
	}
	if got := a.Name(); got != "Antigravity ACP Agent" {
		t.Errorf("Name() = %q, want %q", got, "Antigravity ACP Agent")
	}
	if got := a.DisplayName(); got != "Antigravity" {
		t.Errorf("DisplayName() = %q, want Antigravity", got)
	}
	if !a.Enabled() {
		t.Error("Enabled() = false, want true")
	}
	if got := a.DisplayOrder(); got != 22 {
		t.Errorf("DisplayOrder() = %d, want 22", got)
	}
	if agent.ProtocolACP != a.Runtime().Protocol {
		t.Errorf("Runtime.Protocol = %q, want ACP", a.Runtime().Protocol)
	}
}

// TestAntigravityACP_Description covers AC-AGENTS-ANTIGRAVITY-ACP-001.7: the
// description must carry install guidance (registry entry, the two archive
// entries extracting into one directory, PATH, platform executable name)
// since InstallScript() is empty, and must not embed a rotating archive URL
// or build stamp.
func TestAntigravityACP_Description(t *testing.T) {
	desc := NewAntigravityACP().Description()
	for _, want := range []string{"antigravity-acp", "PATH", "agy_acp_server.par", "agy_acp_server.exe"} {
		if !strings.Contains(desc, want) {
			t.Errorf("Description() = %q, want it to contain %q", desc, want)
		}
	}
	if strings.Contains(desc, "http") {
		t.Errorf("Description() = %q, must not embed an archive URL", desc)
	}
	if strings.Contains(desc, "RC01") || strings.Contains(desc, "20260818") {
		t.Errorf("Description() = %q, must not embed a rotating build stamp", desc)
	}
}

func TestAntigravityACP_NoCLIPassthrough(t *testing.T) {
	a := NewAntigravityACP()
	if _, ok := any(a).(PassthroughAgent); ok {
		t.Error("AntigravityACP must not implement PassthroughAgent (AC-AGENTS-ANTIGRAVITY-ACP-001.9)")
	}
}

// TestAntigravityACP_ArgvIsPlatformSpecific covers
// AC-AGENTS-ANTIGRAVITY-ACP-003.1, .2, .3, .4, .5, .6, .7 for the host
// platform running this test.
func TestAntigravityACP_ArgvIsPlatformSpecific(t *testing.T) {
	want := wantAntigravityArgv()
	a := NewAntigravityACP()

	assertArgvEqual(t, "BuildCommand", a.BuildCommand(CommandOptions{}).Args(), want)
	assertArgvEqual(t, "BuildCommand (repeat)", a.BuildCommand(CommandOptions{}).Args(), want)

	rt := a.Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	assertArgvEqual(t, "Runtime.Cmd", rt.Cmd.Args(), want)

	ic := a.InferenceConfig()
	if ic == nil || !ic.Supported {
		t.Fatalf("InferenceConfig() = %+v, want Supported=true", ic)
	}
	assertArgvEqual(t, "InferenceConfig.Command", ic.Command.Args(), want)

	// AC-003.6: argv must not vary with model, permission policy,
	// auto-approve, agent type, or resume target.
	varied := CommandOptions{
		Model:            "gemini-3-pro",
		SessionID:        "sess-123",
		AutoApprove:      true,
		PermissionPolicy: "supervised",
		AgentType:        "task",
	}
	assertArgvEqual(t, "BuildCommand with varied opts", a.BuildCommand(varied).Args(), want)
}

func TestAntigravityACP_RuntimeConfig(t *testing.T) {
	rt := NewAntigravityACP().Runtime()
	if rt.WorkingDir != "{workspace}" {
		t.Errorf("WorkingDir = %q, want {workspace}", rt.WorkingDir)
	}
	if len(rt.Env) != 0 {
		t.Errorf("Env = %#v, want empty (inherit execution environment unmodified)", rt.Env)
	}
	if len(rt.StripEnv) != 0 {
		t.Errorf("StripEnv = %#v, want empty", rt.StripEnv)
	}
	if rt.ResourceLimits.MemoryMB != 4096 || rt.ResourceLimits.CPUCores != 2.0 || rt.ResourceLimits.Timeout != time.Hour {
		t.Errorf("ResourceLimits = %+v, want {4096, 2.0, 1h}", rt.ResourceLimits)
	}
	if rt.RequiresProcessKill {
		t.Error("RequiresProcessKill = true, want false (no immediate kill on shutdown)")
	}
	if rt.ProjectSkillDir != ".agents/skills" {
		t.Errorf("ProjectSkillDir = %q, want .agents/skills", rt.ProjectSkillDir)
	}
	if rt.UserSkillDir != ".gemini/config/skills" {
		t.Errorf("UserSkillDir = %q, want .gemini/config/skills", rt.UserSkillDir)
	}

	sc := rt.SessionConfig
	if !sc.NativeSessionResume {
		t.Error("NativeSessionResume = false, want true")
	}
	if sc.CanRecover == nil || !*sc.CanRecover {
		t.Error("CanRecover must be true")
	}
	if sc.SessionDirTemplate != "{home}/.gemini/antigravity-acp" {
		t.Errorf("SessionDirTemplate = %q, want {home}/.gemini/antigravity-acp", sc.SessionDirTemplate)
	}
	if sc.SessionDirTarget != "" {
		t.Errorf("SessionDirTarget = %q, want empty (no container bind mount, AC-004.10)", sc.SessionDirTarget)
	}
}

func TestAntigravityACP_InstallScriptEmpty(t *testing.T) {
	got := NewAntigravityACP().InstallScript()
	if got != "" {
		t.Errorf("InstallScript() = %q, want empty string (AC-AGENTS-ANTIGRAVITY-ACP-007.1)", got)
	}
}

func TestAntigravityACP_RemoteAuth(t *testing.T) {
	auth := NewAntigravityACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil")
	}
	if len(auth.Methods) != 3 {
		t.Fatalf("Methods len = %d, want 3", len(auth.Methods))
	}

	files := auth.Methods[0]
	if files.Type != "files" {
		t.Errorf("Methods[0].Type = %q, want files", files.Type)
	}
	if files.TargetRelDir != ".gemini/antigravity-acp" {
		t.Errorf("TargetRelDir = %q, want .gemini/antigravity-acp", files.TargetRelDir)
	}
	wantFiles := []string{
		".gemini/antigravity-acp/settings.json",
		".gemini/antigravity-acp/acp_token.json",
		".gemini/antigravity-acp/acp_business_token.json",
	}
	for _, osName := range []string{"darwin", "linux"} {
		paths := files.SourceFiles[osName]
		if !slices.Equal(paths, wantFiles) {
			t.Errorf("SourceFiles[%s] = %#v, want %#v", osName, paths, wantFiles)
		}
	}

	env1 := auth.Methods[1]
	if env1.Type != "env" || env1.EnvVar != "GEMINI_API_KEY" {
		t.Errorf("Methods[1] = %+v, want env GEMINI_API_KEY", env1)
	}
	env2 := auth.Methods[2]
	if env2.Type != "env" || env2.EnvVar != "GOOGLE_API_KEY" {
		t.Errorf("Methods[2] = %+v, want env GOOGLE_API_KEY", env2)
	}
}

func TestAntigravityACP_LogosNonEmpty(t *testing.T) {
	a := NewAntigravityACP()
	if len(a.Logo(LogoLight)) == 0 {
		t.Error("Logo(LogoLight) is empty")
	}
	if len(a.Logo(LogoDark)) == 0 {
		t.Error("Logo(LogoDark) is empty")
	}
	if !strings.Contains(string(a.Logo(LogoLight)), "<svg") {
		t.Error("Logo(LogoLight) is not SVG")
	}
	// Any other variant falls back to the light logo (AC-001.5).
	if !slices.Equal(a.Logo(LogoVariant(99)), a.Logo(LogoLight)) {
		t.Error("unknown LogoVariant must fall back to the light logo")
	}
}

func TestAntigravityACP_DisplayOrderUnique(t *testing.T) {
	all := []Agent{
		NewClaudeACP(), NewCodexACP(), NewAuggie(), NewOpenCodeACP(),
		NewGemini(), NewCopilotACP(), NewAmpACP(), NewQwenACP(),
		NewIFlowACP(), NewDroidACP(), NewKilocodeACP(), NewPiACP(),
		NewCursorACP(), NewKimiACP(), NewKiroACP(), NewQoderACP(),
		NewTraeACP(), NewOmpACP(), NewDevinACP(), NewGrokACP(),
		NewHermesACP(), NewAntigravityACP(), NewMockAgent(),
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

func TestAntigravityACP_PermissionAndBillingDefaults(t *testing.T) {
	a := NewAntigravityACP()
	if len(a.PermissionSettings()) != 0 {
		t.Errorf("PermissionSettings() = %#v, want empty", a.PermissionSettings())
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

// --- Discovery / harness resolution (REQ-AGENTS-ANTIGRAVITY-ACP-002) ---

// writeFakeAntigravityBinary creates an empty regular file named for the
// host platform's Antigravity binary in dir, executable on unix.
func writeFakeAntigravityBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, wantAntigravityBinaryName())
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

func writeFakeHarness(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("harness"), 0o755); err != nil {
		t.Fatalf("write fake harness %q: %v", name, err)
	}
}

func TestAntigravityACP_DiscoveryRequiresGlobalBinary(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	result, err := NewAntigravityACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true without the binary on PATH")
	}
	if result.MatchedPath != "" {
		t.Errorf("MatchedPath = %q, want empty", result.MatchedPath)
	}
}

func TestAntigravityACP_DiscoveryFailsClosedWithoutHarness(t *testing.T) {
	binDir := t.TempDir()
	path := writeFakeAntigravityBinary(t, binDir)
	t.Setenv("PATH", binDir)
	t.Setenv("ANTIGRAVITY_HARNESS_PATH", "")

	result, err := NewAntigravityACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Errorf("Available=true with binary present but no harness sibling at %s", path)
	}
	if result.MatchedPath != "" {
		t.Errorf("MatchedPath = %q, want empty (AC-002.5 uses the AC-002.2 outcome)", result.MatchedPath)
	}
}

func TestAntigravityACP_DiscoveryAvailableWithHarnessSibling(t *testing.T) {
	for _, harnessName := range wantAntigravityHarnessNames() {
		t.Run(harnessName, func(t *testing.T) {
			binDir := t.TempDir()
			path := writeFakeAntigravityBinary(t, binDir)
			writeFakeHarness(t, binDir, harnessName)
			t.Setenv("PATH", binDir)

			result, err := NewAntigravityACP().IsInstalled(context.Background())
			if err != nil {
				t.Fatalf("IsInstalled error: %v", err)
			}
			if !result.Available {
				t.Fatal("Available=false with binary and harness both present")
			}
			resolved, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil {
				resolved = path
			}
			gotResolved, gotErr := filepath.EvalSymlinks(result.MatchedPath)
			if gotErr != nil {
				gotResolved = result.MatchedPath
			}
			if gotResolved != resolved {
				t.Errorf("MatchedPath = %q, want %q", result.MatchedPath, path)
			}
			if !result.SupportsMCP {
				t.Error("SupportsMCP = false, want true")
			}
			if !result.Capabilities.SupportsSessionResume {
				t.Error("SupportsSessionResume = false, want true")
			}
		})
	}
}

func TestAntigravityACP_DiscoveryHarnessMustBeRegularFile(t *testing.T) {
	binDir := t.TempDir()
	writeFakeAntigravityBinary(t, binDir)
	// A directory named like the harness must not count as a match.
	if err := os.Mkdir(filepath.Join(binDir, wantAntigravityHarnessNames()[0]), 0o755); err != nil {
		t.Fatalf("mkdir fake harness dir: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ANTIGRAVITY_HARNESS_PATH", "")

	result, err := NewAntigravityACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true when the harness name resolves to a directory, not a regular file")
	}
}

func TestAntigravityACP_DiscoveryRejectsNonExecutableHarness(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix execute bits do not control windows executability")
	}

	binDir := t.TempDir()
	writeFakeAntigravityBinary(t, binDir)
	if err := os.WriteFile(
		filepath.Join(binDir, wantAntigravityHarnessNames()[0]),
		[]byte("harness"),
		0o644,
	); err != nil {
		t.Fatalf("write non-executable fake harness: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ANTIGRAVITY_HARNESS_PATH", "")

	result, err := NewAntigravityACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true when the harness file has no execute permission")
	}
}

func TestAntigravityACP_DiscoveryHarnessDanglingSymlinkDoesNotMatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevated privileges on windows")
	}
	binDir := t.TempDir()
	writeFakeAntigravityBinary(t, binDir)
	harnessPath := filepath.Join(binDir, wantAntigravityHarnessNames()[0])
	if err := os.Symlink(filepath.Join(binDir, "does-not-exist"), harnessPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ANTIGRAVITY_HARNESS_PATH", "")

	result, err := NewAntigravityACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if result.Available {
		t.Error("Available=true with a dangling symlink standing in for the harness")
	}
}

func TestAntigravityACP_DiscoveryHarnessPathEnvBypassesCheck(t *testing.T) {
	binDir := t.TempDir()
	writeFakeAntigravityBinary(t, binDir)
	t.Setenv("PATH", binDir)
	t.Setenv("ANTIGRAVITY_HARNESS_PATH", filepath.Join(t.TempDir(), "harness-elsewhere"))

	result, err := NewAntigravityACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled error: %v", err)
	}
	if !result.Available {
		t.Error("Available=false even though ANTIGRAVITY_HARNESS_PATH satisfies the harness check")
	}
}

func TestAntigravityACP_DiscoveryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := NewAntigravityACP().IsInstalled(ctx)
	if err == nil {
		t.Fatal("IsInstalled with a cancelled context returned no error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if result != nil && result.Available {
		t.Error("a cancelled discovery must not report the agent as available")
	}
}

func TestAntigravityACP_DiscoveryDeterministic(t *testing.T) {
	binDir := t.TempDir()
	writeFakeAntigravityBinary(t, binDir)
	writeFakeHarness(t, binDir, wantAntigravityHarnessNames()[0])
	t.Setenv("PATH", binDir)

	a := NewAntigravityACP()
	first, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("first IsInstalled error: %v", err)
	}
	second, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("second IsInstalled error: %v", err)
	}
	if first.Available != second.Available || first.MatchedPath != second.MatchedPath {
		t.Errorf("repeated calls diverged: %+v vs %+v", first, second)
	}
}
