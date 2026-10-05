package utility

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"go.uber.org/zap"
)

func TestParseCodexVersionAcceptsOnlyTheVersionLine(t *testing.T) {
	for _, lineEnding := range []string{"\n", "\r\n"} {
		output := []byte("codex-cli 0.177.3" + lineEnding + "OPENAI_API_KEY=do-not-return" + lineEnding)
		if got := parseCodexVersion(output); got != "0.177.3" {
			t.Fatalf("version from %q line endings = %q, want parsed version only", lineEnding, got)
		}
	}
	for _, output := range [][]byte{
		[]byte("Codex CLI 0.177.3\n"),
		[]byte("codex-cli latest\n"),
		[]byte("codex-cli 0.177.3 account=private\n"),
	} {
		if got := parseCodexVersion(output); got != "" {
			t.Fatalf("parseCodexVersion(%q) = %q, want unknown", output, got)
		}
	}
}

func TestRuntimeComponentKeepsOnlyHTTPSGuidance(t *testing.T) {
	descriptor := agents.RuntimeComponentDescriptor{
		Name:        "Codex CLI",
		Source:      agents.RuntimeComponentExternal,
		Owner:       agents.RuntimeComponentOwnerExternal,
		GuidanceURL: "https://github.com/openai/codex",
	}
	if got := componentFromDescriptor(agents.RuntimeComponentProvider, descriptor).GuidanceURL; got != descriptor.GuidanceURL {
		t.Fatalf("guidance URL = %q, want %q", got, descriptor.GuidanceURL)
	}
	descriptor.GuidanceURL = "javascript:alert(1)"
	if got := componentFromDescriptor(agents.RuntimeComponentProvider, descriptor).GuidanceURL; got != "" {
		t.Fatalf("unsafe guidance URL = %q, want empty", got)
	}
}

func TestRuntimeObservationPreservesExternalPrimaryRuntimeOwnership(t *testing.T) {
	descriptor := &agents.RuntimeObservationDescriptor{
		Bridge: agents.RuntimeComponentDescriptor{
			Name: "OpenCode", Source: agents.RuntimeComponentExternal,
			Owner: agents.RuntimeComponentOwnerExternal, GuidanceURL: "https://opencode.ai/docs/cli/",
		},
	}
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), descriptor, []string{"opencode", "acp"}, nil,
		[]string{"opencode", "acp"}, nil, "", t.TempDir(), "1.2.3",
	)
	bridge := runtimeComponentByRole(t, info, agents.RuntimeComponentBridge)
	if bridge.Source != agents.RuntimeComponentExternal || bridge.Owner != agents.RuntimeComponentOwnerExternal ||
		bridge.Package != "" || bridge.GuidanceURL != "https://opencode.ai/docs/cli/" || bridge.ObservedVersion != "1.2.3" {
		t.Fatalf("native runtime observation = %#v", bridge)
	}
}

func TestInspectBundledDependencyVersionUsesExactNpxTree(t *testing.T) {
	for _, layout := range []string{"nested", "hoisted"} {
		t.Run(layout, func(t *testing.T) {
			root, bridgeManifest, dependencyManifest := runtimePackageTree(t, layout)
			got := inspectBundledDependencyVersion(
				root,
				"@agentclientprotocol/codex-acp@1.2.3",
				"@agentclientprotocol/codex-acp",
				"@openai/codex",
			)
			if got != "0.177.3" {
				t.Fatalf("bundled version = %q, want 0.177.3", got)
			}
			if _, err := os.Stat(bridgeManifest); err != nil {
				t.Fatalf("bridge manifest should remain untouched: %v", err)
			}
			if _, err := os.Stat(dependencyManifest); err != nil {
				t.Fatalf("dependency manifest should remain untouched: %v", err)
			}
		})
	}
}

