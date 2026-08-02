package agents

import (
	"slices"
	"strings"
	"testing"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.agent.ManagedNPMRuntime()
			if got := spec.Package; got != tt.wantPackage {
				t.Fatalf("Package = %q, want %q", got, tt.wantPackage)
			}
			if got := spec.ACPArgs; !slices.Equal(got, tt.wantACPArgs) {
				t.Fatalf("ACPArgs = %#v, want %#v", got, tt.wantACPArgs)
			}

			cached := spec.CachedACPCommand()
			wantCached := append([]string{"npx", "--yes", "--prefer-offline", tt.wantPackage}, tt.wantACPArgs...)
			if !slices.Equal(cached.Args(), wantCached) {
				t.Fatalf("CachedACPCommand = %#v, want %#v", cached.Args(), wantCached)
			}
			assertUnversionedPackage(t, cached.Args(), tt.wantPackage)

			update := spec.CacheUpdateCommand()
			wantUpdate := []string{
				"npm", "exec", "--yes", "--prefer-online",
				"--package=" + tt.wantPackage, "--", "node", "-e", "",
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
	if got := spec.ExecutionCacheKey(); got != "e2094862b59aac7b" {
		t.Fatalf("ExecutionCacheKey = %q, want npm key e2094862b59aac7b", got)
	}
}

func assertUnversionedPackage(t *testing.T, argv []string, wantPackage string) {
	t.Helper()
	packageArg := argv[3]
	if packageArg != wantPackage {
		t.Fatalf("package argv = %q, want unversioned %q", packageArg, wantPackage)
	}
	if packageArg == wantPackage+"@latest" {
		t.Fatalf("package argv contains explicit latest: %q", packageArg)
	}
}
