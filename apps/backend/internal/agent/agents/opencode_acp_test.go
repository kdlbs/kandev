package agents

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestOpenCodeACPUsesManagedRuntime(t *testing.T) {
	// Pin PATH to an empty dir so the executor-safe configuration is
	// deterministic.
	t.Setenv("PATH", t.TempDir())
	a := NewOpenCodeACP()
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
	if got, wantInstall := a.InstallScript(), "npm install -g opencode-ai"; got != wantInstall {
		t.Fatalf("InstallScript = %q, want %q", got, wantInstall)
	}
}

// TestOpenCodeACPBuildCommandPrefersNativeBinary is the CodeNomad-style
// binary-first launch: when the lifecycle probe finds `opencode` on PATH,
// BuildCommand emits the direct binary instead of the per-launch npx
// resolution. It is the regression test for "ACP initialize failed: peer
// disconnected before response", which happened because npx exited with
// empty stdout on a stale packument cache / missing postinstall bootstrap.
func TestOpenCodeACPBuildCommandPrefersNativeBinary(t *testing.T) {
	a := NewOpenCodeACP()

	if name := a.NativeBinaryName(); name != "opencode" {
		t.Fatalf("NativeBinaryName() = %q, want %q", name, "opencode")
	}

	wantNative := []string{"opencode", "acp", "--print-logs"}
	if got := a.BuildCommand(CommandOptions{PreferNativeBinary: true}).Args(); !slices.Equal(got, wantNative) {
		t.Fatalf("BuildCommand(PreferNativeBinary) = %#v, want %#v", got, wantNative)
	}

	// Default (probe absent / containers / remotes) keeps the managed npx runtime.
	if got := a.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, a.ManagedNPMRuntime().CachedACPCommand().Args()) {
		t.Fatalf("BuildCommand(default) = %#v, want npx fallback", got)
	}
}

func TestOpenCodeACPCommandsAcceptBothLogLevelDialects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture is not executable on Windows")
	}

	args := NewOpenCodeACP().BuildCommand(CommandOptions{PreferNativeBinary: true}).Args()
	if !slices.Equal(args, []string{"opencode", "acp", "--print-logs"}) {
		t.Fatalf("native command = %v, want [opencode acp --print-logs]", args)
	}

	managedArgs := NewOpenCodeACP().ManagedNPMRuntime().CachedACPCommand().Args()
	if slices.Contains(managedArgs, "--log-level") {
		t.Fatalf("managed command %v contains optional --log-level", managedArgs)
	}

	for _, body := range []string{
		`for arg in "$@"; do
    if [ "$arg" = "ERROR" ]; then
        echo "Invalid value for flag --log-level: ERROR" >&2
        exit 1
    fi
done
exit 0`,
		`exit 0`,
	} {
		writeOpenCodeTestBinary(t, body)
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("OpenCode rejected command %v: %v (output: %s)", args, err, output)
		}
	}
}

func TestOpenCodeRuntimeFamiliesUseTrustedPackagesAndArguments(t *testing.T) {
	tests := []struct {
		name        string
		family      managedruntime.OpenCodeFamily
		packageName string
		version     string
	}{
		{name: "v1", family: managedruntime.OpenCodeFamilyV1, packageName: "opencode-ai", version: "1.18.32"},
		{name: "v2", family: managedruntime.OpenCodeFamilyV2, packageName: "@opencode/cli", version: "2.0.18"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := NewOpenCodeACP().ManagedNPMRuntimeForFamily(tt.family)
			if err != nil {
				t.Fatalf("ManagedNPMRuntimeForFamily: %v", err)
			}
			if spec.Package != tt.packageName || spec.DefaultVersion != tt.version {
				t.Fatalf("runtime spec = %+v", spec)
			}
			want := []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", tt.packageName + "@" + tt.version, "acp", "--print-logs"}
			got := NewOpenCodeACP().BuildCommand(CommandOptions{
				ManagedRuntimeFamily:  tt.family,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceManaged,
				ManagedRuntimeVersion: tt.version,
				PreferNativeBinary:    true,
			}).Args()
			if !slices.Equal(got, want) {
				t.Fatalf("managed family command = %#v, want %#v", got, want)
			}
		})
	}
}

