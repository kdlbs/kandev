package controller

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestNativeCheckIdentityObservationsAreBoundedAndRequestCancellable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
		gate := make(chan struct{})
		var gateOnce sync.Once
		release := func() { gateOnce.Do(func() { close(gate) }) }
		entered := make(chan struct{}, runtimeUpdateStatusMaxConcurrent+1)
		h.runner.setEnvironmentHook(func() {
			entered <- struct{}{}
			<-gate
		})

		type result struct {
			status dto.AgentUpdateStatusDTO
			err    error
		}
		requestsStarted := make(chan struct{}, runtimeUpdateStatusMaxConcurrent+1)
		results := make(chan result, runtimeUpdateStatusMaxConcurrent+1)
		cancels := make([]context.CancelFunc, 0, runtimeUpdateStatusMaxConcurrent+1)
		defer func() {
			for _, cancel := range cancels {
				cancel()
			}
			release()
			synctest.Wait()
		}()
		for range runtimeUpdateStatusMaxConcurrent + 1 {
			ctx, cancel := context.WithCancel(context.Background())
			cancels = append(cancels, cancel)
			go func() {
				requestsStarted <- struct{}{}
				statuses, err := h.ctrl.ListAgentUpdateStatuses(ctx)
				item := result{err: err}
				if err == nil && len(statuses.Statuses) == 1 {
					item.status = statuses.Statuses[0]
				}
				results <- item
			}()
		}
		for range runtimeUpdateStatusMaxConcurrent + 1 {
			<-requestsStarted
		}
		synctest.Wait()
		activeReads := len(entered)

		for _, cancel := range cancels {
			cancel()
		}
		synctest.Wait()
		completedAfterCancellation := len(results)
		release()
		synctest.Wait()
		if activeReads != runtimeUpdateStatusMaxConcurrent {
			t.Errorf("active identity reads = %d, want the limit %d", activeReads, runtimeUpdateStatusMaxConcurrent)
		}
		if completedAfterCancellation != runtimeUpdateStatusMaxConcurrent+1 {
			t.Errorf("completed requests after cancellation = %d, want %d", completedAfterCancellation, runtimeUpdateStatusMaxConcurrent+1)
		}
		for range len(results) {
			item := <-results
			if item.err != nil || item.status.AgentName != ompAgentID {
				t.Errorf("cancelled status result = %+v, want one OMP status without error", item)
			}
		}
	})
}

func TestNativeCheckPostRunObservationDoesNotDelayDecidedResultPastDeadline(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	lookup := h.ctrl.prepareNativeUpdateCheck(context.Background(), ompAgentID, agents.NewOmpACP().NativeUpdateCheck())
	gate, entered, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate) }) }
	defer release()
	h.runner.setEnvironmentHook(func() {
		close(entered)
		<-gate
		close(exited)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result := make(chan runtimeUpdateStatusCacheEntry, 1)
	lifetime := h.ctrl.currentNativeCheckLifetime()
	go func() { result <- h.ctrl.executeNativeUpdateCheck(ctx, lifetime, lookup, time.Now()) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("post-run identity observation did not start")
	}
	var entry runtimeUpdateStatusCacheEntry
	select {
	case entry = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("post-run identity observation delayed the completed check")
	}
	if entry.state != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("decided result = %+v, want up_to_date", entry)
	}
	if entry.expiresAt.IsZero() {
		t.Error("the completed result was discarded instead of being returned uncached")
	}
	if _, cached := h.ctrl.freshRuntimeUpdateStatusEntry(lookup.key, time.Now()); cached {
		t.Error("a result with incomplete identity revalidation was cached")
	}
	release()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the underlying identity observation did not finish after release")
	}
}

func TestNativeCheckSymlinkRetargetChangesIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	dir := filepath.Join(h.home, "install", "versions")
	first, second := filepath.Join(dir, "18.13.3"), filepath.Join(dir, "18.13.4")
	writeNativeCheckFile(t, first, "first binary")
	writeNativeCheckFile(t, second, "second binary")
	link := filepath.Join(h.home, "omp")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	h.runner.setPath(link)
	spec := agents.NewOmpACP().NativeUpdateCheck()
	before := h.ctrl.prepareNativeUpdateCheck(context.Background(), ompAgentID, spec)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	after := h.ctrl.prepareNativeUpdateCheck(context.Background(), ompAgentID, spec)
	if before.key == after.key {
		t.Fatal("retargeted executable to a different real path but cache identity did not change")
	}
}