func TestInspectBundledDependencyVersionRejectsUnverifiedTrees(t *testing.T) {
	t.Run("wrong bridge identity", func(t *testing.T) {
		root, _, _ := runtimePackageTree(t, "hoisted")
		bridgeManifest := filepath.Join(
			npxTreeRoot(root, "@agentclientprotocol/codex-acp@1.2.3"),
			"node_modules", "@agentclientprotocol", "codex-acp", "package.json",
		)
		writePackageManifest(t, bridgeManifest, "@agentclientprotocol/codex-acp", "9.9.9")
		if got := inspectBundledDependencyVersion(root, "@agentclientprotocol/codex-acp@1.2.3", "@agentclientprotocol/codex-acp", "@openai/codex"); got != "" {
			t.Fatalf("wrong bridge identity produced version %q", got)
		}
	})
	t.Run("invalid dependency version", func(t *testing.T) {
		root, _, dependencyManifest := runtimePackageTree(t, "hoisted")
		writePackageManifest(t, dependencyManifest, "@openai/codex", "latest")
		if got := inspectBundledDependencyVersion(root, "@agentclientprotocol/codex-acp@1.2.3", "@agentclientprotocol/codex-acp", "@openai/codex"); got != "" {
			t.Fatalf("invalid dependency version produced %q", got)
		}
	})
	t.Run("wrong dependency package", func(t *testing.T) {
		root, _, dependencyManifest := runtimePackageTree(t, "hoisted")
		writePackageManifest(t, dependencyManifest, "@openai/unrelated", "0.177.3")
		if got := inspectBundledDependencyVersion(root, "@agentclientprotocol/codex-acp@1.2.3", "@agentclientprotocol/codex-acp", "@openai/codex"); got != "" {
			t.Fatalf("wrong dependency package produced version %q", got)
		}
	})
	if validRuntimePackage("@openai/..") {
		t.Fatal("package traversal was accepted")
	}
	if got := inspectBundledDependencyVersion(t.TempDir(), "@agentclientprotocol/codex-acp@1.2.3", "@agentclientprotocol/codex-acp", "@openai/codex"); got != "" {
		t.Fatalf("missing tree produced version %q", got)
	}
}

func TestRuntimeManifestSnapshotDetectsReplacement(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "package.json")
	writePackageManifest(t, manifestPath, "@openai/codex", "0.177.3")
	_, snapshot, ok := readRuntimeManifest(root, manifestPath)
	if !ok {
		t.Fatal("manifest snapshot was not read")
	}
	if !runtimeManifestUnchanged(snapshot) {
		t.Fatal("unchanged manifest was rejected")
	}
	writePackageManifest(t, manifestPath, "@openai/codex", "0.177.4")
	if runtimeManifestUnchanged(snapshot) {
		t.Fatal("replaced manifest was accepted")
	}
}

func TestRuntimeObservationMarksExternalProviderUnknownUnderPrefix(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	marker := filepath.Join(t.TempDir(), "executed")
	binary := filepath.Join(t.TempDir(), "codex executable")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf x > \""+marker+"\"\nprintf 'codex-cli 0.177.3\\n'\n")
	descriptor := codexRuntimeDescriptor()
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), descriptor, codexManagedCommand(), []string{"sandbox", "--"},
		codexManagedCommand(), []string{"CODEX_PATH=" + binary}, "", t.TempDir(), "1.2.3",
	)
	provider := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
	if provider.Source != agents.RuntimeComponentUnknown || provider.Owner != agents.RuntimeComponentOwnerUnknown || provider.ObservedVersion != "" {
		t.Fatalf("wrapped provider was attributed: %#v", provider)
	}
	bridge := runtimeComponentByRole(t, info, agents.RuntimeComponentBridge)
	if bridge.Source != agents.RuntimeComponentUnknown || bridge.Owner != agents.RuntimeComponentOwnerKandev ||
		bridge.Package != "@agentclientprotocol/codex-acp" || bridge.ObservedVersion != "" || bridge.GuidanceURL != "" {
		t.Fatalf("wrapped primary runtime did not retain only trusted configured identity: %#v", bridge)
	}
	if provider.Package != "" || provider.GuidanceURL != "" {
		t.Fatalf("wrapped provider retained trusted identity: %#v", provider)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external provider was executed despite command prefix: err=%v", err)
	}
}