func TestOpenCodeNativeCommandUsesObservedMajorArguments(t *testing.T) {
	for _, version := range []string{"1.18.5", "2.0.18"} {
		t.Run(version, func(t *testing.T) {
			got := NewOpenCodeACP().BuildCommand(CommandOptions{
				PreferNativeBinary:    true,
				ManagedRuntimeFamily:  managedruntime.OpenCodeFamilyV1,
				ManagedRuntimeSource:  managedruntime.OpenCodeSourceNative,
				NativeRuntimeVersion:  version,
				ManagedRuntimeVersion: "1.18.32",
			}).Args()
			want := []string{"opencode", "acp", "--print-logs"}
			if !slices.Equal(got, want) {
				t.Fatalf("native command = %#v, want %#v", got, want)
			}
		})
	}
}

func TestOpenCodePassthroughUsesSelectedManagedCommand(t *testing.T) {
	got := NewOpenCodeACP().BuildPassthroughCommand(PassthroughOptions{
		BaseCommand: NewCommand("npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, "@opencode/cli@2.0.18"),
	}).Args()
	want := []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, "@opencode/cli@2.0.18"}
	if !slices.Equal(got, want) {
		t.Fatalf("passthrough command = %#v, want %#v", got, want)
	}
}

// TestOpenCodeACPInferenceConfigPrefersNativeBinaryOnPath pins the host-utility
// bootstrap to the standalone binary when it is installed, so the ACP probe
// never depends on npm cache state.
func TestOpenCodeACPInferenceConfigPrefersNativeBinaryOnPath(t *testing.T) {
	writeOpenCodeTestBinary(t, "exit 0")

	a := NewOpenCodeACP()
	want := []string{"opencode", "acp", "--print-logs"}
	if got := a.HostUtilityInferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("HostUtilityInferenceConfig().Command = %#v, want %#v", got, want)
	}
}

func TestOpenCodeACPInferenceConfigStaysExecutorSafeOnHostPath(t *testing.T) {
	writeOpenCodeTestBinary(t, "exit 0")

	a := NewOpenCodeACP()
	want := a.ManagedNPMRuntime().CachedACPCommand().Args()
	if got := a.InferenceConfig().Command.Args(); !slices.Equal(got, want) {
		t.Fatalf("InferenceConfig().Command = %#v, want executor-safe npx command %#v", got, want)
	}
}

func TestOpenCodeACPDiscoveryRecognizesAuthenticationHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture is not executable on Windows")
	}
	binaryPath := writeOpenCodeTestBinary(t, "exit 7")

	a := NewOpenCodeACP()
	result, err := a.IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !result.Available {
		t.Fatal("IsInstalled() Available = false, want true")
	}
	if result.MatchedPath != binaryPath {
		t.Fatalf("IsInstalled() MatchedPath = %q, want %q", result.MatchedPath, binaryPath)
	}
}

func TestOpenCodeACPDiscoveryDoesNotRunVersionCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fixture is not executable on Windows")
	}
	writeOpenCodeTestBinary(t, "exit 7")

	result, err := NewOpenCodeACP().IsInstalled(context.Background())
	if err != nil {
		t.Fatalf("IsInstalled() error = %v", err)
	}
	if !result.Available {
		t.Fatal("IsInstalled() Available = false, want true")
	}
}

func TestDetectOpenCodeNativeRuntimeSelectsSupportedFamily(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantFamily  managedruntime.OpenCodeFamily
		wantVersion string
	}{
		{name: "v1", output: "opencode 1.18.5", wantFamily: managedruntime.OpenCodeFamilyV1, wantVersion: "1.18.5"},
		{name: "v2", output: "opencode 2.0.18", wantFamily: managedruntime.OpenCodeFamilyV2, wantVersion: "2.0.18"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeOpenCodeTestBinary(t, "printf '%s\\n' '"+tt.output+"'")
			got, found, err := DetectOpenCodeNativeRuntime(context.Background())
			if err != nil {
				t.Fatalf("DetectOpenCodeNativeRuntime: %v", err)
			}
			if !found || got.Family != tt.wantFamily || got.Version != tt.wantVersion {
				t.Fatalf("native runtime = %+v, found %v; want %s %s", got, found, tt.wantFamily, tt.wantVersion)
			}
		})
	}
}

