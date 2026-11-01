package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// brokenHandoff is a handoff the sweep can never settle: its inventory names no
// environment generation, so every pass fails it before any provider call.
func brokenHandoff(index int) *models.UnrecordedPluginExecutor {
	id := fmt.Sprintf("execution-broken-%02d", index)
	return &models.UnrecordedPluginExecutor{
		ExecutionID: id, SessionID: "session-" + id, TaskID: "task-" + id, EnvironmentID: "environment-" + id,
		Running: &models.ExecutorRunning{Metadata: map[string]interface{}{MetadataKeyPluginExecutor: pluginExecutorInventory{EnvironmentID: "environment-" + id}}},
	}
}

func sweepStarvationRuntime(t *testing.T, handoffs int) (*PluginRemoteExecutor, *pluginExecutorOperationsFake, *pluginExecutorInventoryStoreFake) {
	t.Helper()
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, pluginExecutorPhaseCleanupPending, workspaceResource("", "")),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	for i := 0; i < handoffs; i++ {
		store.unrecorded = append(store.unrecorded, brokenHandoff(i))
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(progressResumeLoader(), store)
	return runtime, operations, store
}

// Handoffs that fail every pass must not spend the whole batch ahead of a
// cleanup_pending row that could be settled.
func TestSweepDoesNotLetFailingHandoffsStarveACleanupPendingRow(t *testing.T) {
	runtime, operations, _ := sweepStarvationRuntime(t, 3*pluginExecutorCleanupSweepBatch)

	if err := runtime.SweepPendingCleanup(context.Background()); err == nil {
		t.Fatal("the sweep reported success while handoffs are failing")
	}
	if operations.destroyRequest == nil {
		t.Fatal("a failing handoff backlog starved the cleanup_pending row")
	}
}

// Successive passes rotate through the handoffs, so one that sorts late is
// reached instead of the first batch being retried for ever.
func TestSweepRotatesThroughFailingHandoffs(t *testing.T) {
	runtime, _, store := sweepStarvationRuntime(t, 3*pluginExecutorCleanupSweepBatch)
	last := store.unrecorded[len(store.unrecorded)-1].ExecutionID
	attempted := false

	for pass := 0; pass < 6 && !attempted; pass++ {
		err := runtime.SweepPendingCleanup(context.Background())
		attempted = err != nil && strings.Contains(err.Error(), last)
	}
	if !attempted {
		t.Fatal("the sweep never reached the last handoff behind a failing backlog")
	}
}
