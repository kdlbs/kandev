package process

import (
	"context"
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
		// Print, then hold the process open briefly so Start() reliably
		// completes before the agent exits on its own.
		AgentEnv: fixtureEnvSlice("echo-then-sleep started 1"),
		WorkDir:  t.TempDir(),
		Protocol: agent.ProtocolACP,
	}, newTestLogger(t))

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	waitForManagerStatus(t, mgr, StatusStopped)

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
		AgentEnv:  fixtureEnvSlice("echo-then-sleep started 1"),
		WorkDir:   t.TempDir(),
		Protocol:  agent.ProtocolACP,
	}, newTestLogger(t))

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitForManagerStatus(t, mgr, StatusStopped)

	for i := range 3 {
		if err := mgr.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() call %d error = %v", i+1, err)
		}
	}
}

func waitForManagerStatus(t *testing.T, mgr *Manager, want Status) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for mgr.Status() != want {
		if time.Now().After(deadline) {
			t.Fatalf("manager status = %s, want %s", mgr.Status(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
