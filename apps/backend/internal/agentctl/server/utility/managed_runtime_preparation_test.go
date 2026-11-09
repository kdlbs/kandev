package utility

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

func TestManagedRuntimePreparationAttemptBindsExactUtilityProbeAndCancellation(t *testing.T) {
	const (
		packageVersion = "1.2.3"
		secret         = "private-env-secret"
	)
	packageSpec := "@agentclientprotocol/claude-agent-acp@" + packageVersion
	runtimeSpec, ok := agents.BuiltinManagedNPMRuntime("claude-acp")
	if !ok {
		t.Fatal("Claude ACP should be a registered managed npm runtime")
	}
	cacheRoot := t.TempDir()
	executionRoot := npxTreeRoot(cacheRoot, packageSpec)
	writePrivateRuntimePackageManifest(t,
		filepath.Join(executionRoot, "node_modules", "@agentclientprotocol", "claude-agent-acp", "package.json"),
		"@agentclientprotocol/claude-agent-acp", packageVersion,
	)
	workDir := t.TempDir()
	cfg := &InferenceConfigDTO{
		Command: runtimeSpec.RuntimeCommand(packageVersion).Args(),
		WorkDir: workDir,
		Env: map[string]string{
			"NPM_CONFIG_CACHE":   cacheRoot,
			"PRIVATE_TEST_VALUE": secret,
		},
	}
	launchArgs := append([]string(nil), cfg.Command...)
	packageIndex := indexOfArgument(launchArgs, packageSpec)
	if packageIndex < 0 {
		t.Fatal("built-in runtime command is missing the exact package spec")
	}
	launchArgs = append(launchArgs[:packageIndex], append([]string{"--cache", cacheRoot}, launchArgs[packageIndex:]...)...)
	diagnostic := "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "package.json") + "\n"
	var stderr stderrBuffer
	if _, err := stderr.Write([]byte(diagnostic)); err != nil {
		t.Fatalf("write raw npm diagnostic: %v", err)
	}

	state := newManagedRuntimePreparationState()
	evidence := state.managedRuntimeCacheEvidenceForProbe(
		context.Background(), "claude-acp", "acp", cfg, launchArgs,
		[]string{"NPM_CONFIG_CACHE=" + cacheRoot}, "", workDir, &stderr, zap.NewNop(),
	)
	if evidence == nil {
		t.Fatal("exact installed package and matching root-manifest ENOENT should produce utility evidence")
	}
	if evidence.PackageSpec != packageSpec || evidence.Condition != agentctltypes.ManagedRuntimeCacheRootManifestMissing {
		t.Fatalf("utility cache evidence = %#v", evidence)
	}
	if evidence.ProbeAttemptID == "" || strings.Contains(evidence.ProbeAttemptID, secret) {
		t.Fatalf("probe attempt ID is empty or exposes request content: %q", evidence.ProbeAttemptID)
	}
	var directive agentctltypes.ManagedRuntimePreparationDirective
	directive.Condition = evidence.Condition
	directive.PackageSpec = evidence.PackageSpec
	directive.ProbeAttemptID = evidence.ProbeAttemptID
	if !state.validatesDirective(&directive, "claude-acp", "acp", cfg) {
		t.Fatal("directive matching the exact failed utility probe should validate")
	}
	changed := *cfg
	changed.Env = map[string]string{"NPM_CONFIG_CACHE": cacheRoot, "PRIVATE_TEST_VALUE": "changed"}
	if state.validatesDirective(&directive, "claude-acp", "acp", &changed) {
		t.Fatal("directive should not authorize a probe after its exact environment changed")
	}
	state.retireProbeAttempt(evidence.ProbeAttemptID)
	if state.validatesDirective(&directive, "claude-acp", "acp", cfg) {
		t.Fatal("successful probe should retire its preparation attempt")
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if evidence := state.managedRuntimeCacheEvidenceForProbe(
		canceledCtx, "claude-acp", "acp", cfg, launchArgs,
		[]string{"NPM_CONFIG_CACHE=" + cacheRoot}, "", workDir, &stderr, zap.NewNop(),
	); evidence != nil {
		data, _ := json.Marshal(evidence)
		if strings.Contains(string(data), secret) {
			t.Fatalf("canceled probe exposed a private value: %s", data)
		}
		t.Fatalf("canceled utility probe produced preparation evidence: %#v", evidence)
	}
}

func TestManagedRuntimePreparationRejectsRootManifestPresent(t *testing.T) {
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	cacheRoot := t.TempDir()
	executionRoot := npxTreeRoot(cacheRoot, packageSpec)
	writePrivateRuntimePackageManifest(t,
		filepath.Join(executionRoot, "node_modules", "@agentclientprotocol", "claude-agent-acp", "package.json"),
		"@agentclientprotocol/claude-agent-acp", "1.2.3",
	)
	if err := os.WriteFile(filepath.Join(executionRoot, "package.json"), []byte(`{"private":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &InferenceConfigDTO{
		Command: agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args(),
		WorkDir: t.TempDir(),
	}
	diagnostic := "npm error code ENOENT\nnpm error syscall open\nnpm error path " + filepath.Join(executionRoot, "package.json") + "\n"
	var stderr stderrBuffer
	_, _ = stderr.Write([]byte(diagnostic))
	args := append([]string(nil), cfg.Command...)
	packageIndex := indexOfArgument(args, packageSpec)
	if packageIndex < 0 {
		t.Fatal("built-in runtime command is missing the exact package spec")
	}
	args = append(args[:packageIndex], append([]string{"--cache", cacheRoot}, args[packageIndex:]...)...)
	if evidence := newManagedRuntimePreparationState().managedRuntimeCacheEvidenceForProbe(
		context.Background(), "claude-acp", "acp", cfg, args,
		[]string{"NPM_CONFIG_CACHE=" + cacheRoot}, "", cfg.WorkDir, &stderr, zap.NewNop(),
	); evidence != nil {
		t.Fatalf("present execution root manifest produced evidence: %#v", evidence)
	}
}

func writePrivateRuntimePackageManifest(t *testing.T, path, packageName, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	contents, err := json.Marshal(map[string]string{"name": packageName, "version": version, "bin": "bin/cli.js"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func indexOfArgument(args []string, want string) int {
	for index, arg := range args {
		if arg == want {
			return index
		}
	}
	return -1
}
