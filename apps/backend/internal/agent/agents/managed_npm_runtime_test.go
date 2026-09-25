package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestManagedNPMRuntimeContracts(t *testing.T) {
	tests := []struct {
		name        string
		agent       ManagedNPMRuntimeAgent
		wantPackage string
		wantACPArgs []string
	}{
		{"claude", NewClaudeACP(), "@agentclientprotocol/claude-agent-acp", nil},
		{"codex", NewCodexACP(), "@agentclientprotocol/codex-acp", nil},
		{"opencode", NewOpenCodeACP(), "opencode-ai", []string{"acp", "--print-logs", "--log-level", "ERROR"}},
		{"copilot", NewCopilotACP(), "@github/copilot", []string{"--acp"}},
		{"gemini", NewGemini(), "@google/gemini-cli", []string{"--acp"}},
		{"pi", NewPiACP(), "pi-acp", nil},
		{"muse", NewMuseACP(), "@bex-co/muse-code-acp", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.agent.ManagedNPMRuntime()
			wantDefault := spec.DefaultVersionOrPinned()
			if wantDefault == "" {
				t.Fatal("DefaultVersionOrPinned() is empty")
			}
			if got := spec.Package; got != tt.wantPackage {
				t.Fatalf("Package = %q, want %q", got, tt.wantPackage)
			}
			if got := spec.DefaultVersion; got != wantDefault {
				t.Fatalf("DefaultVersion = %q, want %q", got, wantDefault)
			}
			if got := spec.ACPArgs; !slices.Equal(got, tt.wantACPArgs) {
				t.Fatalf("ACPArgs = %#v, want %#v", got, tt.wantACPArgs)
			}

			cached := spec.CachedACPCommand()
			wantCached := append([]string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", tt.wantPackage + "@" + wantDefault}, tt.wantACPArgs...)
			if !slices.Equal(cached.Args(), wantCached) {
				t.Fatalf("CachedACPCommand = %#v, want %#v", cached.Args(), wantCached)
			}
			if got := spec.PackageSpec(""); got != tt.wantPackage+"@"+wantDefault {
				t.Fatalf("empty PackageSpec = %q, want exact default", got)
			}

			update := spec.CacheUpdateCommand()
			wantUpdate := []string{
				"npm", "--prefix", "~/.kandev/managed-npm-runtime", "exec", "--yes", "--prefer-online",
				"--package=" + tt.wantPackage + "@" + wantDefault, "--", "node", "-e", "",
			}
			if !slices.Equal(update.Args(), wantUpdate) {
				t.Fatalf("CacheUpdateCommand = %#v, want %#v", update.Args(), wantUpdate)
			}
			if strings.Contains(strings.Join(update.Args(), " "), "latest") {
				t.Fatalf("CacheUpdateCommand contains explicit latest: %#v", update.Args())
			}
		})
	}
}

func TestManagedNPMRuntimeExecutionCacheKeyMatchesNPM(t *testing.T) {
	spec := ManagedNPMRuntimeSpec{Package: "opencode-ai"}
	want := managedruntime.NpxExecutionCacheKey(spec.PackageSpec(""))
	if got := spec.ExecutionCacheKey(); got != want {
		t.Fatalf("ExecutionCacheKey = %q, want npm key %q", got, want)
	}
}

