package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
)

func TestManagedRuntimeRepairReuseSelection(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	published := publishManagedRuntimeRepair(t, home, packageSpec, env, digest)

	manager := newRepairReuseManager(t, home, command, env, digest)
	selected, outcome := manager.selectManagedRuntimeRepair(context.Background())
	if outcome != managedruntime.RepairOutcomeHit || selected == nil {
		t.Fatalf("selection = (%v, %s), want a hit", selected, outcome)
	}
	if selected.ProjectPrefix() != published.ProjectPrefix() || selected.CacheDirectory() != published.CacheDirectory() {
		t.Fatalf("selected paths = %q/%q, want published %q/%q", selected.ProjectPrefix(), selected.CacheDirectory(), published.ProjectPrefix(), published.CacheDirectory())
	}
	if _, err := selected.RewriteNpxCommand(command); err != nil {
		t.Fatalf("rewrite selected command: %v", err)
	}
	if err := manager.installPreparedRuntime(selected); err != nil {
		t.Fatalf("install selected repair: %v", err)
	}
	rewritten, err := manager.prepareManagedRuntimeCommand(append([]string(nil), command...))
	if err != nil {
		t.Fatalf("rewrite selected command: %v", err)
	}
	if prefix, _ := runtimeCommandArgument(rewritten, "--prefix"); prefix != selected.ProjectPrefix() {
		t.Fatalf("rewritten prefix = %q, want %q", prefix, selected.ProjectPrefix())
	}
	if cache, _ := runtimeCommandArgument(rewritten, "--cache"); cache != selected.CacheDirectory() {
		t.Fatalf("rewritten command = %#v, want the selected generation paths", rewritten)
	}
	if err := manager.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("teardown selected manager: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(selected.ProjectPrefix())); err != nil {
		t.Fatalf("published generation after teardown = %v, want present", err)
	}
}

func TestManagedRuntimeRepairRejectsInvalidBudget(t *testing.T) {
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	if _, err := manager.StartWithManagedRuntimePreparation(context.Background(), types.ManagedRuntimePreparationDirective{
		Condition:               types.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:             "@agentclientprotocol/claude-agent-acp@1.2.3",
		FailedProcessGeneration: 1,
		RemainingBudgetSeconds:  -1,
	}); err == nil {
		t.Fatal("accepted a negative managed runtime preparation budget")
	}
}

func TestManagedRuntimeRepairReuseRestart(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	published := publishManagedRuntimeRepair(t, home, packageSpec, env, digest)

	fresh := newRepairReuseManager(t, home, command, env, digest)
	selected, outcome := fresh.selectManagedRuntimeRepair(context.Background())
	if outcome != managedruntime.RepairOutcomeHit || selected == nil {
		t.Fatalf("restart selection = (%v, %s), want a hit", selected, outcome)
	}
	if selected.CacheDirectory() != published.CacheDirectory() {
		t.Fatalf("restart selected %q, want %q", selected.CacheDirectory(), published.CacheDirectory())
	}
	if err := fresh.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("teardown restarted owner: %v", err)
	}
	if _, err := os.Stat(published.CacheDirectory()); err != nil {
		t.Fatalf("published generation after restarted owner teardown = %v, want present", err)
	}
}

func TestManagedRuntimeRepairReuseWithoutProvenance(t *testing.T) {
	home := t.TempDir()
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, _ := repairReuseEnvironment(t, home)
	manager := NewManager(&config.InstanceConfig{
		HomeDir: home, AgentType: "claude-acp", Protocol: agent.ProtocolACP,
		AgentArgs: command, AgentEnv: env, WorkDir: t.TempDir(),
	}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	if prepared, outcome := manager.selectManagedRuntimeRepair(context.Background()); prepared != nil || outcome != managedruntime.RepairOutcomeMiss {
		t.Fatalf("selection without provenance = (%v, %s), want a miss", prepared, outcome)
	}
	rewritten, err := manager.prepareManagedRuntimeCommand(append([]string(nil), command...))
	if err != nil || strings.Join(rewritten, "\x00") != strings.Join(command, "\x00") {
		t.Fatalf("command without a selected repair = %#v, %v; want unchanged", rewritten, err)
	}
}

func TestManagedRuntimeRepairPublicationAfterInitialization(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })

	store, err := managedruntime.OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
	if !ok {
		t.Fatal("build repair context")
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	if err := manager.installPreparedRuntime(prepared); err != nil {
		t.Fatalf("install unpublished generation: %v", err)
	}
	if _, outcome, lookupErr := store.Lookup(context.Background(), repairContext); lookupErr != nil || outcome != managedruntime.RepairOutcomeMiss {
		t.Fatalf("lookup before initialization = (%s, %v), want a miss", outcome, lookupErr)
	}
	manager.startupEvidenceMu.Lock()
	manager.startupGeneration = 7
	manager.startupEvidenceMu.Unlock()

	manager.ManagedRuntimeInitialized(6)
	if prepared.RepairPublished() {
		t.Fatal("stale process generation published the repair")
	}
	manager.ManagedRuntimeInitialized(7)
	if !prepared.RepairPublished() {
		t.Fatal("successful protocol initialization did not publish the repair")
	}
	hit, outcome, err := store.Lookup(context.Background(), repairContext)
	if err != nil || outcome != managedruntime.RepairOutcomeHit || hit == nil {
		t.Fatalf("lookup after initialization = (%v, %s, %v)", hit, outcome, err)
	}
	if hit.ProjectPrefix() != prepared.ProjectPrefix() {
		t.Fatalf("published generation paths = %q, want %q", hit.ProjectPrefix(), prepared.ProjectPrefix())
	}
}

