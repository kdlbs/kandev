package utility

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

func TestManagedRuntimeRepairReuseProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair requires native ACL verification on Windows")
	}
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	cfg := repairProbeConfig(t, packageSpec)
	state := newManagedRuntimePreparationStateWithHome(home, nil, zap.NewNop())
	if state.store == nil {
		t.Fatal("repair store was not opened for the probe handler")
	}
	req := &ProbeRequest{
		AgentID: "claude-acp",
		InstallContext: &agentctltypes.ManagedRuntimeInstallContext{
			InstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(cfg.Env),
			OwnerScope:         managedruntime.RepairOwnerScopeAgentWide("owner-a"),
		},
		InferenceConfig: cfg,
	}
	repairContext, ok := state.probeRepairContext(context.Background(), req, cfg)
	if !ok {
		t.Fatal("probe repair context was not built")
	}
	lease, err := state.store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin probe recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), sanitizeEnvForAgent(cfg), repairProbeCacheFixture(packageSpec))
	if err != nil {
		t.Fatalf("prepare probe repair: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish probe repair: %v", err)
	}
	selected, outcome := state.selectProbeRuntime(context.Background(), req, cfg)
	if outcome != managedruntime.RepairOutcomeHit || selected == nil {
		t.Fatalf("probe selection = (%v, %s), want a hit", selected, outcome)
	}
	args, err := selected.RewriteNpxCommand(buildACPCommand(cfg, ""))
	if err != nil {
		t.Fatalf("rewrite probe command: %v", err)
	}
	if !strings.Contains(strings.Join(args, " "), selected.ProjectPrefix()) {
		t.Fatalf("rewritten probe command = %#v, want the selected project", args)
	}
	changed := repairProbeConfig(t, packageSpec)
	changed.Env["NPM_CONFIG_REGISTRY"] = "https://changed.example.test/"
	changedReq := *req
	changedReq.InferenceConfig = changed
	if _, changedOutcome := state.selectProbeRuntime(context.Background(), &changedReq, changed); changedOutcome == managedruntime.RepairOutcomeHit {
		t.Fatal("changed npm configuration reused an incompatible repair")
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.11
// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.12
func TestManagedRuntimeProbeRepairWithCLIArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair requires native ACL verification on Windows")
	}
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	cfg := repairProbeConfig(t, packageSpec)
	cfg.CLIFlags = []string{"--dangerously-skip-permissions"}
	state := newManagedRuntimePreparationStateWithHome(home, nil, zap.NewNop())
	req := &ProbeRequest{
		AgentID: "claude-acp",
		InstallContext: &agentctltypes.ManagedRuntimeInstallContext{
			InstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(cfg.Env),
			OwnerScope:         managedruntime.RepairOwnerScopeAgentWide("owner-a"),
		},
		InferenceConfig: cfg,
	}
	repairContext, ok := state.probeRepairContext(context.Background(), req, cfg)
	if !ok {
		t.Fatal("flagged probe repair context was not built")
	}
	lease, err := state.store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin flagged probe recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), sanitizeEnvForAgent(cfg), repairProbeCacheFixture(packageSpec))
	if err != nil {
		t.Fatalf("prepare flagged probe repair: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish flagged probe repair: %v", err)
	}
	selected, outcome := state.selectProbeRuntime(context.Background(), req, cfg)
	if outcome != managedruntime.RepairOutcomeHit || selected == nil {
		t.Fatalf("flagged probe selection = (%v, %s), want a hit", selected, outcome)
	}
	rewritten, err := selected.RewriteNpxCommand(buildACPCommand(cfg, ""))
	if err != nil {
		t.Fatalf("rewrite flagged probe command: %v", err)
	}
	launch, err := prepareACPLaunch(cfg, "npx", rewritten, sanitizeEnvForAgent(cfg))
	if err != nil {
		t.Fatalf("compose flagged probe command: %v", err)
	}
	if got := strings.Join(launch, "\x00"); strings.Count(got, "--dangerously-skip-permissions") != 1 {
		t.Fatalf("launched flagged command = %#v, want the profile flag exactly once", launch)
	}
	if launch[len(launch)-1] != "--dangerously-skip-permissions" {
		t.Fatalf("launched flagged command tail = %#v, want the original flag", launch)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
func TestManagedRuntimeProbeRetryReusesConcurrentPublication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair requires native ACL verification on Windows")
	}
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	cfg := repairProbeConfig(t, packageSpec)
	cfg.CLIFlags = []string{"--dangerously-skip-permissions"}
	argvCapture := filepath.Join(t.TempDir(), "argv")
	cfg.Env["ARGV_CAPTURE"] = argvCapture
	cfg.Env["PATH"] = cfg.Env["PATH"] + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Setenv("PATH", cfg.Env["PATH"])
	pathParts := strings.Split(cfg.Env["PATH"], string(os.PathListSeparator))
	agentScript := `#!/bin/sh
printf '%s\n' "$@" > "$ARGV_CAPTURE"
while IFS= read -r request; do
  id=$(printf '%s\n' "$request" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
  case "$request" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{}}}\n' "$id"
      ;;
    *'"method":"session/new"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"test","configOptions":[]}}\n' "$id"
      ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(pathParts[0], "npx"), []byte(agentScript), 0o700); err != nil {
		t.Fatal(err)
	}
	state := newManagedRuntimePreparationStateWithHome(home, nil, zap.NewNop())
	req := &ProbeRequest{
		AgentID: "claude-acp",
		InstallContext: &agentctltypes.ManagedRuntimeInstallContext{
			InstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(cfg.Env),
			OwnerScope:         managedruntime.RepairOwnerScopeAgentWide("owner-a"),
		},
		InferenceConfig: cfg,
	}
	evidence := state.rememberProbeFailure(packageSpec, "claude-acp", "acp", cfg)
	if evidence == nil || evidence.ProbeAttemptID == "" {
		t.Fatal("probe failure was not remembered")
	}
	repairContext, ok := state.probeRepairContext(context.Background(), req, cfg)
	if !ok {
		t.Fatal("probe repair context was not built")
	}
	lease, err := state.store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin concurrent probe recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), sanitizeEnvForAgent(cfg), repairProbeCacheFixture(packageSpec))
	if err != nil {
		t.Fatalf("prepare concurrent probe repair: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish concurrent probe repair: %v", err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatalf("close concurrent probe repair: %v", err)
	}
	generationsBefore, err := filepath.Glob(filepath.Join(state.store.RepairRoot(), "generations", "*"))
	if err != nil {
		t.Fatal(err)
	}
	req.Preparation = &agentctltypes.ManagedRuntimePreparationDirective{
		Condition:      agentctltypes.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:    packageSpec,
		ProbeAttemptID: evidence.ProbeAttemptID,
	}
	response, err := NewACPInferenceExecutor(zap.NewNop(), state).Probe(context.Background(), req)
	if err != nil {
		t.Fatalf("probe returned error: %v", err)
	}
	if !response.Success {
		t.Fatalf("probe failed: %s", response.Error)
	}
	generationsAfter, err := filepath.Glob(filepath.Join(state.store.RepairRoot(), "generations", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(generationsAfter) != len(generationsBefore) {
		t.Fatalf("concurrent reuse installed another generation: before=%d after=%d", len(generationsBefore), len(generationsAfter))
	}
	raw, err := os.ReadFile(argvCapture)
	if err != nil {
		t.Fatalf("read launched argv: %v", err)
	}
	launched := strings.Fields(string(raw))
	if strings.Count(strings.Join(launched, "\x00"), "--dangerously-skip-permissions") != 1 {
		t.Fatalf("launched argv = %#v, want the permission flag exactly once", launched)
	}
	if launched[len(launched)-1] != "--dangerously-skip-permissions" {
		t.Fatalf("launched argv tail = %#v, want the profile flag", launched)
	}
}

func repairProbeConfig(t *testing.T, packageSpec string) *InferenceConfigDTO {
	t.Helper()
	bin := t.TempDir()
	for name, version := range map[string]string{"node": "24.0.0", "npm": "11.0.0"} {
		script := "#!/bin/sh\necho " + version + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand(strings.TrimPrefix(packageSpec[strings.LastIndexByte(packageSpec, '@'):], "@")).Args()
	return &InferenceConfigDTO{
		Protocol: agent.ProtocolACP,
		Command:  command,
		WorkDir:  t.TempDir(),
		Env: map[string]string{
			"HOME":                t.TempDir(),
			"PATH":                bin,
			"NPM_CONFIG_CACHE":    filepath.Join(t.TempDir(), "npm-cache"),
			"NPM_CONFIG_REGISTRY": "https://registry.example.test/",
		},
	}
}

func repairProbeCacheFixture(packageSpec string) managedruntime.ManagedRuntimeCommandRunner {
	return func(_ context.Context, command managedruntime.ManagedRuntimeCommand) error {
		cache, ok := runtimeCommandValue(command.Args, "--cache")
		if !ok {
			return os.ErrInvalid
		}
		separator := strings.LastIndexByte(packageSpec, '@')
		name, exactVersion := packageSpec[:separator], packageSpec[separator+1:]
		executionRoot := filepath.Join(cache, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
		packageDir := filepath.Join(executionRoot, "node_modules", filepath.FromSlash(name))
		binName, target := "claude-agent-acp", filepath.Join("bin", "cli.js")
		manifest, marshalErr := json.Marshal(map[string]any{
			"name": name, "version": exactVersion, "bin": map[string]string{binName: filepath.ToSlash(target)},
		})
		if marshalErr != nil {
			return marshalErr
		}
		if err := os.MkdirAll(filepath.Join(packageDir, "bin"), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(executionRoot, "package.json"), []byte(`{}`), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(packageDir, "package.json"), manifest, 0o600); err != nil {
			return err
		}
		executable := filepath.Join(packageDir, target)
		if err := os.WriteFile(executable, []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
			return err
		}
		binDirectory := filepath.Join(executionRoot, "node_modules", ".bin")
		if err := os.MkdirAll(binDirectory, 0o700); err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			relative, err := filepath.Rel(binDirectory, executable)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(binDirectory, binName+".cmd"), []byte(strings.ReplaceAll(relative, "/", "\\")), 0o600)
		}
		relative, err := filepath.Rel(binDirectory, executable)
		if err != nil {
			return err
		}
		return os.Symlink(relative, filepath.Join(binDirectory, binName))
	}
}

func runtimeCommandValue(args []string, key string) (string, bool) {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == key {
			return args[index+1], true
		}
	}
	return "", false
}

func TestPublishProbeRepairRevalidatesContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair requires native ACL verification on Windows")
	}
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	cfg := repairProbeConfig(t, packageSpec)
	state := newManagedRuntimePreparationStateWithHome(home, nil, zap.NewNop())
	req := &ProbeRequest{
		AgentID: "claude-acp",
		InstallContext: &agentctltypes.ManagedRuntimeInstallContext{
			InstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(cfg.Env),
			OwnerScope:         managedruntime.RepairOwnerScopeAgentWide("owner-a"),
		},
		InferenceConfig: cfg,
	}
	repairContext, ok := state.probeRepairContext(context.Background(), req, cfg)
	if !ok {
		t.Fatal("probe repair context was not built")
	}
	lease, err := state.store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin probe recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), sanitizeEnvForAgent(cfg), repairProbeCacheFixture(packageSpec))
	if err != nil {
		t.Fatalf("prepare probe repair: %v", err)
	}
	if err := state.publishProbeRepair(context.Background(), prepared, req, cfg); err != nil {
		t.Fatalf("publish unchanged probe repair: %v", err)
	}
	if _, outcome, _ := state.store.Lookup(context.Background(), repairContext); outcome != managedruntime.RepairOutcomeHit {
		t.Fatalf("published probe repair outcome = %s, want a hit", outcome)
	}

	changedCfg := repairProbeConfig(t, packageSpec)
	changedCfg.Env["NPM_CONFIG_REGISTRY"] = "https://changed.example.test/"
	changedReq := *req
	changedReq.InferenceConfig = changedCfg
	state2 := newManagedRuntimePreparationStateWithHome(t.TempDir(), nil, zap.NewNop())
	changedContext, ok := state2.probeRepairContext(context.Background(), &changedReq, changedCfg)
	if !ok {
		t.Fatal("changed probe context was not built")
	}
	lease2, err := state2.store.BeginRecovery(context.Background(), changedContext)
	if err != nil {
		t.Fatalf("begin changed probe recovery: %v", err)
	}
	candidate, err := lease2.Prepare(context.Background(), sanitizeEnvForAgent(changedCfg), repairProbeCacheFixture(packageSpec))
	if err != nil {
		t.Fatalf("prepare changed probe repair: %v", err)
	}
	if err := state2.publishProbeRepair(context.Background(), candidate, req, cfg); err == nil {
		t.Fatal("published a probe repair under a changed installer context")
	}
	if candidate.RepairPublished() {
		t.Fatal("changed probe context published a generation")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	freshLease, err := state.store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin cancelled probe recovery: %v", err)
	}
	cancelledCandidate, err := freshLease.Prepare(context.Background(), sanitizeEnvForAgent(cfg), repairProbeCacheFixture(packageSpec))
	if err != nil {
		t.Fatalf("prepare cancelled probe repair: %v", err)
	}
	if err := state.publishProbeRepair(cancelled, cancelledCandidate, req, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publication error = %v, want context cancellation", err)
	}
	if cancelledCandidate.RepairPublished() {
		t.Fatal("cancelled probe context published a generation")
	}
}
