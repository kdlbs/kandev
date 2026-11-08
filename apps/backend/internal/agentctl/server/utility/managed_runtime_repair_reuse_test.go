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
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
	"go.uber.org/zap"
)

func TestManagedRuntimeRepairReuseProbe(t *testing.T) {
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
	repairContext, ok := state.probeRepairContext(req, cfg)
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
	repairContext, ok := state.probeRepairContext(req, cfg)
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
	if err := state.publishProbeRepair(prepared, req, cfg); err != nil {
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
	changedContext, ok := state2.probeRepairContext(&changedReq, changedCfg)
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
	if err := state2.publishProbeRepair(candidate, req, cfg); err == nil {
		t.Fatal("published a probe repair under a changed installer context")
	}
	if candidate.RepairPublished() {
		t.Fatal("changed probe context published a generation")
	}
}