func TestManagedRuntimeRepairRetirementKeepsCurrentProcessUsable(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })

	store, err := managedruntime.OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
	if !ok {
		t.Fatal("build repair context")
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	if err := manager.installPreparedRuntime(prepared); err != nil {
		t.Fatalf("install generation: %v", err)
	}
	prepared.RetireRepairPublication()
	manager.startupEvidenceMu.Lock()
	manager.startupGeneration = 9
	manager.startupEvidenceMu.Unlock()
	manager.ManagedRuntimeInitialized(9)
	if prepared.RepairPublished() {
		t.Fatal("retired publication window published the repair")
	}
	if _, err := os.Stat(filepath.Join(prepared.CacheDirectory(), "_npx", managedruntime.NpxExecutionCacheKey(packageSpec), "package.json")); err != nil {
		t.Fatalf("current process files after failed publication = %v, want present", err)
	}
	if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome == managedruntime.RepairOutcomeHit {
		t.Fatal("failed publication selected an unusable generation")
	}
}

func newRepairReuseManager(t *testing.T, home string, command, env []string, digest string) *Manager {
	t.Helper()
	manager := NewManager(&config.InstanceConfig{
		HomeDir: home, AgentType: "claude-acp", Protocol: agent.ProtocolACP,
		AgentArgs: command, AgentEnv: env, WorkDir: t.TempDir(),
	}, newTestLogger(t))
	manager.cfg.ManagedRuntimeInstallContext = &types.ManagedRuntimeInstallContext{
		InstallerEnvDigest: digest,
		OwnerScope:         managedruntime.RepairOwnerScopeTask("owner-a"),
	}
	return manager
}