func TestRuntimeObservationUsesCapturedExternalCodexAndSanitizesOutput(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	dir := filepath.Join(t.TempDir(), "provider binaries with spaces")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create provider bin: %v", err)
	}
	binary := filepath.Join(dir, "codex")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf 'codex-cli 0.177.3\\nOPENAI_API_KEY=private-secret\\n'\nprintf 'ANTHROPIC_API_KEY=private-secret\\n' >&2\n")
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), codexRuntimeDescriptor(), codexManagedCommand(), nil,
		codexManagedCommand(), []string{"CODEX_PATH=codex", "PATH=" + dir}, "", t.TempDir(), "1.2.3",
	)
	if info == nil || info.Scope != "host" {
		t.Fatalf("runtime info = %#v", info)
	}
	bridge := runtimeComponentByRole(t, info, agents.RuntimeComponentBridge)
	provider := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
	if bridge.EffectiveVersion != "" || bridge.ObservedVersion != "1.2.3" {
		t.Fatalf("bridge observation = %#v", bridge)
	}
	if provider.Source != agents.RuntimeComponentExternal || provider.Owner != agents.RuntimeComponentOwnerExternal || provider.ObservedVersion != "0.177.3" {
		t.Fatalf("external provider observation = %#v", provider)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	if strings.Contains(string(data), "private-secret") || strings.Contains(string(data), binary) {
		t.Fatalf("observation exposed command output or path: %s", data)
	}
}

func TestRuntimeObservationResolvesRelativeExecutableInputsAgainstCapturedWorkDir(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses Unix executable fixtures")
	}

	processRelativeDir, err := os.MkdirTemp(".", "runtime-observation-path-")
	if err != nil {
		t.Fatalf("create agentctl-cwd fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(processRelativeDir) })
	processRelativeDir, err = filepath.Rel(".", processRelativeDir)
	if err != nil {
		t.Fatalf("make process fixture relative: %v", err)
	}
	workDir := t.TempDir()
	processBinary := filepath.Join(processRelativeDir, "bin", "codex")
	workBinary := filepath.Join(workDir, processRelativeDir, "bin", "codex")
	if err := os.MkdirAll(filepath.Dir(processBinary), 0o700); err != nil {
		t.Fatalf("create agentctl-cwd bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(workBinary), 0o700); err != nil {
		t.Fatalf("create workDir bin: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatalf("create workDir: %v", err)
	}
	writeRuntimeObservationExecutable(t, processBinary, "#!/bin/sh\nprintf 'codex-cli 9.9.9\\n'\n")
	writeRuntimeObservationExecutable(t, workBinary, "#!/bin/sh\nprintf 'codex-cli 0.177.3\\n'\n")
	writeRuntimeObservationExecutable(t, filepath.Join(workDir, "codex"), "#!/bin/sh\nprintf 'codex-cli 0.177.3\\n'\n")

	tests := []struct {
		name  string
		codex string
		path  string
	}{
		{
			name:  "relative PATH entries and an empty entry",
			codex: "codex",
			path:  "./" + filepath.ToSlash(processRelativeDir) + "/bin::/usr/bin",
		},
		{
			name:  "relative executable path",
			codex: "./" + filepath.ToSlash(processRelativeDir) + "/bin/codex",
			path:  "/usr/bin",
		},
		{
			name:  "empty PATH entry uses workDir",
			codex: "codex",
			path:  "::/usr/bin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
				context.Background(), codexRuntimeDescriptor(), codexManagedCommand(), nil,
				codexManagedCommand(), []string{"CODEX_PATH=" + tt.codex, "PATH=" + tt.path}, "", workDir, "1.2.3",
			)
			provider := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
			if provider.Source != agents.RuntimeComponentExternal || provider.ObservedVersion != "0.177.3" {
				t.Fatalf("provider observation = %#v, want workDir executable version 0.177.3", provider)
			}
		})
	}
}

func TestRuntimeObservationRefreshReadsReplacementAtSameExternalPath(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	binary := filepath.Join(t.TempDir(), "codex")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf 'codex-cli 0.177.3\\n'\n")
	observe := func() string {
		info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
			context.Background(), codexRuntimeDescriptor(), codexManagedCommand(), nil,
			codexManagedCommand(), []string{"CODEX_PATH=" + binary}, "", t.TempDir(), "1.2.3",
		)
		return runtimeComponentByRole(t, info, agents.RuntimeComponentProvider).ObservedVersion
	}
	if got := observe(); got != "0.177.3" {
		t.Fatalf("first observation = %q", got)
	}
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf 'codex-cli 0.177.4\\n'\n")
	if got := observe(); got != "0.177.4" {
		t.Fatalf("refresh at same path = %q, want 0.177.4", got)
	}
}

