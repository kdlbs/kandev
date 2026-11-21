package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.17
func TestManagedRuntimeRepairRejectedStartPreservesTicket(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	store := openManagedRuntimeRepairStoreForTest(t, home)
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
		t.Fatalf("install pending generation: %v", err)
	}
	manager.startupEvidenceMu.Lock()
	manager.startupGeneration = 7
	manager.startupEvidenceMu.Unlock()
	deadline := time.Now().Add(time.Hour)
	manager.scheduleManagedRuntimePublicationExpiry(prepared, deadline, manager.configRevision)
	timer := manager.repairPublicationTimer
	if timer == nil {
		t.Fatal("pending generation has no publication timer")
	}
	manager.status.Store(StatusRunning)

	if _, err := manager.startWithGeneration(context.Background(), nil, nil, nil, nil, nil); err == nil {
		t.Fatal("duplicate start succeeded while the agent was running")
	}
	if manager.preparedRuntime != prepared {
		t.Fatal("rejected start detached the pending repair ticket")
	}
	if manager.repairPublicationTimer != timer {
		t.Fatal("rejected start replaced the original publication deadline")
	}

	// The rejected request was not the ticket owner: initialization still
	// publishes and a later consumer can acquire the context lock.
	manager.status.Store(StatusStarting)
	manager.ManagedRuntimeInitialized(context.Background(), 7)
	if !prepared.RepairPublished() {
		t.Fatal("rejected start prevented initialization publication")
	}
	if hit, outcome, lookupErr := store.Lookup(context.Background(), repairContext); lookupErr != nil ||
		outcome != managedruntime.RepairOutcomeHit || hit == nil {
		t.Fatalf("lookup after rejected start = (%v, %s, %v), want hit", hit, outcome, lookupErr)
	}
	nextLease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("acquire context lock after rejected start: %v", err)
	}
	if err := nextLease.Close(); err != nil {
		t.Fatalf("close successor lease: %v", err)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.17
func TestManagedRuntimeRepairClosedAdmissionPreservesStaleHandle(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	published := publishManagedRuntimeRepair(t, home, packageSpec, env, digest)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	manager.preparedRuntime = published
	manager.CloseAdmission()
	if _, err := manager.startWithGeneration(context.Background(), nil, nil, nil, nil, nil); err == nil {
		t.Fatal("start succeeded after admission closed")
	}
	if manager.preparedRuntime != published {
		t.Fatal("rejected start detached the selected repair handle")
	}
	if _, err := os.Stat(published.CacheDirectory()); err != nil {
		t.Fatalf("detaching removed published files: %v", err)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.17
func TestManagedRuntimeRepairAcceptedNormalMissDetachesStaleHandle(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	published := publishManagedRuntimeRepair(t, home, packageSpec, env, digest)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	manager.preparedRuntime = published
	// The command is intentionally not launchable. The accepted stopped-process
	// start must release the incompatible handle before its later failure.
	manager.cfg.AgentArgs = []string{"/missing-managed-runtime"}
	manager.managedRuntimeCommand = append([]string(nil), manager.cfg.AgentArgs...)
	if _, err := manager.startWithGeneration(context.Background(), nil, nil, nil, nil, nil); err == nil {
		t.Fatal("an invalid command unexpectedly started a process")
	}
	if manager.preparedRuntime != nil {
		t.Fatal("accepted normal miss retained the stale reusable handle")
	}
	if _, err := os.Stat(published.CacheDirectory()); err != nil {
		t.Fatalf("detaching removed published files: %v", err)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestManagedRuntimeRepairLookupCancellationDoesNotStartProvider(t *testing.T) {
	home := t.TempDir()
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.StartWithGeneration(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start error = %v, want context cancellation", err)
	}
	if manager.ProcessGeneration() != 0 {
		t.Fatalf("cancelled lookup started provider generation %d", manager.ProcessGeneration())
	}
}

func newBlockedRepairLookupManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair store requires native ACL verification on Windows")
	}
	home := t.TempDir()
	bin := t.TempDir()
	started := filepath.Join(bin, "started")
	release := filepath.Join(bin, "release")
	npmScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  touch \"$STARTED\"\n" +
		"  while [ ! -f \"$RELEASE\" ]; do sleep 0.01; done\n" +
		"  echo 11.0.0\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(npmScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho 24.0.0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + home,
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STARTED=" + started,
		"RELEASE=" + release,
	}
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	manager := NewManager(&config.InstanceConfig{
		HomeDir: home, AgentType: "claude-acp", Protocol: agent.ProtocolACP,
		AgentArgs: command, AgentEnv: env, WorkDir: t.TempDir(),
		ManagedRuntimeInstallContext: &types.ManagedRuntimeInstallContext{
			InstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(map[string]string{"PROFILE": "baseline"}),
			OwnerScope:         managedruntime.RepairOwnerScopeTask("owner-a"),
		},
	}, newTestLogger(t))
	t.Cleanup(func() {
		_ = os.WriteFile(release, []byte("release"), 0o600)
		_ = manager.StopForTeardown(context.Background())
	})
	return manager, started, release
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestManagedRuntimeRepairLookupRequestCancellation(t *testing.T) {
	manager, started, _ := newBlockedRepairLookupManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := manager.StartWithGeneration(ctx)
		done <- err
	}()
	waitForFile(t, started)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled lookup error = %v, want context cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled lookup did not return")
	}
	if manager.ProcessGeneration() != 0 {
		t.Fatalf("cancelled lookup started provider generation %d", manager.ProcessGeneration())
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestManagedRuntimeRepairLookupRecoveryDeadline(t *testing.T) {
	manager, started, _ := newBlockedRepairLookupManager(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := manager.StartWithGeneration(ctx)
		done <- err
	}()
	waitForFile(t, started)
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline lookup error = %v, want deadline exceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("deadline lookup did not return")
	}
	if manager.ProcessGeneration() != 0 {
		t.Fatalf("deadline lookup started provider generation %d", manager.ProcessGeneration())
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestManagedRuntimeRepairLookupStopsWithAdmission(t *testing.T) {
	manager, started, _ := newBlockedRepairLookupManager(t)
	done := make(chan error, 1)
	go func() {
		_, err := manager.StartWithGeneration(context.Background())
		done <- err
	}()
	waitForFile(t, started)
	manager.CloseAdmission()
	drainCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.WaitForAdmission(drainCtx); err != nil {
		t.Fatalf("admission did not drain after lookup cancellation: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lookup did not return after admission closed")
	}
	if manager.ProcessGeneration() != 0 {
		t.Fatalf("admission-closed lookup started provider generation %d", manager.ProcessGeneration())
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.19
func TestManagedRuntimeRepairContextDoesNotBlockStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reusable repair store requires native ACL verification on Windows")
	}
	home := t.TempDir()
	bin := t.TempDir()
	started := filepath.Join(bin, "started")
	release := filepath.Join(bin, "release")
	npmScript := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  touch \"$STARTED\"\n" +
		"  while [ ! -f \"$RELEASE\" ]; do sleep 0.01; done\n" +
		"  echo 11.0.0\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(npmScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho 24.0.0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + home,
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"STARTED=" + started,
		"RELEASE=" + release,
	}
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	manager := NewManager(&config.InstanceConfig{
		HomeDir: home, AgentType: "claude-acp", Protocol: agent.ProtocolACP,
		AgentArgs: command, AgentEnv: env, WorkDir: t.TempDir(),
		ManagedRuntimeInstallContext: &types.ManagedRuntimeInstallContext{
			InstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(map[string]string{"PROFILE": "baseline"}),
			OwnerScope:         managedruntime.RepairOwnerScopeTask("owner-a"),
		},
	}, newTestLogger(t))
	t.Cleanup(func() {
		_ = os.WriteFile(release, []byte("release"), 0o600)
		_ = manager.StopForTeardown(context.Background())
	})
	if manager.repairStore == nil {
		t.Fatal("repair store is unavailable")
	}
	manager.startMu.Lock()
	revision := manager.configRevision
	stopGeneration := manager.stopGeneration
	manager.startMu.Unlock()

	lookupDone := make(chan error, 1)
	go func() {
		_, _, err := manager.repairContextForStartWithError(context.Background())
		lookupDone <- err
	}()
	waitForFile(t, started)
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	stopStarted := time.Now()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("Stop during toolchain lookup: %v", err)
	}
	if elapsed := time.Since(stopStarted); elapsed > time.Second {
		t.Fatalf("Stop waited %s for toolchain lookup", elapsed)
	}
	if _, err := manager.startWithGeneration(context.Background(), nil, nil, &revision, &stopGeneration, nil); err == nil {
		t.Fatal("late start authority survived a concurrent Stop")
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-lookupDone:
		if err != nil {
			t.Logf("toolchain lookup error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("toolchain lookup did not finish after release")
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.13
func TestManagedRuntimePublicationContextRace(t *testing.T) {
	for _, race := range []string{"configuration", "successor", "stop", "cancelled"} {
		t.Run(race, func(t *testing.T) {
			home := t.TempDir()
			packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
			command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
			env, digest := repairReuseEnvironment(t, home)
			manager := newRepairReuseManager(t, home, command, env, digest)
			t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
			store := openManagedRuntimeRepairStoreForTest(t, home)
			repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
			if !ok {
				t.Fatal("build repair context")
			}
			lease, err := store.BeginRecovery(context.Background(), repairContext)
			if err != nil {
				t.Fatalf("begin recovery: %v", err)
			}
			candidate, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
			if err != nil {
				t.Fatalf("prepare generation: %v", err)
			}
			if err := manager.installPreparedRuntime(candidate); err != nil {
				t.Fatalf("install candidate: %v", err)
			}
			manager.preparedRepairContext = &repairContext
			manager.preparedRepairEnv = append([]string(nil), env...)
			manager.preparedRepairInherited = os.Environ()
			manager.startupEvidenceMu.Lock()
			manager.startupGeneration = 11
			manager.startupEvidenceMu.Unlock()

			started := make(chan struct{})
			release := make(chan struct{})
			manager.repairPublicationContext = func(ctx context.Context, input managedruntime.RepairContextInput) (managedruntime.RepairContext, error) {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return managedruntime.RepairContext{}, ctx.Err()
				}
				return store.Context(ctx, input)
			}
			publicationCtx := context.Background()
			cancelPublication := func() {}
			if race == "cancelled" {
				var cancel context.CancelFunc
				publicationCtx, cancel = context.WithCancel(context.Background())
				cancelPublication = cancel
			}
			published := make(chan struct{})
			go func() {
				manager.ManagedRuntimeInitialized(publicationCtx, 11)
				close(published)
			}()
			waitForFileSignal(t, started)
			var successor *managedruntime.PreparedRuntime
			switch race {
			case "configuration":
				manager.startMu.Lock()
				manager.configRevision++
				manager.startMu.Unlock()
			case "successor":
				successor, err = managedruntime.PreparePrivateRuntime(context.Background(), packageSpec, env, populateManagedRuntimeRepairCache(packageSpec))
				if err != nil {
					t.Fatalf("prepare successor: %v", err)
				}
				if err := manager.installPreparedRuntime(successor); err != nil {
					t.Fatalf("install successor: %v", err)
				}
			case "cancelled":
				cancelPublication()
			case "stop":
				stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := manager.Stop(stopCtx); err != nil {
					cancel()
					t.Fatalf("Stop during publication: %v", err)
				}
				cancel()
			}
			close(release)
			select {
			case <-published:
			case <-time.After(2 * time.Second):
				t.Fatal("publication callback did not settle")
			}
			if candidate.RepairPublished() {
				t.Fatal("late publication result published the replaced candidate")
			}
			if _, outcome, _ := store.Lookup(context.Background(), repairContext); outcome == managedruntime.RepairOutcomeHit {
				t.Fatal("late publication result selected an unusable generation")
			}
			if successor != nil {
				if manager.preparedRuntime != successor {
					t.Fatal("late publication result detached the successor generation")
				}
				if successor.RepairPublished() {
					t.Fatal("successor was published by the replaced candidate callback")
				}
			}
			if race == "configuration" && manager.preparedRuntime != candidate {
				t.Fatal("configuration race detached the current candidate")
			}
			if race == "stop" && manager.Status() != StatusStopped {
				t.Fatalf("stop race manager status = %s, want stopped", manager.Status())
			}
			if race == "cancelled" && manager.preparedRuntime != candidate {
				t.Fatal("cancellation race detached the current candidate")
			}
		})
	}
}

func waitForFileSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the publication barrier")
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.17
func TestManagedRuntimeRepairStaleExpiryDoesNotStopSuccessor(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	store := openManagedRuntimeRepairStoreForTest(t, home)
	repairContext, ok := storeRepairContext(store, packageSpec, env, digest)
	if !ok {
		t.Fatal("build repair context")
	}
	lease, err := store.BeginRecovery(context.Background(), repairContext)
	if err != nil {
		t.Fatalf("begin recovery: %v", err)
	}
	oldCandidate, err := lease.Prepare(context.Background(), env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare old generation: %v", err)
	}
	if err := manager.installPreparedRuntime(oldCandidate); err != nil {
		t.Fatalf("install old candidate: %v", err)
	}
	successor, err := managedruntime.PreparePrivateRuntime(context.Background(), packageSpec, env, populateManagedRuntimeRepairCache(packageSpec))
	if err != nil {
		t.Fatalf("prepare successor: %v", err)
	}
	t.Cleanup(func() { _ = successor.Close() })
	if err := manager.installPreparedRuntime(successor); err != nil {
		t.Fatalf("install successor: %v", err)
	}
	manager.expireManagedRuntimeRepair(oldCandidate, manager.configRevision)
	if manager.preparedRuntime != successor {
		t.Fatal("stale expiry detached the successor generation")
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.12
func TestManagedRuntimeRepairExpiryDoesNotStopInitializedAgent(t *testing.T) {
	home := t.TempDir()
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	env, digest := repairReuseEnvironment(t, home)
	manager := newRepairReuseManager(t, home, command, env, digest)
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	store := openManagedRuntimeRepairStoreForTest(t, home)
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
		t.Fatalf("install candidate: %v", err)
	}
	manager.startupEvidenceMu.Lock()
	manager.startupGeneration = 5
	manager.startupEvidenceMu.Unlock()
	manager.status.Store(StatusRunning)

	expiryWaiting := make(chan struct{})
	continueExpiry := make(chan struct{})
	manager.repairExpiryBeforeLock = func() {
		close(expiryWaiting)
		<-continueExpiry
	}
	expired := make(chan struct{})
	go func() {
		manager.expireManagedRuntimeRepair(prepared, manager.configRevision)
		close(expired)
	}()
	<-expiryWaiting
	manager.startOperationMu.Lock()
	manager.ManagedRuntimeInitialized(context.Background(), 5)
	manager.startOperationMu.Unlock()
	close(continueExpiry)
	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatal("expiry did not settle")
	}
	if !prepared.RepairPublished() {
		t.Fatal("initialization publication was retired by the expiry race")
	}
	if manager.Status() == StatusStopped {
		t.Fatal("expiry stopped a healthy initialized agent")
	}
}
