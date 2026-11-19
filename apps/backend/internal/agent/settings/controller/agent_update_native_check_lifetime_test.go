package controller

import (
	"context"
	"sync/atomic"
	"testing"
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

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
func TestNativeCheckLifetimeAdmissionRacingStop(t *testing.T) {
	lifetime := newNativeCheckLifetime()
	if !lifetime.admit() {
		t.Fatal("open lifetime refused admission")
	}
	closed := make(chan struct{})
	go func() {
		lifetime.close()
		close(closed)
	}()
	<-lifetime.ctx.Done()
	if lifetime.admit() {
		t.Fatal("closing lifetime admitted a new execution")
	}
	select {
	case <-closed:
		t.Fatal("stop returned while an admitted execution was still running")
	default:
	}
	lifetime.done()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not return after the admitted execution ended")
	}
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
	defer func() {
		for range cap(slots) {
			<-slots
		}
	}()
	waiter := make(chan dto.AgentUpdateStatusDTO, 1)
	go func() { waiter <- listStatus(t, h.ctrl, ompAgentID) }()
	lifetime := h.ctrl.currentNativeCheckLifetime()
	waitFor(t, "the request to wait for a slot", func() bool { return lifetime.inFlight() == 1 })
	stop()
	select {
	case status := <-waiter:
		if status.CheckReason != dto.AgentUpdateCheckReason(agents.NativeUpdateCheckReasonCancelled) {
			t.Errorf("slot waiter status = %+v, want cancelled", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stop left a slot waiter blocked")
	}
	requireRuns(t, h.runner, 0, "after releasing slot waiters")
}

// @covers AC-AGENTS-RUNTIME-NATIVE-CHECK-001.3
func TestNativeCheckBackgroundStopCancelsProcess(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	stop := startIdleRuntimeBackground(t, h.ctrl)
	started, release := h.runner.gated()
	subscriber := make(chan dto.AgentUpdateStatusDTO, 1)
	go func() { subscriber <- listStatus(t, h.ctrl, ompAgentID) }()
	<-started
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case status := <-subscriber:
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
	paused, resume := make(chan struct{}), make(chan struct{})
	var pausing atomic.Bool
	pausing.Store(true)
	h.ctrl.nativeCheckBeforePublish = func() {
		if pausing.CompareAndSwap(true, false) {
			close(paused)
			<-resume
		}
	}
	defer close(resume)
	go func() { _ = listStatus(t, h.ctrl, ompAgentID) }()
	<-started
	stop()
	<-paused
	release()
	startIdleRuntimeBackground(t, h.ctrl)
	fresh := make(chan dto.AgentUpdateStatusDTO, 1)
	go func() { fresh <- listStatus(t, h.ctrl, ompAgentID) }()
	select {
	case status := <-fresh:
		if status.CheckState != dto.AgentUpdateCheckStateUpToDate {
			t.Errorf("status after restart = %+v, want a fresh check", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request after restart joined the stopped lifetime's unpublished flight")
	}
	requireRuns(t, h.runner, 2, "after restart with an unpublished stopped flight")
}
