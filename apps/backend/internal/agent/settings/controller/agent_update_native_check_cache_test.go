package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (h nativeCheckHarness) withClock() *testClock {
	clock := &testClock{now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	h.ctrl.SetRuntimeUpdateStatusClock(clock.Now)
	return clock
}

func requireRuns(t *testing.T, runner *fakeNativeCheckRunner, want int, step string) {
	t.Helper()
	if got := runner.callCount(); got != want {
		t.Fatalf("%s: native check runs = %d, want %d", step, got, want)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.1
func TestNativeCheckCacheTTLsAndSingleFlight(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeNativeCheckRunner)
		ttl   time.Duration
	}{
		{name: "update available", setup: func(r *fakeNativeCheckRunner) { r.setOutput(ompCanaryAvailable, 0) }, ttl: runtimeUpdateStatusSuccessTTL},
		{name: "up to date", setup: func(r *fakeNativeCheckRunner) { r.setOutput(ompUpToDate, 0) }, ttl: runtimeUpdateStatusSuccessTTL},
		{name: "managed installation", setup: func(r *fakeNativeCheckRunner) {
			r.setOutput("/opt/tern/bin/omp is installed and kept up to date by Tern; update it from Tern.\n", 0)
		}, ttl: runtimeUpdateStatusSuccessTTL},
		{name: "check failed", setup: func(r *fakeNativeCheckRunner) { r.setOutput(ompCheckFailed, 1) }, ttl: runtimeUpdateStatusFailureTTL},
		{name: "timeout", setup: func(r *fakeNativeCheckRunner) { r.setReason(agents.NativeUpdateCheckReasonTimeout) }, ttl: runtimeUpdateStatusFailureTTL},
		{name: "not installed", setup: func(r *fakeNativeCheckRunner) { r.setPath("") }, ttl: runtimeUpdateStatusFailureTTL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
			clock := h.withClock()
			tc.setup(h.runner)
			first := h.status(t, ompAgentID)
			wantRuns := 1
			if tc.name == "not installed" {
				wantRuns = 0
				if first.CheckReason != "not_installed" {
					t.Fatalf("unresolved executable status = %+v", first)
				}
			}
			requireRuns(t, h.runner, wantRuns, "first status")
			clock.Advance(tc.ttl - time.Minute)
			_ = h.status(t, ompAgentID)
			if err := h.ctrl.RunRuntimeUpdatePass(context.Background()); err != nil {
				t.Fatal(err)
			}
			requireRuns(t, h.runner, wantRuns, "inside TTL")
			if tc.name == "not installed" {
				h.runner.setPath(filepath.Join(h.home, "bin", "omp"))
				h.runner.setOutput(ompUpToDate, 0)
				_ = h.status(t, ompAgentID)
				requireRuns(t, h.runner, 1, "after the executable appears")
				return
			}
			clock.Advance(2 * time.Minute)
			_ = h.status(t, ompAgentID)
			requireRuns(t, h.runner, 2, "after TTL")
		})
	}
	t.Run("cancelled is never cached", func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		h.runner.setReason(agents.NativeUpdateCheckReasonCancelled)
		if status := h.status(t, ompAgentID); status.CheckReason != "cancelled" || status.CheckedAt != nil {
			t.Fatalf("cancelled status = %+v", status)
		}
		_ = h.status(t, ompAgentID)
		requireRuns(t, h.runner, 2, "after a cancelled check")
	})
	t.Run("concurrent requests share one run", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
			h.runner.setOutput(ompUpToDate, 0)
			_, release := h.runner.gated()
			var wg sync.WaitGroup
			statuses := make([]dto.AgentUpdateStatusDTO, 4)
			for i := range statuses {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, err := h.ctrl.ListAgentUpdateStatuses(context.Background())
					if err == nil && len(result.Statuses) == 1 {
						statuses[i] = result.Statuses[0]
					}
				}()
			}
			cancelled, cancel := context.WithCancel(context.Background())
			cancelledDone := make(chan struct{})
			go func() {
				defer close(cancelledDone)
				_, _ = h.ctrl.ListAgentUpdateStatuses(cancelled)
			}()
			// Every request is now blocked: one in the gated run, the rest
			// waiting on the shared flight rather than running their own.
			synctest.Wait()
			requireRuns(t, h.runner, 1, "while the first run is in flight")
			cancel()
			synctest.Wait()
			select {
			case <-cancelledDone:
			default:
				t.Fatal("a cancelled subscriber did not return while the shared run was in flight")
			}
			release()
			wg.Wait()
			requireRuns(t, h.runner, 1, "concurrent requests")
			for _, status := range statuses {
				if status.CheckState != dto.AgentUpdateCheckStateUpToDate {
					t.Errorf("subscriber status = %+v", status)
				}
			}
		})
	})
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2
func TestNativeCheckRechecksOnVersionPathOrConfigChange(t *testing.T) {
	t.Run("runtime version", func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		h.runner.setOutput(ompUpToDate, 0)
		_ = h.status(t, ompAgentID)
		h.updater.setACPVersion("18.13.4")
		_ = h.status(t, ompAgentID)
		requireRuns(t, h.runner, 2, "after a runtime version change")
	})
	t.Run("resolved path", func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		h.runner.setOutput(ompUpToDate, 0)
		_ = h.status(t, ompAgentID)
		h.runner.setPath(filepath.Join(h.home, ".bun", "bin", "omp"))
		_ = h.status(t, ompAgentID)
		requireRuns(t, h.runner, 2, "after a resolved path change")
	})
	for _, tc := range nativeCheckConfigCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
			h.runner.setEnv(append(h.baseEnv(), tc.env(h)...)...)
			if tc.prepare != nil {
				tc.prepare(t, h)
			}
			h.runner.setOutput(ompUpToDate, 0)
			_ = h.status(t, ompAgentID)
			_ = h.status(t, ompAgentID)
			requireRuns(t, h.runner, 1, "unchanged configuration")
			tc.change(t, h)
			_ = h.status(t, ompAgentID)
			requireRuns(t, h.runner, 2, "after "+tc.name)
		})
	}
}