func TestDetectOpenCodeNativeRuntimeRejectsUnknownAndFailedVersions(t *testing.T) {
	for _, body := range []string{"printf 'opencode 3.0.0\\n'", "exit 1"} {
		t.Run(body, func(t *testing.T) {
			writeOpenCodeTestBinary(t, body)
			if _, found, err := DetectOpenCodeNativeRuntime(context.Background()); !found || err == nil {
				t.Fatalf("DetectOpenCodeNativeRuntime = found %v, err %v; want a found binary and compatibility error", found, err)
			}
		})
	}
}

func TestDetectOpenCodeNativeRuntimeAllowsNoBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got, found, err := DetectOpenCodeNativeRuntime(context.Background()); err != nil || found || got != (OpenCodeNativeRuntime{}) {
		t.Fatalf("DetectOpenCodeNativeRuntime = %+v, found %v, err %v; want absent without error", got, found, err)
	}
}

func writeOpenCodeTestBinary(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, agentTestExecutableName("opencode"))
	contents := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(binaryPath, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}
	t.Setenv("PATH", dir)
	return binaryPath
}

func agentTestExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// TestOpenCodeACPRuntime_RequiresProcessKill is the regression test for GH
// issue #1247: opencode acp keeps its HTTP server + MCP child tree alive
// when stdin closes, so its RuntimeConfig must signal that the process
// group should be reaped immediately. Without this flag the ACP adapter
// returns RequiresProcessKill=false and the process manager waits for the
// graceful EOF path before it falls back to process-group cleanup.
func TestOpenCodeACPRuntime_RequiresProcessKill(t *testing.T) {
	rt := NewOpenCodeACP().Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if !rt.RequiresProcessKill {
		t.Error("RequiresProcessKill = false; opencode acp must opt into process-group kill")
	}
}

// TestACPAgents_DefaultProcessKill confirms the rest of the ACP agents
// stick with the default (false). They communicate over plain stdin/stdout
// and should get a short graceful EOF path before the process manager reaps
// any remaining process-group descendants.
func TestACPAgents_DefaultProcessKill(t *testing.T) {
	cases := []struct {
		name  string
		agent Agent
	}{
		{"claude", NewClaudeACP()},
		{"codex", NewCodexACP()},
		{"cursor", NewCursorACP()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := tc.agent.Runtime()
			if rt == nil {
				t.Fatalf("%s Runtime() returned nil", tc.name)
			}
			if rt.RequiresProcessKill {
				t.Errorf("%s RequiresProcessKill = true; expected default false", tc.name)
			}
		})
	}
}

func TestOpenCodeACPRemoteAuth(t *testing.T) {
	auth := NewOpenCodeACP().RemoteAuth()
	if auth == nil {
		t.Fatal("RemoteAuth() returned nil; expected files-based auth method")
	}
	if len(auth.Methods) != 1 {
		t.Fatalf("Methods len = %d, want 1", len(auth.Methods))
	}
	m := auth.Methods[0]
	if m.Type != "files" {
		t.Errorf("Type = %q, want %q", m.Type, "files")
	}
	if m.TargetRelDir != ".local/share/opencode" {
		t.Errorf("TargetRelDir = %q, want %q", m.TargetRelDir, ".local/share/opencode")
	}
	if m.FileConflictPolicy != RemoteAuthFileConflictPolicyMergeJSONObject {
		t.Errorf("FileConflictPolicy = %q, want %q", m.FileConflictPolicy, RemoteAuthFileConflictPolicyMergeJSONObject)
	}
	want := []string{".local/share/opencode/auth.json"}
	for _, os := range []string{"darwin", "linux"} {
		got := m.SourceFiles[os]
		if !slices.Equal(got, want) {
			t.Errorf("SourceFiles[%q] = %v, want %v", os, got, want)
		}
	}
}