func TestRuntimeObservationUsesInheritedCodexPathWhenProfileHasNoOverride(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	binary := filepath.Join(t.TempDir(), "inherited codex")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf 'codex-cli 0.177.3\\n'\n")
	t.Setenv("CODEX_PATH", binary)
	childEnv := sanitizeEnvForAgent(&InferenceConfigDTO{Env: map[string]string{}})
	if got := environmentValue(childEnv, "CODEX_PATH"); got != binary {
		t.Fatalf("inherited CODEX_PATH = %q, want %q", got, binary)
	}
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), codexRuntimeDescriptor(), codexManagedCommand(), nil,
		codexManagedCommand(), childEnv, "", t.TempDir(), "1.2.3",
	)
	provider := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
	if provider.Source != agents.RuntimeComponentExternal || provider.ObservedVersion != "0.177.3" {
		t.Fatalf("inherited provider observation = %#v", provider)
	}
}

func TestRuntimeObservationUsesBundledClaudeSDKManifest(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses Unix process fixtures")
	}
	cacheRoot := t.TempDir()
	bridgeSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	npxRoot := npxTreeRoot(cacheRoot, bridgeSpec)
	writePackageManifest(t,
		filepath.Join(npxRoot, "node_modules", "@agentclientprotocol", "claude-agent-acp", "package.json"),
		"@agentclientprotocol/claude-agent-acp", "1.2.3",
	)
	writePackageManifest(t,
		filepath.Join(npxRoot, "node_modules", "@anthropic-ai", "claude-agent-sdk", "package.json"),
		"@anthropic-ai/claude-agent-sdk", "0.16.1",
	)
	decoyCacheRoot := t.TempDir()
	decoyNpxRoot := npxTreeRoot(decoyCacheRoot, bridgeSpec)
	writePackageManifest(t,
		filepath.Join(decoyNpxRoot, "node_modules", "@agentclientprotocol", "claude-agent-acp", "package.json"),
		"@agentclientprotocol/claude-agent-acp", "1.2.3",
	)
	writePackageManifest(t,
		filepath.Join(decoyNpxRoot, "node_modules", "@anthropic-ai", "claude-agent-sdk", "package.json"),
		"@anthropic-ai/claude-agent-sdk", "9.9.9",
	)
	npxBinDir := filepath.Join(t.TempDir(), "launched npx")
	if err := os.MkdirAll(npxBinDir, 0o700); err != nil {
		t.Fatalf("create launched npx bin: %v", err)
	}
	childPathDir := filepath.Join(t.TempDir(), "child PATH")
	if err := os.MkdirAll(childPathDir, 0o700); err != nil {
		t.Fatalf("create child PATH bin: %v", err)
	}
	argsFile := filepath.Join(t.TempDir(), "npm args")
	npxExecutable := filepath.Join(npxBinDir, "npx")
	writeRuntimeObservationExecutable(t, npxExecutable, "#!/bin/sh\nexit 0\n")
	writeRuntimeObservationExecutable(t, filepath.Join(npxBinDir, "npm"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NPM_ARGS_FILE\"\nprintf 'npm warn using a legacy cache setting\\n%s\\n' \"$NPM_CACHE_ROOT\"\n")
	decoyNPMMarker := filepath.Join(t.TempDir(), "decoy npm used")
	writeRuntimeObservationExecutable(t, filepath.Join(childPathDir, "npm"), "#!/bin/sh\nprintf x > \""+decoyNPMMarker+"\"\nprintf '%s\\n' \"$DECOY_NPM_CACHE_ROOT\"\n")
	globalClaudeMarker := filepath.Join(t.TempDir(), "global-claude-used")
	writeRuntimeObservationExecutable(t, filepath.Join(childPathDir, "claude"), "#!/bin/sh\nprintf x > \""+globalClaudeMarker+"\"\nprintf 'Claude Code 99.9.9\\n'\n")
	preparedPrefix := filepath.Join(t.TempDir(), "managed npm prefix")
	preparedCommand := []string{"npx", "--yes", "--prefer-offline", "--prefix", preparedPrefix, bridgeSpec}
	command := []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, bridgeSpec}
	provider := agents.RuntimeComponentDescriptor{
		Name: "Claude Agent SDK", Package: "@anthropic-ai/claude-agent-sdk",
		Source: agents.RuntimeComponentBundled, Owner: agents.RuntimeComponentOwnerKandev,
	}
	descriptor := &agents.RuntimeObservationDescriptor{
		Bridge: agents.RuntimeComponentDescriptor{
			Name: "Claude", Package: "@agentclientprotocol/claude-agent-acp",
			Source: agents.RuntimeComponentManaged, Owner: agents.RuntimeComponentOwnerKandev,
		},
		Provider: &provider,
	}
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), descriptor, command, nil, preparedCommand,
		[]string{"PATH=" + childPathDir, "NPM_CACHE_ROOT=" + cacheRoot,
			"DECOY_NPM_CACHE_ROOT=" + decoyCacheRoot, "NPM_ARGS_FILE=" + argsFile},
		npxExecutable,
		t.TempDir(), "1.2.3",
	)
	observed := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
	if observed.Source != agents.RuntimeComponentBundled || observed.ObservedVersion != "0.16.1" {
		t.Fatalf("SDK observation = %#v", observed)
	}
	if _, err := os.Stat(globalClaudeMarker); !os.IsNotExist(err) {
		t.Fatalf("global Claude CLI was inspected: %v", err)
	}
	if _, err := os.Stat(decoyNPMMarker); !os.IsNotExist(err) {
		t.Fatalf("npm from child PATH was inspected instead of the launched npx sibling: %v", err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read npm args: %v", err)
	}
	want := "--prefix\n" + preparedPrefix + "\nconfig\nget\ncache\n"
	if string(args) != want {
		t.Fatalf("npm args = %q, want %q", args, want)
	}
}