func TestNativeCheckFoldedXDGBunfigChangesIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	xdg := filepath.Join(h.home, "xdg")
	h.runner.setEnv(append(h.baseEnv(), "xdg_config_home="+xdg)...)
	config := filepath.Join(xdg, ".bunfig.toml")
	writeNativeCheckFile(t, config, "[install]\nregistry = 'https://old.invalid'\n")
	spec := agents.NewOmpACP().NativeUpdateCheck()
	before := h.ctrl.prepareNativeUpdateCheck(context.Background(), ompAgentID, spec)
	writeNativeCheckFile(t, config, "[install]\nregistry = 'https://new-and-longer.invalid'\n")
	after := h.ctrl.prepareNativeUpdateCheck(context.Background(), ompAgentID, spec)
	if before.key == after.key {
		t.Fatal("changed upstream-selected XDG bunfig size but cache identity did not change")
	}
}

// A result produced after the executable changed during the wait or the run
// describes another installation, so it must not be cached under the key that
// was prepared earlier.
func TestNativeCheckIdentityChangeDuringRunIsNotCached(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	first := filepath.Join(h.home, "install", "a", "omp")
	second := filepath.Join(h.home, "install", "b", "omp")
	writeNativeCheckFile(t, first, "a")
	writeNativeCheckFile(t, second, "b")
	h.runner.setPath(first)
	h.runner.queue(ompUpToDate, ompCanaryAvailable)
	started, release := h.runner.gated()
	t.Cleanup(release)
	done := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "native check start", started)
	h.runner.setPath(second)
	release()
	_ = awaitNativeStatus(t, "status after identity change", done)
	h.runner.setPath(first)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("status after identity change = %+v; the result from another executable was cached", status)
	}
	requireRuns(t, h.runner, 2, "after the executable changed during a run")
}

// Removing a configuration file is a change even when it restores an
// identity that was cached earlier (AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2).
func TestNativeCheckReturnToPriorIdentityRechecks(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.queue(ompUpToDate, ompUpToDate, ompCanaryAvailable)
	_ = h.status(t, ompAgentID)
	npmrc := filepath.Join(h.home, ".npmrc")
	writeNativeCheckFile(t, npmrc, "registry=https://registry.invalid/\n")
	_ = h.status(t, ompAgentID)
	if err := os.Remove(npmrc); err != nil {
		t.Fatal(err)
	}
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("status after removing .npmrc = %+v; a superseded identity was reused", status)
	}
	requireRuns(t, h.runner, 3, "after create and remove")
	h.ctrl.runtimeUpdateStatusMu.Lock()
	defer h.ctrl.runtimeUpdateStatusMu.Unlock()
	native := 0
	for key := range h.ctrl.runtimeUpdateStatusCache {
		if strings.HasPrefix(key, nativeUpdateCheckKeyPrefix(ompAgentID)) {
			native++
		}
	}
	if native != 1 {
		t.Errorf("native cache entries = %d, want only the active identity", native)
	}
}

// A request that returns to identity A while the first A check is still in
// flight must start a new check rather than join it, and the stale A result
// must not fill the cache.
func TestNativeCheckReturnToIdentityWhileInFlightStartsNewCheck(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.queue(ompUpToDate, ompUpToDate, ompCanaryAvailable)
	started, release := h.runner.gated()
	t.Cleanup(release)
	awaitStart := func(step string) {
		t.Helper()
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			release()
			t.Fatalf("%s did not start its own check", step)
		}
	}
	npmrc := filepath.Join(h.home, ".npmrc")
	first := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitStart("first A")
	writeNativeCheckFile(t, npmrc, "registry=https://registry.invalid/\n")
	second := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitStart("B")
	if err := os.Remove(npmrc); err != nil {
		t.Fatal(err)
	}
	third := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitStart("returning A")
	release()
	_ = awaitNativeStatus(t, "first A status", first)
	_ = awaitNativeStatus(t, "B status", second)
	_ = awaitNativeStatus(t, "returning A status", third)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("cached status = %+v; want the returning A check's result, not the stale first A", status)
	}
	requireRuns(t, h.runner, 3, "after A, B, and a returning A")
}

