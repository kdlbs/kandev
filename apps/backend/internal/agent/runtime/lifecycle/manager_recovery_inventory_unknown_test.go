package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
)

// TestManagerStartUnreadableRecoveryInventoryStopsNothing pins that a failed
// read of the live standalone recovery-inventory records is treated as an
// UNKNOWN record set, never an empty one.
//
// AC-EXECUTORS-SURVIVAL-002.6 stops a live instance because it is known to
// have no record. Correlation cannot tell an empty inventory from an unread
// one, so handing it the empty slice a failed read returns makes every live
// instance look record-less and sends all of them down that orphan-stop path:
// one transient database error during startup would stop every agent that had
// just survived the restart. The outcome required here is the one
// AC-EXECUTORS-SURVIVAL-002.12 sets for the mirror failure -- recover
// nothing, stop nothing, leave every record to the existing repair path.
func TestManagerStartUnreadableRecoveryInventoryStopsNothing(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	// Two live instances that WOULD each be stopped as a record-less orphan
	// if an unreadable inventory were passed on as an empty one.
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", Port: 41001, SessionID: "session-1", LeaseGeneration: 0, ListenerActive: false},
		{ID: "instance-2", Port: 41001, SessionID: "session-2", LeaseGeneration: 9, ListenerActive: false},
	}

	mgr := newLivenessTestManager(t, control)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{err: errors.New("database unavailable")})
	mgr.SetRecoveryDeadline(5 * time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v, want an unreadable inventory to be survivable at startup", err)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 0 {
		t.Fatalf("deleted = %v, want none: an unreadable recovery inventory must not make live instances look record-less", control.deleted)
	}
}

func TestRecoveryLeaseDiagnosticsAreNonAuthoritative(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "live-instance", Port: 41001, SessionID: "live-session", Status: "running", LeaseGeneration: 0, ListenerActive: false},
		{ID: "waiting-instance", Port: 41001, SessionID: "waiting-session", Status: "running", LeaseGeneration: 12, ListenerActive: false},
		{ID: "stale-instance", Port: 41001, SessionID: "stale-session", Status: "running", LeaseGeneration: 13, ListenerActive: true},
	}
	exec := control.executor(t)
	records := []*models.ExecutorRunning{
		{SessionID: "live-session", AgentExecutionID: "live-instance", Status: "RUNNING"},
		{SessionID: "waiting-session", AgentExecutionID: "waiting-instance", Status: "WAITING_FOR_INPUT"},
		{SessionID: "terminal-session", AgentExecutionID: "terminal-instance", Status: "COMPLETED"},
	}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	recoveredIDs := make(map[string]struct{}, len(recovered))
	for _, inst := range recovered {
		recoveredIDs[inst.InstanceID] = struct{}{}
	}
	for _, protected := range []string{"live-instance", "waiting-instance"} {
		if _, ok := recoveredIDs[protected]; !ok {
			t.Errorf("protected instance %q was not recovered; diagnostics and duplicate ports are not cleanup authority", protected)
		}
	}

	control.mu.Lock()
	deleted := append([]string(nil), control.deleted...)
	control.mu.Unlock()
	if len(deleted) != 1 || deleted[0] != "stale-instance" {
		t.Fatalf("deleted instances = %v, want only the stale instance sharing the numeric port", deleted)
	}

	probe := newStandaloneControlServer(t, true)
	probe.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "live-instance", Port: 41001, SessionID: "live-session", LeaseGeneration: 0, ListenerActive: false},
		{ID: "waiting-instance", Port: 41001, SessionID: "waiting-session", LeaseGeneration: 0, ListenerActive: false},
	}
	probe.listInstancesErr = true
	probeExec := probe.executor(t)
	if recovered, err := probeExec.RecoverInstances(context.Background(), records[:2]); err != nil || len(recovered) != 0 {
		t.Fatalf("RecoverInstances after failed instance probe = %v, %v; want no result and no error", recovered, err)
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if len(probe.deleted) != 0 {
		t.Fatalf("failed instance probe deleted %v, want no cleanup", probe.deleted)
	}
}

// TestManagerStartUnreadableRecoveryInventoryRetracksNothing pins the other
// half of the same outcome: nothing is re-tracked either. Re-tracking off an
// unknown inventory would publish sessions whose identity could not be
// correlated to a record.
func TestManagerStartUnreadableRecoveryInventoryRetracksNothing(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", SessionID: "session-1"},
	}

	mgr := newLivenessTestManager(t, control)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{err: errors.New("database unavailable")})
	mgr.SetRecoveryDeadline(5 * time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if _, ok := mgr.GetExecutionBySessionID("session-1"); ok {
		t.Fatal("session-1 was re-tracked from an unreadable recovery inventory; nothing may be correlated to an unknown record set")
	}
}

// TestManagerStartEmptyRecoveryInventoryStillStopsOrphans pins the boundary
// the fix above must not blur: a SUCCESSFUL read that returns no records is
// authoritative, and a live instance really is a record-less orphan then, so
// AC-EXECUTORS-SURVIVAL-002.6 still stops it. Without this, treating an
// unreadable inventory as unknown could be over-applied to an empty one and
// leave genuine orphans running.
func TestManagerStartEmptyRecoveryInventoryStillStopsOrphans(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", SessionID: "session-1"},
	}

	mgr := newLivenessTestManager(t, control)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)
	mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{}})
	mgr.SetRecoveryDeadline(5 * time.Second)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 1 || control.deleted[0] != "instance-1" {
		t.Fatalf("deleted = %v, want [instance-1]: an authoritative empty inventory still makes a live instance an orphan", control.deleted)
	}
}