func TestRuntimeObservationKeepsExternalIdentityAfterVersionFailure(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	binary := filepath.Join(t.TempDir(), "broken codex")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf 'token=secret\\n' >&2\nexit 2\n")
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), codexRuntimeDescriptor(), codexManagedCommand(), nil,
		codexManagedCommand(), []string{"CODEX_PATH=" + binary}, "", t.TempDir(), "1.2.3",
	)
	provider := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
	if provider.Source != agents.RuntimeComponentExternal || provider.ObservedVersion != "" {
		t.Fatalf("failed external inspection lost its source or invented a version: %#v", provider)
	}
}

func TestRuntimeObservationRejectsOversizedVersionOutput(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	binary := filepath.Join(t.TempDir(), "large codex")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nprintf 'codex-cli 0.177.3\\n'; head -c 8192 /dev/zero | tr '\\000' x; printf 'credential=secret\\n' >&2\n")
	info := NewACPInferenceExecutor(zap.NewNop()).collectRuntimeObservation(
		context.Background(), codexRuntimeDescriptor(), codexManagedCommand(), nil,
		codexManagedCommand(), []string{"CODEX_PATH=" + binary}, "", t.TempDir(), "1.2.3",
	)
	provider := runtimeComponentByRole(t, info, agents.RuntimeComponentProvider)
	if provider.Source != agents.RuntimeComponentExternal || provider.ObservedVersion != "" {
		t.Fatalf("oversized output produced unsafe or false evidence: %#v", provider)
	}
	data, _ := json.Marshal(info)
	if strings.Contains(string(data), "credential") || strings.Contains(string(data), "secret") {
		t.Fatalf("oversized command output escaped: %s", data)
	}
}