func TestManagedNPMRuntimeDefaultVersionOrPinned(t *testing.T) {
	tests := []struct {
		name string
		spec ManagedNPMRuntimeSpec
		want string
	}{
		{name: "explicit default", spec: ManagedNPMRuntimeSpec{Package: "@scope/managed", DefaultVersion: "1.2.3"}, want: "1.2.3"},
		{name: "built-in catalogue fallback", spec: ManagedNPMRuntimeSpec{Package: "opencode-ai"}, want: MustDefaultManagedNPMRuntimeVersion("opencode-ai")},
		{name: "custom package without default", spec: ManagedNPMRuntimeSpec{Package: "@scope/managed"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.DefaultVersionOrPinned(); got != tt.want {
				t.Fatalf("DefaultVersionOrPinned() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManagedNPMRuntimeBuildsExactVersionCommandsAndCacheKey(t *testing.T) {
	spec := ManagedNPMRuntimeSpec{
		Package: "opencode-ai",
		ACPArgs: []string{"acp", "--print-logs"},
	}
	wantACP := []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "opencode-ai@1.18.5", "acp", "--print-logs"}
	if got := spec.ACPCommand("1.18.5").Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("ACPCommand = %#v, want %#v", got, wantACP)
	}
	wantUpdate := []string{
		"npm", "--prefix", "~/.kandev/managed-npm-runtime", "exec", "--yes", "--prefer-online",
		"--package=opencode-ai@1.18.5", "--", "node", "-e", "",
	}
	if got := spec.CacheUpdateCommand("1.18.5").Args(); !slices.Equal(got, wantUpdate) {
		t.Fatalf("CacheUpdateCommand = %#v, want %#v", got, wantUpdate)
	}
	if got := spec.ExecutionCacheKey("1.18.5"); got != "cd439a892fc193b3" {
		t.Fatalf("versioned ExecutionCacheKey = %q, want cd439a892fc193b3", got)
	}
}

func TestManagedNPMRuntimeOnlineCommandChangesOnlyNpmFreshnessFlag(t *testing.T) {
	spec := ManagedNPMRuntimeSpec{
		Package: "@scope/managed-acp",
		ACPArgs: []string{"--acp", "--model", "fast"},
	}
	offline := spec.ACPCommand("1.2.3").Args()
	online := spec.ACPCommandWithNpmPreference("1.2.3", true).Args()
	want := []string{"npx", "--yes", "--prefer-online", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}
	if !reflect.DeepEqual(online, want) {
		t.Fatalf("online argv = %#v, want %#v", online, want)
	}
	offline[2] = "--prefer-online"
	if !reflect.DeepEqual(offline, online) {
		t.Fatalf("online command changed more than npm preference: offline=%#v online=%#v", offline, online)
	}
}

func TestManagedNPMRuntimeExactVersionSupportsScopedPackages(t *testing.T) {
	spec := ManagedNPMRuntimeSpec{Package: "@scope/managed-acp", ACPArgs: []string{"--acp"}}
	want := []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/managed-acp@3.4.5", "--acp"}
	if got := spec.ACPCommand("3.4.5").Args(); !slices.Equal(got, want) {
		t.Fatalf("scoped ACPCommand = %#v, want %#v", got, want)
	}
	if spec.ExecutionCacheKey("3.4.5") == spec.ExecutionCacheKey() {
		t.Fatal("versioned scoped cache key equals legacy key")
	}
}

func TestManagedAgentsHonorExactVersionCommandOption(t *testing.T) {
	tests := []struct {
		name  string
		agent ManagedNPMRuntimeAgent
	}{
		{"claude", NewClaudeACP()},
		{"codex", NewCodexACP()},
		{"opencode", NewOpenCodeACP()},
		{"copilot", NewCopilotACP()},
		{"gemini", NewGemini()},
		{"pi", NewPiACP()},
		{"muse", NewMuseACP()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := "1.2.3"
			want := tt.agent.ManagedNPMRuntime().ACPCommand(version).Args()
			got := tt.agent.(interface {
				BuildCommand(CommandOptions) Command
			}).BuildCommand(CommandOptions{ManagedRuntimeVersion: version}).Args()
			if !slices.Equal(got, want) {
				t.Fatalf("exact BuildCommand = %#v, want %#v", got, want)
			}
		})
	}
}

func TestManagedNPMRuntimeNativeBinaryPreference(t *testing.T) {
	nativeBin := "opencode"
	ab := ManagedNPMRuntimeSpec{
		Package:        "opencode-ai",
		DefaultVersion: "1.2.3",
		ACPArgs:        []string{"acp", "--print-logs", "--log-level", "ERROR"},
		NativeBinary:   nativeBin,
	}
	t.Setenv("PATH", t.TempDir())
	if ab.NativeBinaryOnPath() {
		t.Fatal("NativeBinaryOnPath() = true with empty PATH dir, want false")
	}
	wantNpx := []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "opencode-ai@1.2.3", "acp", "--print-logs", "--log-level", "ERROR"}
	if got := ab.RefreshCommand().Args(); !slices.Equal(got, wantNpx) {
		t.Fatalf("RefreshCommand (absent binary) = %#v, want %#v", got, wantNpx)
	}

	dir := t.TempDir()
	name := nativeBin
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	t.Setenv("PATH", dir)
	if !ab.NativeBinaryOnPath() {
		t.Fatal("NativeBinaryOnPath() = false with binary on PATH, want true")
	}
	wantNative := []string{"opencode", "acp", "--print-logs", "--log-level", "ERROR"}
	if got := ab.NativeCommand().Args(); !slices.Equal(got, wantNative) {
		t.Fatalf("NativeCommand() = %#v, want %#v", got, wantNative)
	}
	if got := ab.UpdateCommand("2.0.0").Args(); !slices.Equal(got, []string{"npm", "install", "-g", "opencode-ai@2.0.0"}) {
		t.Fatalf("UpdateCommand() = %#v, want native npm install", got)
	}
	if got := ab.RefreshCommand().Args(); !slices.Equal(got, wantNative) {
		t.Fatalf("RefreshCommand (present binary) = %#v, want %#v", got, wantNative)
	}

	plain := ManagedNPMRuntimeSpec{Package: "@scope/custom", ACPArgs: []string{"acp"}}
	if got := plain.RefreshCommand().Args(); !slices.Equal(got, []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@scope/custom", "acp"}) {
		t.Fatalf("RefreshCommand (no native) = %#v, want npx", got)
	}
	if got := ab.ExecutionCacheKey(); got != managedruntime.NpxExecutionCacheKey("opencode-ai@1.2.3") {
		t.Fatalf("ExecutionCacheKey with NativeBinary = %q, want package key", got)
	}
}

func TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc(t *testing.T) {
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		t.Skip("npm is not installed")
	}

	home := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, ".npmrc"), []byte("min-release-age=87600\nregistry=https://registry.invalid/\n"), 0o600); err != nil {
		t.Fatalf("write workspace npmrc: %v", err)
	}

	readConfig := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(npmPath, args...)
		cmd.Dir = workspace
		cmd.Env = envWithHome(os.Environ(), home)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("npm %v: %v (%s)", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}

	if got := readConfig("config", "get", "registry"); got != "https://registry.invalid/" {
		t.Fatalf("workspace registry = %q, want configured registry", got)
	}
	workspaceReleaseAge := readConfig("config", "get", "min-release-age")

	args := NewOpenCodeACP().ManagedNPMRuntime().ACPCommand("1.18.5").Args()
	if !slices.Contains(args, "--prefix") {
		t.Fatalf("managed launch args = %#v, missing isolated npm prefix", args)
	}
	prefixIndex := slices.Index(args, "--prefix")
	if prefixIndex < 0 || prefixIndex+1 >= len(args) || args[prefixIndex+1] != "~/.kandev/managed-npm-runtime" {
		t.Fatalf("managed launch prefix = %#v, want executor-local home prefix", args)
	}

	isolatedArgs := append([]string(nil), args[prefixIndex:prefixIndex+2]...)
	if err := managedruntime.PrepareNPMProjectPrefix(isolatedArgs); err != nil {
		t.Fatalf("prepare managed npm prefix: %v", err)
	}
	prefix := isolatedArgs[1]
	if !filepath.IsAbs(prefix) || !strings.HasPrefix(filepath.Clean(prefix), filepath.Clean(os.TempDir())+string(filepath.Separator)) {
		t.Fatalf("managed npm prefix = %q, want an absolute path under %q", prefix, os.TempDir())
	}
	isolatedArgs = append(isolatedArgs, "config", "get", "min-release-age")
	if got := readConfig(isolatedArgs...); workspaceReleaseAge == "87600" && got == workspaceReleaseAge {
		t.Fatalf("managed npm still reads workspace min-release-age: %q", got)
	}
	isolatedArgs[len(isolatedArgs)-1] = "registry"
	if got := readConfig(isolatedArgs...); got == "https://registry.invalid/" {
		t.Fatalf("managed npm still reads workspace registry: %q", got)
	}
	if got := args[prefixIndex+2]; got != "opencode-ai@1.18.5" {
		t.Fatalf("managed package spec shifted or changed: %q", got)
	}
}

func envWithHome(env []string, home string) []string {
	filtered := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, "HOME=") {
			filtered = append(filtered, item)
		}
	}
	return append(filtered, "HOME="+home)
}