func repairReuseEnvironment(t *testing.T, home string) ([]string, string) {
	t.Helper()
	bin := t.TempDir()
	for name, version := range map[string]string{"node": "24.0.0", "npm": "11.0.0"} {
		script := "#!/bin/sh\necho " + version + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{
		"HOME=" + home,
		"PATH=" + bin,
		"NPM_CONFIG_CACHE=" + filepath.Join(t.TempDir(), "npm-cache"),
	}
	return env, managedruntime.InstallerEnvironmentDigest(map[string]string{"PROFILE": "baseline"})
}

func storeRepairContext(
	store *managedruntime.RepairStore,
	packageSpec string,
	env []string,
	digest string,
) (managedruntime.RepairContext, bool) {
	repairContext, err := store.Context(managedruntime.RepairContextInput{
		OwnerScope: managedruntime.RepairOwnerScopeTask("owner-a"), InstallerEnvDigest: digest,
		PackageSpec: packageSpec, Protocol: string(agent.ProtocolACP), Env: env,
		InheritedEnv: os.Environ(),
	})
	if err != nil {
		return managedruntime.RepairContext{}, false
	}
	return repairContext, true
}

func publishManagedRuntimeRepair(t *testing.T, home, packageSpec string, env []string, digest string) *managedruntime.PreparedRuntime {
	t.Helper()
	store, err := managedruntime.OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
	if !ok {
		t.Fatal("build repair context")
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	if err := prepared.PublishRepair(); err != nil {
		t.Fatalf("publish generation: %v", err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared
}

func populateManagedRuntimeRepairCache(packageSpec string) managedruntime.ManagedRuntimeCommandRunner {
	return func(_ context.Context, command managedruntime.ManagedRuntimeCommand) error {
		cache, ok := runtimeCommandArgument(command.Args, "--cache")
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
		if runtime.GOOS == preparedRuntimeWindowsOSForTest {
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

const preparedRuntimeWindowsOSForTest = "windows"

func TestManagedRuntimeRepairNormalMissDetachesStaleHandle(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	published := publishManagedRuntimeRepair(t, home, packageSpec, env, digest)
	manager := newRepairReuseManager(t, home, command, env, digest)
	manager.preparedRuntime = published
	manager.CloseAdmission()
	if _, err := manager.startWithGeneration(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("start succeeded after admission closed")
	}
	if manager.preparedRuntime != nil {
		t.Fatal("a normal miss retained the previous reusable handle")
	}
	if _, err := os.Stat(published.CacheDirectory()); err != nil {
		t.Fatalf("detaching removed published files: %v", err)
	}
}

func TestManagedRuntimeRepairRejectsConfigurationChangeDuringRecovery(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	published := publishManagedRuntimeRepair(t, home, packageSpec, env, digest)
	manager := newRepairReuseManager(t, home, command, env, digest)
	manager.startupEvidence = &types.ManagedStartupEvidence{ProcessGeneration: 1}
	revision := manager.configRevision
	manager.configRevision++
	if _, err := manager.startWithGeneration(context.Background(), &types.ManagedRuntimePreparationDirective{
		Condition: types.ManagedRuntimeCacheRootManifestMissing, PackageSpec: packageSpec, FailedProcessGeneration: 1,
	}, published, &revision); err == nil {
		t.Fatal("started a replacement after its recovery configuration changed")
	}
	if _, err := os.Stat(published.CacheDirectory()); err != nil {
		t.Fatalf("published files were removed by the rejected recovery: %v", err)
	}
}

func TestManagedRuntimePublicationDeadlineUsesAdmissionTime(t *testing.T) {
	admission := time.Now()
	deadline := managedRuntimePublicationDeadline(admission, 60)
	if got, want := deadline.Sub(admission), 60*time.Second; got != want {
		t.Fatalf("publication budget = %s, want %s", got, want)
	}
	if remaining := deadline.Sub(admission.Add(50 * time.Second)); remaining > 10*time.Second {
		t.Fatalf("late remaining budget = %s, want at most the original deadline", remaining)
	}
	if deadline := managedRuntimePublicationDeadline(admission, 600); deadline.Sub(admission) != 90*time.Second {
		t.Fatal("large publication budget was not capped at 90 seconds")
	}
	if deadline := managedRuntimePublicationDeadline(admission, 0); deadline.Sub(admission) != 90*time.Second {
		t.Fatal("missing publication budget did not use the 90-second default")
	}
}

func TestManagedRuntimeRepairExpiryRetiresUninitializedCandidate(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	store, err := managedruntime.OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
	if !ok {
		t.Fatal("build repair context")
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	manager.preparedRuntime = prepared
	manager.preparedRuntimeRevision = manager.configRevision
	manager.expireManagedRuntimeRepair(prepared, manager.configRevision)
	if prepared.RepairPublished() {
		t.Fatal("expired publication window selected a generation")
	}
	if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome == managedruntime.RepairOutcomeHit {
		t.Fatal("expired publication selected an uninitialized generation")
	}
}

func TestManagedRuntimeRepairRevalidationRejectsChangedNPMRC(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	userConfig := filepath.Join(t.TempDir(), "npmrc")
	if err := os.WriteFile(userConfig, []byte("registry=https://one.example.test/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env = append(env, "NPM_CONFIG_USERCONFIG="+userConfig)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	store, err := managedruntime.OpenRepairStore(home)
	if err != nil {
		t.Fatalf("open repair store: %v", err)
	}
	repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
	if !ok {
		t.Fatal("build repair context")
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	prepared, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare generation: %v", err)
	}
	manager.preparedRuntime = prepared
	manager.preparedRuntimeRevision = manager.configRevision
	manager.preparedRepairContext = &repairContext
	manager.preparedRepairEnv = append([]string(nil), env...)
	manager.preparedRepairInherited = os.Environ()
	manager.startupEvidenceMu.Lock()
	manager.startupGeneration = 3
	manager.startupEvidenceMu.Unlock()
	manager.scheduleManagedRuntimePublicationExpiry(prepared, time.Now().Add(time.Hour), manager.configRevision)
	if err := os.WriteFile(userConfig, []byte("registry=https://two.example.test/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.ManagedRuntimeInitialized(3)
	if prepared.RepairPublished() {
		t.Fatal("published a repair under a changed npm configuration")
	}
	if manager.repairPublicationTimer != nil {
		t.Fatal("publication failure left expiry armed against a healthy initialized agent")
	}
	if !manager.repairInitialized {
		t.Fatal("initialization success did not fence expiry")
	}
	if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome == managedruntime.RepairOutcomeHit {
		t.Fatal("changed npm configuration selected the candidate")
	}
}
