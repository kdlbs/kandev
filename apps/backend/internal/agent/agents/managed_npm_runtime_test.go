package agents

import (
	"os"
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
			wantCached := append([]string{"npx", "--yes", "--prefer-offline", tt.wantPackage + "@" + wantDefault}, tt.wantACPArgs...)
			if !slices.Equal(cached.Args(), wantCached) {
				t.Fatalf("CachedACPCommand = %#v, want %#v", cached.Args(), wantCached)
			}
			if got := spec.PackageSpec(""); got != tt.wantPackage+"@"+wantDefault {
				t.Fatalf("empty PackageSpec = %q, want exact default", got)
			}

			update := spec.CacheUpdateCommand()
			wantUpdate := []string{
				"npm", "exec", "--yes", "--prefer-online",
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
	wantACP := []string{"npx", "--yes", "--prefer-offline", "opencode-ai@1.18.5", "acp", "--print-logs"}
	if got := spec.ACPCommand("1.18.5").Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("ACPCommand = %#v, want %#v", got, wantACP)
	}
	wantUpdate := []string{
		"npm", "exec", "--yes", "--prefer-online",
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
	want := []string{"npx", "--yes", "--prefer-online", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}
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
	want := []string{"npx", "--yes", "--prefer-offline", "@scope/managed-acp@3.4.5", "--acp"}
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
	wantNpx := []string{"npx", "--yes", "--prefer-offline", "opencode-ai@1.2.3", "acp", "--print-logs", "--log-level", "ERROR"}
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
	if got := plain.RefreshCommand().Args(); !slices.Equal(got, []string{"npx", "--yes", "--prefer-offline", "@scope/custom", "acp"}) {
		t.Fatalf("RefreshCommand (no native) = %#v, want npx", got)
	}
	if got := ab.ExecutionCacheKey(); got != managedruntime.NpxExecutionCacheKey("opencode-ai@1.2.3") {
		t.Fatalf("ExecutionCacheKey with NativeBinary = %q, want package key", got)
	}
}
