package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// A terminal row left in cleanup_progress owes its destroy exactly like a
// cleanup_pending one, so the startup repair hands it to recovery.
func TestHousekeepingPluginInventoryRepairsTerminalCleanupProgress(t *testing.T) {
	b := &housekeepingRemoteBackend{MockExecutor: MockExecutor{name: executor.NamePluginRemote}}
	m, _ := task08ManagerWithBackend(t, b)
	row := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseCleanupProgress, &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource"})
	row.Status = models.ExecutorRunningStatusStopped
	m.SetExecutorRunningWriter(&task08ExecutorInventoryWriter{rows: []*models.ExecutorRunning{row}})

	if err := m.ReconcileStartupPluginInventory(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(b.recovered) != 1 || b.recovered[0].SessionID != row.SessionID {
		t.Fatalf("recovered %d rows, want the cleanup_progress row", len(b.recovered))
	}
}