type nativeCheckConfigCase struct {
	name    string
	env     func(nativeCheckHarness) []string
	prepare func(*testing.T, nativeCheckHarness)
	change  func(*testing.T, nativeCheckHarness)
}

func nativeCheckConfigCases() []nativeCheckConfigCase {
	noEnv := func(nativeCheckHarness) []string { return nil }
	edit := func(path func(nativeCheckHarness) string) func(*testing.T, nativeCheckHarness) {
		return func(t *testing.T, h nativeCheckHarness) { writeNativeCheckFile(t, path(h), "edited: longer value\n") }
	}
	defaultAgent := func(h nativeCheckHarness) string { return filepath.Join(h.home, ".omp", "agent", "config.yml") }
	workAgentDir := func(h nativeCheckHarness) string { return filepath.Join(h.home, ".omp", "profiles", "work", "agent") }
	manifest := func(h nativeCheckHarness) string {
		return filepath.Join(filepath.Dir(filepath.Dir(h.runner.path)), "manager.json")
	}
	inherited := func(omp string) func(nativeCheckHarness) []string {
		return func(h nativeCheckHarness) []string {
			return []string{"OMP_PROFILE=" + omp, "PI_PROFILE=work", "PI_CODING_AGENT_DIR=" + workAgentDir(h)}
		}
	}
	return []nativeCheckConfigCase{
		{name: "default config", env: noEnv, change: edit(defaultAgent)},
		{
			name: "named profile config", env: func(nativeCheckHarness) []string { return []string{"OMP_PROFILE=work"} },
			change: edit(func(h nativeCheckHarness) string { return filepath.Join(workAgentDir(h), "config.yml") }),
		},
		{
			name: "config overlay", env: func(h nativeCheckHarness) []string {
				return []string{"PI_CONFIG_FILES=" + filepath.Join(h.home, "overlay.yml")}
			},
			change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.home, "overlay.yml") }),
		},
		{name: "new project file", env: noEnv, change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.cwd, ".omp", "rules.json") })},
		{
			name: "known project candidate in a large directory", env: noEnv,
			prepare: func(t *testing.T, h nativeCheckHarness) {
				writeNativeCheckFile(t, filepath.Join(h.cwd, ".omp", "config.yml"), "a")
				for i := range 300 {
					writeNativeCheckFile(t, filepath.Join(h.cwd, ".omp", fmt.Sprintf("entry-%03d", i)), "x")
				}
			},
			change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.cwd, ".omp", "config.yml") }),
		},
		{name: "ownership manifest created", env: noEnv, change: edit(manifest)},
		{
			name: "ownership manifest removed", env: noEnv,
			prepare: func(t *testing.T, h nativeCheckHarness) { writeNativeCheckFile(t, manifest(h), "{}") },
			change: func(t *testing.T, h nativeCheckHarness) {
				if err := os.Remove(manifest(h)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{name: "npmrc", env: noEnv, change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.home, ".npmrc") })},
		{name: "explicitly empty profile ignores inherited agent dir", env: inherited(""), change: edit(defaultAgent)},
		{name: "default profile ignores inherited agent dir", env: inherited("default"), change: edit(defaultAgent)},
		{name: "home dotenv", env: noEnv, change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.home, ".env") })},
		{name: "agent dotenv", env: noEnv, change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.home, ".omp", "agent", ".env") })},
		{name: "project dotenv", env: noEnv, change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.cwd, ".env") })},
		{
			name: "named profile root dotenv", env: func(nativeCheckHarness) []string { return []string{"OMP_PROFILE=work"} },
			change: edit(func(h nativeCheckHarness) string { return filepath.Join(h.home, ".omp", "profiles", "work", ".env") }),
		},
	}
}

func writeNativeCheckFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2
func TestNativeCheckInvalidatedByTerminalHarnessJob(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		h.runner.setOutput(ompCanaryAvailable, 0)
		_ = h.status(t, ompAgentID)
		h.updater.refreshCaps = acpCapabilities("18.22.0-canary.3")
		runNativeHarnessJob(t, h, dto.AgentUpdateJobStatusSucceeded)
		// Restore the pre-job identity so only the generation can force a recheck.
		h.updater.setACPVersion("18.13.3")
		_ = h.status(t, ompAgentID)
		requireRuns(t, h.runner, 2, "after a successful harness job")
	})
	t.Run("failure", func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		h.runner.setOutput(ompCanaryAvailable, 0)
		_ = h.status(t, ompAgentID)
		h.updater.runErr = errors.New("installer failed")
		runNativeHarnessJob(t, h, dto.AgentUpdateJobStatusFailed)
		_ = h.status(t, ompAgentID)
		requireRuns(t, h.runner, 2, "after a failed harness job")
	})
	t.Run("managed activation invalidates only its package", func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP(), agents.NewClaudeACP())
		h.ctrl.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "0.100.0", nil })
		h.runner.setOutput(ompCanaryAvailable, 0)
		_ = h.status(t, ompAgentID)
		claudePackage := agents.RuntimeUpdateCapabilities(agents.NewClaudeACP()).Source.NPM
		pinnedKey := runtimeUpdateStatusCacheKey(claudePackage, dto.AgentUpdateModePinned)
		h.ctrl.invalidateRuntimeUpdateStatusForJob(RuntimeStatusInvalidation{
			AgentName: "claude-acp", Package: claudePackage, UpdateMode: dto.AgentUpdateModePinned,
		})
		h.ctrl.runtimeUpdateStatusMu.Lock()
		_, pinnedCached := h.ctrl.runtimeUpdateStatusCache[pinnedKey]
		h.ctrl.runtimeUpdateStatusMu.Unlock()
		if pinnedCached {
			t.Error("managed activation kept its package status")
		}
		_ = h.status(t, ompAgentID)
		requireRuns(t, h.runner, 1, "after another agent's managed activation")
	})
}

func acpCapabilities(version string) hostutility.AgentCapabilities {
	return hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: version}
}

func runNativeHarnessJob(t *testing.T, h nativeCheckHarness, want dto.AgentUpdateJobStatus) *dto.AgentUpdateJobDTO {
	t.Helper()
	hub := newUpdateTerminalBroadcaster()
	h.ctrl.SetJobBroadcaster(hub)
	h.ctrl.updateJobStore.onRefresh = nil
	job, err := h.ctrl.EnqueueAgentUpdate(context.Background(), ompAgentID, "")
	if err != nil {
		t.Fatal(err)
	}
	return waitForUpdateStatus(t, hub.completed, job.JobID, want)
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2
func TestNativeCheckStaleFlightCannotRepopulateAfterInvalidation(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.queue(ompUpToDate, ompCanaryAvailable)
	started, release := h.runner.gated()
	t.Cleanup(release)
	stale := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "stale check start", started)
	h.ctrl.InvalidateNativeUpdateCheck(ompAgentID)
	fresh := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("post-invalidation request joined the stale check")
	}
	release()
	if got := awaitNativeStatus(t, "stale subscriber status", stale); got.CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("stale subscriber = %+v", got)
	}
	if got := awaitNativeStatus(t, "fresh subscriber status", fresh); got.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("fresh subscriber = %+v", got)
	}
	if got := h.status(t, ompAgentID); got.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("cached status = %+v, want the fresh check", got)
	}
	requireRuns(t, h.runner, 2, "after the barrier")
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2
func TestNativeCheckTerminalJobPublishesAfterInvalidation(t *testing.T) {
	for _, outcome := range []dto.AgentUpdateJobStatus{dto.AgentUpdateJobStatusSucceeded, dto.AgentUpdateJobStatusFailed} {
		t.Run(string(outcome), func(t *testing.T) {
			h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
			h.runner.queue(ompUpToDate, ompCanaryAvailable)
			if outcome == dto.AgentUpdateJobStatusSucceeded {
				h.updater.refreshCaps = acpCapabilities("18.13.4")
			} else {
				h.updater.runErr = errors.New("installer failed")
			}
			hub := newUpdateTerminalBroadcaster()
			h.ctrl.SetJobBroadcaster(hub)
			h.ctrl.updateJobStore.onRefresh = nil
			started, release := h.runner.gated()
			defer release()
			preJob := fetchNativeStatusAsync(h.ctrl, ompAgentID)
			awaitNativeSignal(t, "pre-job check start", started)
			job, err := h.ctrl.EnqueueAgentUpdate(context.Background(), ompAgentID, "")
			if err != nil {
				t.Fatal(err)
			}
			waitForTerminalJobSnapshot(t, h.ctrl, job.JobID)
			if outcome == dto.AgentUpdateJobStatusSucceeded {
				h.updater.setACPVersion("18.13.3")
			}
			post := fetchNativeStatusAsync(h.ctrl, ompAgentID)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("status after the terminal job joined the pre-job check")
			}
			release()
			_ = awaitNativeStatus(t, "pre-job status", preJob)
			if got := awaitNativeStatus(t, "post-job status", post); got.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
				t.Errorf("post-job status = %+v, want the fresh check", got)
			}
			<-hub.completed
		})
	}
}

