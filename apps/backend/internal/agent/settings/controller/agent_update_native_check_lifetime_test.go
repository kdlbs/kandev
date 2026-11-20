package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// startIdleRuntimeBackground starts the background owner without running a
// pass, so only the test's own requests execute native checks.
func startIdleRuntimeBackground(t *testing.T, ctrl *Controller) func() {
	t.Helper()
	never := make(chan struct{})
	stop := ctrl.StartRuntimeUpdateBackground(context.Background(), never)
	t.Cleanup(stop)
	return stop
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func stopWithin(t *testing.T, stop func(), releaseBlockers ...func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
	}
	for _, release := range releaseBlockers {
		release()
	}
	select {
	case <-done:
		t.Fatal("stop exceeded its deadline and needed test cleanup to finish")
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not finish after test-owned blockers were released")
	}
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
func TestNativeCheckLifetimeAdmissionRacingStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lifetime := newNativeCheckLifetime()
		if !lifetime.admit() {
			t.Fatal("open lifetime refused admission")
		}
		closed := make(chan struct{})
		go func() {
			lifetime.close()
			close(closed)
		}()
		synctest.Wait()
		if lifetime.ctx.Err() == nil {
			t.Fatal("close did not cancel the lifetime")
		}
		if lifetime.admit() {
			t.Fatal("closing lifetime admitted a new execution")
		}
		select {
		case <-closed:
			t.Fatal("stop returned while an admitted execution was still running")
		default:
		}
		lifetime.done()
		synctest.Wait()
		select {
		case <-closed:
		default:
			t.Fatal("stop did not return after the admitted execution ended")
		}
	})
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
func TestNativeCheckBackgroundStopReleasesSlotWaiters(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	stop := startIdleRuntimeBackground(t, h.ctrl)
	slots := h.ctrl.runtimeUpdateStatusSemaphore()
	for range cap(slots) {
		slots <- struct{}{}
	}
	var releaseSlotsOnce sync.Once
	releaseSlots := func() {
		releaseSlotsOnce.Do(func() {
			for range cap(slots) {
				<-slots
			}
		})
	}
	defer releaseSlots()
	waiter := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	lifetime := h.ctrl.currentNativeCheckLifetime()
	waitFor(t, "the request to wait for a slot", func() bool { return lifetime.inFlight() == 1 })
	stopWithin(t, stop, releaseSlots)
	if status := awaitNativeStatus(t, "slot waiter status", waiter); status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonCancelled) {
		t.Errorf("slot waiter status = %+v, want cancelled", status)
	}
	requireRuns(t, h.runner, 0, "after releasing slot waiters")
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
func TestNativeCheckBackgroundStopCancelsProcess(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	stop := startIdleRuntimeBackground(t, h.ctrl)
	started, release := h.runner.gated()
	t.Cleanup(release)
	subscriber := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "native check start", started)
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case fetched := <-subscriber:
		if fetched.err != nil {
			t.Fatalf("live subscriber failed: %v", fetched.err)
		}
		status := fetched.status
		if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonCancelled) {
			t.Errorf("live subscriber status = %+v, want cancelled", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not cancel the running check")
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return after the check ended")
	}
	if status := h.status(t, ompAgentID); status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonCancelled) {
		t.Errorf("status after stop = %+v, want cancelled without a new process", status)
	}
	requireRuns(t, h.runner, 1, "after stop")

	release()
	startIdleRuntimeBackground(t, h.ctrl)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("status after restart = %+v, want a fresh check", status)
	}
	requireRuns(t, h.runner, 2, "after restart")
}

// After a stop, the cancelled flight can still be unpublished when the
// restarted owner receives a request; that request must start its own check
// rather than join the stopped lifetime's flight.
//
// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
func TestNativeCheckRestartDoesNotJoinStoppedLifetimeFlight(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	stop := startIdleRuntimeBackground(t, h.ctrl)
	started, release := h.runner.gated()
	t.Cleanup(release)
	paused, resume := make(chan struct{}), make(chan struct{})
	var resumeOnce sync.Once
	releasePublication := func() { resumeOnce.Do(func() { close(resume) }) }
	t.Cleanup(releasePublication)
	var pausing atomic.Bool
	pausing.Store(true)
	h.ctrl.nativeCheckBeforePublish = func() {
		if pausing.CompareAndSwap(true, false) {
			close(paused)
			<-resume
		}
	}
	stale := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	awaitNativeSignal(t, "native check start", started)
	stopWithin(t, stop, release, releasePublication)
	awaitNativeSignal(t, "pre-publication hook", paused)
	release()
	startIdleRuntimeBackground(t, h.ctrl)
	fresh := fetchNativeStatusAsync(h.ctrl, ompAgentID)
	select {
	case fetched := <-fresh:
		if fetched.err != nil {
			t.Fatalf("status after restart failed: %v", fetched.err)
		}
		status := fetched.status
		if status.CheckState != dto.AgentUpdateCheckStateUpToDate {
			t.Errorf("status after restart = %+v, want a fresh check", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request after restart joined the stopped lifetime's unpublished flight")
	}
	releasePublication()
	_ = awaitNativeStatus(t, "previous lifetime subscriber", stale)
	requireRuns(t, h.runner, 2, "after restart with an unpublished stopped flight")
}
