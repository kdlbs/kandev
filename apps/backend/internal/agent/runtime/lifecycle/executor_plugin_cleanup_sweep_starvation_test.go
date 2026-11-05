package lifecycle

import (
	"context"
	"errors"
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

// A handoff and a cleanup_pending row can name the same execution. The sweep
// lists the rows before it settles the handoff, so it must not destroy the
// resource a second time from that stale listing.
func TestSweepDestroysAHandoffAndItsCleanupPendingRowOnce(t *testing.T) {
	runtime, operations, request, handoff, ctx := sweepRuntime(t, true)
	destroys := 0
	operations.beforeDestroy = func() { destroys++ }
	environment := &models.TaskEnvironment{ID: handoff.EnvironmentID, TaskID: handoff.TaskID, OwnershipGeneration: 7}
	if err := runtime.adoptUnrecordedPluginExecutor(ctx, environment, handoff); err != nil {
		t.Fatalf("seed the inventory row: %v", err)
	}
	stored, err := runtime.currentPluginExecutorRecord(ctx, request.SessionID, request.InstanceID)
	if err != nil {
		t.Fatalf("read the inventory row: %v", err)
	}
	inventory, err := decodePluginExecutorInventory(stored.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.Phase = pluginExecutorPhaseCleanupPending
	if err := runtime.checkpointPluginExecutorRecord(ctx, stored, inventory); err != nil {
		t.Fatalf("seed the cleanup_pending row: %v", err)
	}

	if err := runtime.SweepPendingCleanup(ctx); err != nil {
		t.Fatalf("SweepPendingCleanup() = %v, want no error", err)
	}
	if destroys != 1 {
		t.Fatalf("destroys = %d, want exactly one", destroys)
	}
}

// Rows that settle or vanish between passes must not make the rotation skip the
// ones still failing.
func TestSweepRotationSurvivesRowsDisappearingBetweenPasses(t *testing.T) {
	runtime, _, store := sweepStarvationRuntime(t, 3*pluginExecutorCleanupSweepBatch)
	seen := map[string]bool{}
	for pass := 0; pass < 8; pass++ {
		err := runtime.SweepPendingCleanup(context.Background())
		for _, item := range store.unrecorded {
			seen[item.ExecutionID] = seen[item.ExecutionID] || err != nil && strings.Contains(err.Error(), item.ExecutionID)
		}
		if pass == 0 && len(store.unrecorded) > 4 {
			// The rows the first pass just visited disappear; the list shrinks and shifts.
			store.unrecorded = store.unrecorded[pluginExecutorCleanupSweepBatch/2:]
		}
	}
	for _, item := range store.unrecorded {
		if !seen[item.ExecutionID] {
			t.Fatalf("handoff %s was never visited after rows disappeared", item.ExecutionID)
		}
	}
}

// A handoff whose destroy is not confirmed is adopted into a cleanup_pending
// row. That row must not be destroyed again in the same pass.
func TestSweepRetriesAFailingHandoffOnceAPass(t *testing.T) {
	runtime, operations, request, handoff, ctx := sweepRuntime(t, false)
	destroys := 0
	operations.beforeDestroy = func() { destroys++ }
	environment := &models.TaskEnvironment{ID: handoff.EnvironmentID, TaskID: handoff.TaskID, OwnershipGeneration: 7}
	if err := runtime.adoptUnrecordedPluginExecutor(ctx, environment, handoff); err != nil {
		t.Fatalf("seed the inventory row: %v", err)
	}
	stored, err := runtime.currentPluginExecutorRecord(ctx, request.SessionID, request.InstanceID)
	if err != nil {
		t.Fatalf("read the inventory row: %v", err)
	}
	inventory, err := decodePluginExecutorInventory(stored.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.Phase = pluginExecutorPhaseCleanupPending
	if err := runtime.checkpointPluginExecutorRecord(ctx, stored, inventory); err != nil {
		t.Fatalf("seed the cleanup_pending row: %v", err)
	}

	if err := runtime.SweepPendingCleanup(ctx); err == nil {
		t.Fatal("an unconfirmed destroy was reported as success")
	}
	if destroys != 1 {
		t.Fatalf("destroys = %d, want one attempt in the pass", destroys)
	}
}

// A failed read of a cleanup_pending row is a failure of the pass, not a
// rotated execution to skip quietly.
func TestSweepSurfacesAReadErrorOnACleanupPendingRow(t *testing.T) {
	runtime, operations, store := sweepStarvationRuntime(t, 0)
	store.getErr = errors.New("database is locked")

	err := runtime.SweepPendingCleanup(context.Background())
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("SweepPendingCleanup() = %v, want the read error", err)
	}
	if operations.destroyRequest != nil {
		t.Fatal("destroyed a row that could not be read")
	}
}

// After the rows the last pass visited disappear, the next pass resumes at the
// first still-failing row after them. An index cursor would skip the rows that
// moved into the visited positions.
func TestSweepResumesAtTheNextKeyWhenVisitedRowsDisappear(t *testing.T) {
	runtime, _, store := sweepStarvationRuntime(t, 3*pluginExecutorCleanupSweepBatch)
	visited := pluginExecutorCleanupSweepBatch - 1
	if err := runtime.SweepPendingCleanup(context.Background()); err == nil {
		t.Fatal("the first pass reported success")
	}
	store.unrecorded = store.unrecorded[visited:]

	err := runtime.SweepPendingCleanup(context.Background())
	next := fmt.Sprintf("execution-broken-%02d", visited)
	if err == nil || !strings.Contains(err.Error(), next) {
		t.Fatalf("second pass = %v, want it to resume at %s", err, next)
	}
}
