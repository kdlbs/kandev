package lifecycle

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
)

// A task cleanup job that is queued, running or waiting to retry owns the
// task's resources, and its own runs carry the job into the claim. The sweep
// has no job, so it leaves such a handoff to the job instead of reporting the
// job's barrier as a failure on every pass.
func TestSweepLeavesAHandoffToTheTaskCleanupJobThatOwnsIt(t *testing.T) {
	early := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-early", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, operations, store := claimSettlementRuntime(t, pluginExecutorPhaseCleanupPending, nil, early)
	running := store.record
	inventory, err := decodePluginExecutorInventory(running.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	store.unrecorded = []*models.UnrecordedPluginExecutor{{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: inventory.EnvironmentID, Running: running,
	}}
	store.claimErr = fmt.Errorf("%w: %s", repoerrors.ErrTaskCleanupInProgress, running.TaskID)

	if err := runtime.SweepPendingCleanup(context.Background()); err != nil {
		t.Fatalf("sweep reported the owning cleanup job's barrier as a failure: %v", err)
	}
	if operations.destroyRequest != nil {
		t.Fatal("the sweep destroyed a resource the cleanup job owns")
	}
	if len(store.unrecorded) != 1 {
		t.Fatal("the handoff must stay recorded for the cleanup job")
	}
}

// The task cleanup job's own run of a handoff destroy carries the job into the
// claim request, which is what lets it pass the job's barrier.
func TestHandoffDestroyUnderATaskCleanupJobCarriesTheJobIntoTheClaim(t *testing.T) {
	early := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-early", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, _, store := claimSettlementRuntime(t, pluginExecutorPhaseCleanupPending, checkpointedUnder(1), early)
	running := store.record
	inventory, err := decodePluginExecutorInventory(running.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	store.unrecorded = []*models.UnrecordedPluginExecutor{{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: inventory.EnvironmentID, Running: running,
	}}
	ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{ID: "job-1", TaskID: running.TaskID})

	if err := runtime.DestroyTaskEnvironment(ctx, &models.TaskEnvironment{ID: inventory.EnvironmentID, TaskID: running.TaskID, OwnershipGeneration: 7}); err != nil {
		t.Fatalf("handoff destroy under the cleanup job: %v", err)
	}
	if store.claimRequest.CleanupJobID != "job-1" {
		t.Fatalf("claim request cleanup job = %q, want job-1", store.claimRequest.CleanupJobID)
	}
}