func TestRuntimeObservationCommandRejectsOversizedOutput(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("uses a Unix executable fixture")
	}
	binary := filepath.Join(t.TempDir(), "large output")
	writeRuntimeObservationExecutable(t, binary, "#!/bin/sh\nhead -c 8192 /dev/zero | tr '\\000' x\n")
	output, err := runRuntimeObservationCommand(context.Background(), binary, nil, os.Environ(), t.TempDir(), zap.NewNop())
	if err == nil {
		t.Fatalf("oversized output was accepted with %d captured bytes", len(output))
	}
}

func TestBoundedObservationBufferCapsWrites(t *testing.T) {
	var output boundedObservationBuffer
	if n, err := output.Write(make([]byte, runtimeCommandMaxBytes*2)); err != nil || n != runtimeCommandMaxBytes*2 {
		t.Fatalf("Write() = %d, %v", n, err)
	}
	if output.Len() != runtimeCommandMaxBytes || !output.overflow {
		t.Fatalf("buffer length/overflow = %d/%t", output.Len(), output.overflow)
	}
}

func codexRuntimeDescriptor() *agents.RuntimeObservationDescriptor {
	provider := agents.RuntimeComponentDescriptor{
		Name: "Codex CLI", Package: "@openai/codex", Source: agents.RuntimeComponentBundled,
		Owner: agents.RuntimeComponentOwnerKandev, ExternalVersionEnv: "CODEX_PATH",
	}
	return &agents.RuntimeObservationDescriptor{
		Bridge: agents.RuntimeComponentDescriptor{
			Name: "Codex", Package: "@agentclientprotocol/codex-acp",
			Source: agents.RuntimeComponentManaged, Owner: agents.RuntimeComponentOwnerKandev,
		},
		Provider: &provider,
	}
}

func codexManagedCommand() []string {
	return []string{
		"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix,
		"@agentclientprotocol/codex-acp@1.2.3",
	}
}

func runtimeComponentByRole(t *testing.T, info *agents.RuntimeInfo, role agents.RuntimeComponentRole) agents.RuntimeComponent {
	t.Helper()
	if info == nil {
		t.Fatal("runtime observation is nil")
	}
	for _, component := range info.Components {
		if component.Role == role {
			return component
		}
	}
	t.Fatalf("runtime observation has no %q component: %#v", role, info.Components)
	return agents.RuntimeComponent{}
}

func runtimePackageTree(t *testing.T, layout string) (string, string, string) {
	t.Helper()
	cacheRoot := t.TempDir()
	bridgeSpec := "@agentclientprotocol/codex-acp@1.2.3"
	npxRoot := npxTreeRoot(cacheRoot, bridgeSpec)
	bridgeManifest := filepath.Join(npxRoot, "node_modules", "@agentclientprotocol", "codex-acp", "package.json")
	writePackageManifest(t, bridgeManifest, "@agentclientprotocol/codex-acp", "1.2.3")
	dependencyManifest := filepath.Join(npxRoot, "node_modules", "@openai", "codex", "package.json")
	if layout == "nested" {
		dependencyManifest = filepath.Join(npxRoot, "node_modules", "@agentclientprotocol", "codex-acp", "node_modules", "@openai", "codex", "package.json")
	}
	writePackageManifest(t, dependencyManifest, "@openai/codex", "0.177.3")
	return cacheRoot, bridgeManifest, dependencyManifest
}

func npxTreeRoot(cacheRoot, packageSpec string) string {
	return filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
}

func writePackageManifest(t *testing.T, path, name, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create package tree: %v", err)
	}
	data, err := json.Marshal(packageManifest{Name: name, Version: version})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func writeRuntimeObservationExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}
