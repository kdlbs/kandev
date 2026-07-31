package process

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

// An agent process can reach StatusStopped without an explicit Stop: waitForExit
// stores it when the child exits on its own (crash, self-exit, OOM). Stop must
// still tear the adapter down in that case. Before this was fixed the status
// guard in stop() returned early, so adapter.Close() and close(stopCh) never
// ran and the adapter's update worker plus forwardUpdates stayed alive for the
// lifetime of the manager — a real leak in production, and an intermittent
// goleak failure for this package in CI.
func TestManager_StopAfterAgentSelfExitTearsDownAdapter(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		WorkDir:   t.TempDir(),
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	killAgentProcess(t, mgr)

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// forwardUpdates must have returned: Stop closed stopCh and drained the
	// manager's WaitGroup.
	drained := make(chan struct{})
	go func() {
		mgr.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() returned with manager goroutines still running")
	}

	// The adapter must have been closed: Close() waits for its update worker
	// to exit and then closes the updates channel.
	select {
	case _, ok := <-mgr.adapter.Updates():
		if ok {
			t.Fatal("adapter updates channel delivered an event instead of being closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() left the adapter open after the agent process self-exited")
	}
}

// TestManager_StopIsIdempotentAfterSelfExit pins that the extra teardown added
// for the self-exit path does not break a second Stop call.
func TestManager_StopIsIdempotentAfterSelfExit(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		WorkDir:   t.TempDir(),
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	killAgentProcess(t, mgr)

	for i := range 3 {
		if err := mgr.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() call %d error = %v", i+1, err)
		}
	}
}

// Start accepts StatusStopped, so a restart after the agent self-exited can run
// without an intervening Stop. The previous lifecycle never tore itself down, so
// Start must finish that teardown before replacing stopCh, doneCh and the
// adapter — otherwise the old forwardUpdates waits forever on a stopCh nobody
// closes, and the old adapter's update worker is stranded with it.
func TestManager_RestartAfterSelfExitDrainsPreviousLifecycle(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		WorkDir:   t.TempDir(),
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	killAgentProcess(t, mgr)

	firstAdapter := mgr.adapter
	firstStopCh := mgr.stopCh

	// Restart without an intervening Stop.
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	if mgr.stopCh == firstStopCh {
		t.Fatal("restart reused the previous stop channel")
	}

	// The first lifecycle's adapter must have been closed by the restart.
	select {
	case _, ok := <-firstAdapter.Updates():
		if ok {
			t.Fatal("previous adapter delivered an event instead of being closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restart left the previous adapter open")
	}
}

// A Start that fails after its adapter was created must still leave that adapter
// closable: the stopCh guard is scoped to the channel, not to teardown as a
// whole, so it must not swallow the adapter close for the failed attempt.
func TestManager_StopClosesAdapterFromFailedRestart(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(),
		AgentEnv:  fixtureEnvSlice("sleep 60"),
		WorkDir:   t.TempDir(),
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	killAgentProcess(t, mgr)
	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Restart with a command that cannot be spawned: Start fails after
	// buildAdapterConfig has already created a second adapter.
	mgr.cfg.AgentArgs = []string{filepath.Join(t.TempDir(), "does-not-exist")}
	if err := mgr.Start(context.Background()); err == nil {
		t.Fatal("Start() with a missing binary unexpectedly succeeded")
	}
	failedAdapter := mgr.adapter

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after failed restart error = %v", err)
	}

	select {
	case _, ok := <-failedAdapter.Updates():
		if ok {
			t.Fatal("adapter from failed start delivered an event instead of being closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() left the failed start's adapter open")
	}
}

// killAgentProcess terminates the agent's child process directly and waits for
// the manager to observe it. That models the real trigger for this bug — an
// agent that dies without an explicit Stop (crash, self-exit, OOM) — and keeps
// the test independent of how a spawned binary receives its arguments and
// environment, which differs per platform: Windows starts agents suspended
// under a Job Object, so a short-lived fixture is not a portable exit signal.
func killAgentProcess(t *testing.T, mgr *Manager) {
	t.Helper()
	if mgr.cmd == nil || mgr.cmd.Process == nil {
		t.Fatal("manager has no agent process to kill")
	}
	if err := mgr.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill agent process: %v", err)
	}
	waitForManagerStatus(t, mgr, StatusStopped)
}

func waitForManagerStatus(t *testing.T, mgr *Manager, want Status) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for mgr.Status() != want {
		if time.Now().After(deadline) {
			t.Fatalf("manager status = %s, want %s", mgr.Status(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