// An observation that started before a newer one committed must not replace
// the newer identity or evict its cached result.
func TestNativeCheckStaleObservationDoesNotEvictNewerIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	first := filepath.Join(h.home, "install", "a", "omp")
	second := filepath.Join(h.home, "install", "b", "omp")
	writeNativeCheckFile(t, first, "a")
	writeNativeCheckFile(t, second, "b")
	h.runner.setPath(first)
	h.runner.setOutput(ompUpToDate, 0)
	paused, resume := make(chan struct{}), make(chan struct{})
	var resumeOnce sync.Once
	releaseObservation := func() { resumeOnce.Do(func() { close(resume) }) }
	t.Cleanup(releaseObservation)
	var pausing atomic.Bool
	pausing.Store(true)
	h.runner.setEnvironmentHook(func() {
		if pausing.CompareAndSwap(true, false) {
			close(paused)
			<-resume
		}
	})
	stale := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "first identity observation", paused)
	h.runner.setPath(second)
	_ = h.status(t, ompAgentID)
	requireRuns(t, h.runner, 1, "after the newer observation")
	releaseObservation()
	_ = awaitNativeStatus(t, "stale observation status", stale)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("status = %+v", status)
	}
	requireRuns(t, h.runner, 1, "after a stale observation resumed")
}

// The standalone build service origin (PI_BUILD_URL) selects the release
// source in check mode, so changing it reruns the check.
func TestNativeCheckBuildURLChangeRechecks(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	_ = h.status(t, ompAgentID)
	h.runner.setEnv(append(h.baseEnv(), "PI_BUILD_URL=https://builds.invalid")...)
	_ = h.status(t, ompAgentID)
	requireRuns(t, h.runner, 2, "after PI_BUILD_URL changed")
}

// Status and preview must show the runtime version that the native
// observation used, even when the runtime reported it only after the
// request's earlier read.
func TestNativeCheckProjectsObservedRuntimeVersion(t *testing.T) {
	h := newNativeCheckHarness(t, "", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	h.updater.sequenceACPVersions("", "18.13.3")
	status := h.status(t, ompAgentID)
	if status.CheckState != dto.AgentUpdateCheckStateUpToDate || status.CurrentVersion != "18.13.3" || status.EffectiveVersion != "18.13.3" {
		t.Errorf("status = state %q current %q effective %q; want the observed 18.13.3", status.CheckState, status.CurrentVersion, status.EffectiveVersion)
	}
	h.updater.sequenceACPVersions("", "18.13.3")
	preview, err := h.ctrl.PreviewAgentUpdate(context.Background(), ompAgentID, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.CurrentVersion != "18.13.3" || preview.EffectiveVersion != "18.13.3" || preview.Operation != "update" {
		t.Errorf("preview = current %q effective %q operation %q; want the observed 18.13.3 and update", preview.CurrentVersion, preview.EffectiveVersion, preview.Operation)
	}
}

// A check that actually launched a different executable than the one
// prepared must not fill the prepared identity's cache, even if the PATH is
// restored before the post-run observation.
func TestNativeCheckLaunchedExecutableMustMatchPreparedIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	first := filepath.Join(h.home, "install", "a", "omp")
	second := filepath.Join(h.home, "install", "b", "omp")
	writeNativeCheckFile(t, first, "a")
	writeNativeCheckFile(t, second, "b")
	h.runner.setPath(first)
	h.runner.queue(ompUpToDate, ompCanaryAvailable)
	var switched atomic.Bool
	h.runner.setEnvironmentHook(func() {
		if switched.CompareAndSwap(false, true) {
			h.runner.setPath(second)
		}
	})
	started, release := h.runner.gated()
	t.Cleanup(release)
	done := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "native check start", started)
	h.runner.setPath(first)
	release()
	_ = awaitNativeStatus(t, "status after executable replacement", done)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("status at A = %+v; A reused the result launched from B", status)
	}
	requireRuns(t, h.runner, 2, "after a run launched a different executable")
}

