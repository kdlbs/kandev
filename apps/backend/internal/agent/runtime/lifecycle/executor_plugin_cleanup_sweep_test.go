package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func sweepRuntime(t *testing.T, destroyed bool) (*PluginRemoteExecutor, *persistedClaimOperations, *ExecutorCreateRequest, *models.UnrecordedPluginExecutor, context.Context) {
	t.Helper()
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	request := harness.request
	request.PluginExecutor.Profile.OwnershipGeneration = 7
	repo := seedArchiveRepository(t, request)
	ctx := context.Background()
	inventory := pluginExecutorLaunchInventory(request, request.PluginExecutor.Profile, request.InstanceID, "digest")
	inventory.Resource = workspaceResource("", "")
	inventory.Phase = pluginExecutorPhaseCleanupPending
	handoff := &models.UnrecordedPluginExecutor{
		ExecutionID: request.InstanceID, SessionID: request.SessionID, TaskID: request.TaskID, EnvironmentID: request.TaskEnvironmentID,
		Running: launchRunning(request, map[string]interface{}{MetadataKeyPluginExecutor: inventory}),
	}
	if err := repo.RecordUnrecordedPluginExecutor(ctx, handoff); err != nil {
		t.Fatal(err)
	}
	operations := &persistedClaimOperations{
		pluginExecutorOperationsFake: pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: destroyed}},
		repo:                         repo,
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(staticProfileLoader{profile: request.PluginExecutor.Profile}, repo)
	return runtime, operations, request, handoff, ctx
}

// A handoff nobody asked the environment cleanup to settle is still destroyed
// by the periodic sweep, under the cleanup claim, and forgotten once confirmed.
func TestSweepDestroysAHandoffWithoutAnEnvironmentCleanup(t *testing.T) {
	runtime, operations, request, _, ctx := sweepRuntime(t, true)

	if err := runtime.SweepPendingCleanup(ctx); err != nil {
		t.Fatalf("SweepPendingCleanup() error = %v", err)
	}
	if operations.destroyRequest == nil || operations.destroyRequest.GetResource().GetResourceHandle() != "opaque-resource" {
		t.Fatalf("destroy request = %v, want the handed-off handle", operations.destroyRequest)
	}
	repo := runtime.inventoryStore
	if left, _ := repo.ListUnrecordedPluginExecutors(ctx, request.TaskID, request.TaskEnvironmentID); len(left) != 0 {
		t.Fatalf("handoff survived a confirmed destroy: %d", len(left))
	}
}

func TestSweepKeepsAHandoffWhoseDestroyIsNotConfirmed(t *testing.T) {
	runtime, _, request, _, ctx := sweepRuntime(t, false)

	if err := runtime.SweepPendingCleanup(ctx); err == nil {
		t.Fatal("an unconfirmed destroy reported success")
	}
	if left, _ := runtime.inventoryStore.ListUnrecordedPluginExecutors(ctx, request.TaskID, request.TaskEnvironmentID); len(left) != 1 {
		t.Fatalf("handoff = %d after an unconfirmed destroy, want it retained for the next sweep", len(left))
	}
}

// The sweep is bounded work owned by Start and Stop: it runs on a timer, stops
// promptly, and can be started again.
func TestCleanupSweepRunsPeriodicallyAndRestarts(t *testing.T) {
	runtime, operations, request, handoff, ctx := sweepRuntime(t, true)
	runtime.cleanupSweepInterval = 5 * time.Millisecond
	settled := func() bool {
		left, _ := runtime.inventoryStore.ListUnrecordedPluginExecutors(ctx, request.TaskID, request.TaskEnvironmentID)
		return len(left) == 0
	}

	runtime.StartCleanupSweep(ctx)
	runtime.StartCleanupSweep(ctx) // idempotent
	require.Eventually(t, settled, 5*time.Second, 5*time.Millisecond)
	runtime.StopCleanupSweep()
	runtime.StopCleanupSweep() // idempotent
	require.NotNil(t, operations.destroyRequest)

	operations.destroyRequest = nil
	recorder := runtime.inventoryStore.(interface {
		RecordUnrecordedPluginExecutor(context.Context, *models.UnrecordedPluginExecutor) error
	})
	require.NoError(t, recorder.RecordUnrecordedPluginExecutor(ctx, handoff))
	runtime.StartCleanupSweep(ctx)
	require.Eventually(t, settled, 5*time.Second, 5*time.Millisecond)
	runtime.StopCleanupSweep()
	require.NotNil(t, operations.destroyRequest, "a restarted sweep did not run")
}

// A row left cleanup_pending by an unconfirmed destroy is retried by the sweep;
// a healthy ready row of the same environment is never touched.
func TestSweepRetriesCleanupPendingRowsAndLeavesReadyRowsAlone(t *testing.T) {
	runtime, operations, store := claimSettlementRuntime(t, pluginExecutorPhaseCleanupPending, checkpointedUnder(1), workspaceResource("", ""))
	require.NoError(t, runtime.SweepPendingCleanup(context.Background()))
	require.NotNil(t, operations.destroyRequest, "a retained cleanup_pending row was not retried")
	_ = store

	runtime, operations, _ = claimSettlementRuntime(t, "ready", nil, workspaceResource("", ""))
	require.NoError(t, runtime.SweepPendingCleanup(context.Background()))
	require.Nil(t, operations.destroyRequest, "the sweep destroyed a ready environment")
}
