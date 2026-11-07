package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kdlbs/kandev/pluginsdk"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.4
func TestHousekeepingPreparationGuardsRemoteWithoutContact(t *testing.T) {
	m := newTestManager(t)
	row := &models.ExecutorRunning{SessionID: "remote", AgentExecutionID: "remote-owner", Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning}
	m.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})
	if err := m.PrepareStartupRecovery(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !m.HasPendingRemoteRecovery(row.SessionID, row.AgentExecutionID) {
		t.Fatal("preparation did not retain the exact remote owner")
	}
	if err := m.recoveryGuard.CheckLaunchAllowed(row.SessionID); !errors.Is(err, ErrSessionRecoveryGuarded) {
		t.Fatalf("unresolved owner allowed replacement: %v", err)
	}
	if m.WasSessionRetracked(row.SessionID) {
		t.Fatal("preparation claimed successful attachment")
	}
}

type housekeepingRemoteBackend struct {
	MockExecutor
	contacts  int
	recovered []*models.ExecutorRunning
}

func (b *housekeepingRemoteBackend) HealthCheck(context.Context) error { b.contacts++; return nil }
func (b *housekeepingRemoteBackend) RecoverInstances(_ context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	b.contacts++
	b.recovered = append(b.recovered, records...)
	return nil, nil
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.4
func TestHousekeepingRemoteContactWaitsForActivation(t *testing.T) {
	b := &housekeepingRemoteBackend{MockExecutor: MockExecutor{name: executor.NameSSH}}
	m, _ := task08ManagerWithBackend(t, b)
	r := startup.New(nil)
	ctx := startup.WithReporter(t.Context(), r)
	if err := m.StartDeferredRemoteRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	if b.contacts != 0 {
		t.Fatalf("remote contacted during housekeeping: %d", b.contacts)
	}
	direct, _ := task08ManagerWithBackend(t, b)
	if err := direct.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if b.contacts == 0 {
		t.Fatal("positive control never contacted backend")
	}
}

func TestHousekeepingStoppedPluginHandleIsNotPendingAgentRecovery(t *testing.T) {
	row := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, nil)
	row.Status = models.ExecutorRunningStatusStopped
	m, _ := task08ManagerWithBackend(t, &MockExecutor{name: executor.NamePluginRemote})
	m.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})
	r := startup.New(nil)
	ctx := startup.WithReporter(t.Context(), r)
	if err := m.StartDeferredRemoteRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.recoveryGuard.CheckLaunchAllowed(row.SessionID); err != nil {
		t.Fatalf("stopped environment became an unresolved live agent: %v", err)
	}
	if entry := m.pendingRemoteRecoveryForSession(row.SessionID); entry != nil {
		t.Fatal("retained stopped plugin environment entered agent attachment retry")
	}
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.4
func TestHousekeepingPluginInventoryRepairsOnlyUnfinishedOperations(t *testing.T) {
	b := &housekeepingRemoteBackend{MockExecutor: MockExecutor{name: executor.NamePluginRemote}}
	m, _ := task08ManagerWithBackend(t, b)
	rows := []*models.ExecutorRunning{{SessionID: "malformed", Runtime: agentruntime.RuntimePluginRemote, Status: models.ExecutorRunningStatusStopped}}
	for _, phase := range []string{pluginExecutorPhaseReady, pluginExecutorPhaseAllocating, pluginExecutorPhaseCleanupPending} {
		row := pluginExecutorRecoveryRecord(t, phase, &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource"})
		row.SessionID = phase
		row.Status = models.ExecutorRunningStatusStopped
		rows = append(rows, row)
	}
	live := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, nil)
	live.SessionID = "live"
	rows = append(rows, live)
	m.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: rows})
	repair, ok := any(m).(interface{ ReconcileStartupPluginInventory(context.Context) error })
	if !ok {
		t.Fatal("plugin operation repair is not available after plugin boot")
	}
	if err := repair.ReconcileStartupPluginInventory(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(b.recovered) != 2 {
		t.Fatalf("recovered %d rows, want only two unfinished terminal operations", len(b.recovered))
	}
	for _, row := range b.recovered {
		if row.SessionID == pluginExecutorPhaseReady || row.SessionID == "live" {
			t.Fatalf("repaired stable or live inventory: %s", row.SessionID)
		}
	}
}

type unreadableHousekeepingRemoteInventory struct {
	task08ExecutorInventoryWriter
	err error
}

func (w *unreadableHousekeepingRemoteInventory) ListExecutorsRunning(context.Context) ([]*models.ExecutorRunning, error) {
	return nil, w.err
}

// @covers AC-PLATFORM-STARTUP-HOUSEKEEPING-001.3
func TestHousekeepingRequiresReadablePersistedOwnershipInventory(t *testing.T) {
	m := newTestManager(t)
	want := errors.New("ownership inventory is unreadable")
	m.SetExecutorRunningWriter(&unreadableHousekeepingRemoteInventory{err: want})
	if err := m.PrepareStartupRecovery(t.Context()); !errors.Is(err, want) {
		t.Fatalf("missing local ownership evidence admitted startup: %v", err)
	}
}