// A symlinked launcher briefly retargeted while the check launches, then
// restored before publication, must not cache the other target's result.
func TestNativeCheckLaunchTimeRealExecutableMustMatch(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	first := filepath.Join(h.home, "install", "a", "omp")
	second := filepath.Join(h.home, "install", "b", "omp")
	writeNativeCheckFile(t, first, "a")
	writeNativeCheckFile(t, second, "b")
	link := filepath.Join(h.home, "bin", "omp")
	mustMkdir(t, filepath.Dir(link))
	retarget := func(target string) {
		t.Helper()
		_ = os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	retarget(first)
	h.runner.setPath(link)
	h.runner.queue(ompUpToDate, ompCanaryAvailable)
	var once atomic.Bool
	h.runner.setRunHook(func() {
		if once.CompareAndSwap(false, true) {
			retarget(second)
		}
	})
	started, release := h.runner.gated()
	t.Cleanup(release)
	done := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "native check start", started)
	retarget(first)
	release()
	_ = awaitNativeStatus(t, "status after symlink retarget", done)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("status = %+v; the result from the temporary symlink target was cached", status)
	}
	requireRuns(t, h.runner, 2, "after a launch-time symlink retarget")
}

// OMP's own updater replaces a standalone executable at the same path. That
// replacement changes the resolved executable and must trigger a new check.
func TestNativeCheckSamePathExecutableReplacementRechecks(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	binary := filepath.Join(h.home, "install", "bin", "omp")
	writeNativeCheckFile(t, binary, "old binary")
	h.runner.setPath(binary)
	h.runner.queue(ompCanaryAvailable, ompUpToDate)
	_ = h.status(t, ompAgentID)
	replacement := binary + ".new"
	writeNativeCheckFile(t, replacement, "a different, longer binary")
	if err := os.Rename(replacement, binary); err != nil {
		t.Fatal(err)
	}
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("status after replacing the executable = %+v; the old result was reused", status)
	}
	requireRuns(t, h.runner, 2, "after a same-path executable replacement")
}

// OMP resolves overlays against process.cwd(), the physical directory. When
// the backend runs from a symlinked directory, a "../" overlay must be
// fingerprinted where OMP actually reads it.
func TestNativeCheckOverlayResolvesAgainstPhysicalWorkingDir(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	real := filepath.Join(h.home, "real", "project")
	mustMkdir(t, real)
	logical := filepath.Join(h.home, "logical")
	mustMkdir(t, logical)
	if err := os.Symlink(real, filepath.Join(logical, "project")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(logical, "project"))
	overlay := filepath.Join(h.home, "real", "channel.yml")
	writeNativeCheckFile(t, overlay, "update:\n  channel: stable\n")
	h.runner.setEnv(append(h.baseEnv(), "PI_CONFIG_FILES=../channel.yml")...)
	h.runner.setOutput(ompUpToDate, 0)
	_ = h.status(t, ompAgentID)
	writeNativeCheckFile(t, overlay, "update:\n  channel: canary-longer\n")
	_ = h.status(t, ompAgentID)
	requireRuns(t, h.runner, 2, "after editing the overlay OMP reads through the physical directory")
}

// A fixture backend with canned release lookups must never run a host agent
// CLI for a native check, even when one is on PATH.
func TestNativeCheckDisabledExecutionNeverRunsHostBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script helper")
	}
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	marker := filepath.Join(h.home, "ran")
	binary := filepath.Join(h.home, "bin", "omp")
	writeNativeCheckFile(t, binary, "#!/bin/sh\n: > "+marker+"\n")
	if err := os.Chmod(binary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(binary))
	// Positive control: the process runner does run this helper.
	h.ctrl.SetNativeUpdateCheckRunner(nil)
	_ = h.status(t, ompAgentID)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("positive control: the helper did not run: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	h.ctrl.DisableNativeUpdateCheckExecution()
	status := h.status(t, ompAgentID)
	if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonNotInstalled) {
		t.Errorf("status = %+v, want unknown/not_installed", status)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the host omp binary ran")
	}
}