func waitForTerminalJobSnapshot(t *testing.T, ctrl *Controller, jobID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if job, found := ctrl.updateJobStore.Get(jobID); found && job.FinishedAt != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("job %s did not become terminal", jobID)
}

type recordedInvalidation struct {
	invalidation  RuntimeStatusInvalidation
	jobFinished   bool
	claimReleased bool
}

// recordInvalidations captures whether the job was still active and the
// maintenance claim still held when each invalidation ran.
func recordInvalidations(store *AgentUpdateJobStore) (func() []recordedInvalidation, func(RuntimeStatusInvalidation)) {
	var mu sync.Mutex
	var recorded []recordedInvalidation
	return func() []recordedInvalidation {
			mu.Lock()
			defer mu.Unlock()
			return append([]recordedInvalidation(nil), recorded...)
		}, func(invalidation RuntimeStatusInvalidation) {
			_, active := store.GetActive(invalidation.AgentName)
			store.maintenance.mu.Lock()
			_, held := store.maintenance.byAgent[invalidation.AgentName]
			store.maintenance.mu.Unlock()
			mu.Lock()
			defer mu.Unlock()
			recorded = append(recorded, recordedInvalidation{invalidation: invalidation, jobFinished: !active, claimReleased: !held})
		}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2
func TestAgentUpdateJobStatusInvalidationContract(t *testing.T) {
	t.Run("self-update invalidates before publishing the failure", func(t *testing.T) {
		updater := &harnessTestUpdater{}
		updater.current, updater.currentFound = acpCapabilities("18.13.3"), true
		updater.runErr = errors.New("installer failed")
		store, completed := newUpdateTestStore(updater, newMaintenanceCoordinator(), nil)
		snapshot, invalidator := recordInvalidations(store)
		store.SetStatusInvalidator(invalidator)
		job, err := store.EnqueueHarness(ompAgentID, agents.NewOmpACP().HarnessUpdate(), agents.NewCommand("omp", "acp"))
		if err != nil {
			t.Fatal(err)
		}
		waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusFailed)
		got := snapshot()
		want := RuntimeStatusInvalidation{AgentName: ompAgentID, Package: ompPackage, UpdateMode: dto.AgentUpdateModeSelfUpdate}
		if len(got) != 1 || got[0].invalidation != want || got[0].jobFinished || got[0].claimReleased {
			t.Fatalf("self-update invalidations = %+v, want one %+v before publication", got, want)
		}
	})
	t.Run("managed activation invalidates its package after publication", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
		updater := &fakeRuntimeUpdater{
			current: acpCapabilities("1.0.0"), currentFound: true, target: "1.1.0",
			refreshCaps: acpCapabilities("1.1.0"),
		}
		store, completed := newUpdateTestStore(updater, newMaintenanceCoordinator(), nil)
		snapshot, invalidator := recordInvalidations(store)
		store.SetStatusInvalidator(invalidator)
		job, err := store.Enqueue("opencode-acp", agents.ManagedNPMRuntimeSpec{Package: "opencode-ai", ACPArgs: []string{"acp"}, NativeBinary: "opencode"})
		if err != nil {
			t.Fatal(err)
		}
		waitForUpdateStatus(t, completed, job.ID, dto.AgentUpdateJobStatusSucceeded)
		got := snapshot()
		if len(got) != 1 || got[0].invalidation.Package != "opencode-ai" || got[0].invalidation.AgentName != "opencode-acp" || !got[0].jobFinished {
			t.Fatalf("managed invalidations = %+v, want one package invalidation after publication", got)
		}
	})
}