// Process shutdown cancels the parent context before HTTP drain; that alone
// must cancel a running native check so a waiting request returns
// unknown/cancelled without waiting for the later stop callback.
func TestNativeCheckParentCancellationCancelsRunningCheck(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	started, release := h.runner.gated()
	defer release()
	parent, cancelParent := context.WithCancel(context.Background())
	stop := h.ctrl.StartRuntimeUpdateBackground(parent, make(chan struct{}))
	defer stop()
	result := make(chan dto.AgentUpdateStatusDTO, 1)
	go func() {
		statuses, err := h.ctrl.ListAgentUpdateStatuses(context.Background())
		if err == nil && len(statuses.Statuses) == 1 {
			result <- statuses.Statuses[0]
			return
		}
		result <- dto.AgentUpdateStatusDTO{}
	}()
	awaitNativeSignal(t, "native check start", started)
	// A post-run fingerprint can block on unreachable storage. A cancelled
	// run must publish and release without one.
	fingerprintGate := make(chan struct{})
	defer close(fingerprintGate)
	h.runner.setEnvironmentHook(func() { <-fingerprintGate })
	cancelParent()
	select {
	case status := <-result:
		if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonCancelled) {
			t.Errorf("status after parent cancellation = %+v, want unknown/cancelled", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parent cancellation did not cancel the running native check")
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not join the cancelled check")
	}
}

// Identity observation reads files that can block on unreachable storage.
// Process shutdown must not wait for an observation before or after the run:
// the waiting request settles and stop joins while the observation is stuck.
func TestNativeCheckShutdownDoesNotWaitForBlockedObservation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		afterRun  bool
		shutInRun bool
		wantState dto.AgentUpdateCheckState
		wantRuns  int
	}{
		{name: "before the run", wantState: dto.AgentUpdateCheckStateUnknown, wantRuns: 0},
		{name: "after the run", afterRun: true, wantState: dto.AgentUpdateCheckStateUpToDate, wantRuns: 1},
		// Shutdown closes the lifetime after the CLI finished but before the
		// revalidation starts; the revalidation must not run inline.
		{name: "closed before revalidation", shutInRun: true, wantState: dto.AgentUpdateCheckStateUpToDate, wantRuns: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
			h.runner.setOutput(ompUpToDate, 0)
			gate, entered := make(chan struct{}), make(chan struct{}, 1)
			defer close(gate)
			block := func() {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-gate
			}
			parent, cancelParent := context.WithCancel(context.Background())
			stop := h.ctrl.StartRuntimeUpdateBackground(parent, make(chan struct{}))
			switch {
			case tc.shutInRun:
				h.runner.setRunHook(func() {
					h.ctrl.currentNativeCheckLifetime().shut()
					h.runner.setEnvironmentHook(block)
				})
			case tc.afterRun:
				h.runner.setRunHook(func() { h.runner.setEnvironmentHook(block) })
			default:
				h.runner.setEnvironmentHook(block)
			}
			result := make(chan dto.AgentUpdateStatusDTO, 1)
			go func() {
				statuses, err := h.ctrl.ListAgentUpdateStatuses(context.Background())
				if err == nil && len(statuses.Statuses) == 1 {
					result <- statuses.Statuses[0]
					return
				}
				result <- dto.AgentUpdateStatusDTO{}
			}()
			defer cancelParent()
			if !tc.shutInRun {
				awaitNativeSignal(t, "blocked identity observation", entered)
				cancelParent()
			}
			select {
			case status := <-result:
				if status.CheckState != tc.wantState {
					t.Errorf("status = %+v, want %s", status, tc.wantState)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the request waited for a blocked observation after shutdown")
			}
			stopped := make(chan struct{})
			go func() { stop(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("stop waited for a blocked observation")
			}
			requireRuns(t, h.runner, tc.wantRuns, "with a blocked observation")
		})
	}
}

// A pass still building targets when shutdown closes the lifetime is joined
// by stop, so it must not begin an identity observation at all.
func TestNativeCheckClosedLifetimeSkipsObservation(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.ctrl.currentNativeCheckLifetime().shut()
	h.runner.setEnvironmentHook(func() { t.Error("observed identity after the lifetime closed") })
	status := h.status(t, ompAgentID)
	if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonCancelled) {
		t.Errorf("status = %+v, want unknown/cancelled", status)
	}
	requireRuns(t, h.runner, 0, "after the lifetime closed")
}
