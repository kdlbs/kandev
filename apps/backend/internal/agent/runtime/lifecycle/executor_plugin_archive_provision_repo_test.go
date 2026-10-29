package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
)

// persistedClaimOperations destroys only under a cleanup claim that exists as a
// row, the way a provider that checkpoints cleanup_progress first does, and
// records the claim row it saw.
type persistedClaimOperations struct {
	pluginExecutorOperationsFake
	repo *tasksqlite.Repository
}

func (f *persistedClaimOperations) DestroyExecutorEnvironment(ctx context.Context, req *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error) {
	claim, err := f.repo.GetTaskEnvironmentRecoveryClaim(ctx, req.GetContext().GetEnvironmentId())
	if err != nil || claim == nil || claim.OperationID != req.GetCleanupClaim() {
		return nil, errors.New("cleanup claim not found: checkpoint refused")
	}
	return f.pluginExecutorOperationsFake.DestroyExecutorEnvironment(ctx, req)
}

// staticProfileLoader returns the launch profile for any recovery lookup.
type staticProfileLoader struct {
	profile models.ExecutorProviderLaunchProfile
}

func (l staticProfileLoader) ExecutorProviderProfileForLaunch(context.Context, string, string) (*models.ExecutorProviderLaunchProfile, error) {
	return &l.profile, nil
}

func (l staticProfileLoader) ExecutorProviderProfileForRecovery(context.Context, string, string, string, string, int64, map[string]string, map[string]string) (*models.ExecutorProviderLaunchProfile, error) {
	return &l.profile, nil
}

func seedArchiveRepository(t *testing.T, request *ExecutorCreateRequest) *tasksqlite.Repository {
	t.Helper()
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "kandev.db"))
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: request.TaskID, Title: "archive during provision"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: request.TaskEnvironmentID, TaskID: request.TaskID, ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: 7,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: request.SessionID, TaskID: request.TaskID, TaskEnvironmentID: request.TaskEnvironmentID}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func launchRunning(request *ExecutorCreateRequest, metadata map[string]interface{}) *models.ExecutorRunning {
	return &models.ExecutorRunning{
		ID: request.SessionID, SessionID: request.SessionID, TaskID: request.TaskID, ExecutorID: "exec-plugin-1",
		ExecutionProfileID: request.AgentProfileID, Runtime: agentruntime.RuntimePluginRemote,
		Status: models.ExecutorRunningStatusStarting, AgentExecutionID: request.InstanceID, Metadata: metadata,
	}
}

// The rig's archive-during-provision failure, against the real repository: the
// archive removes the inventory row (the rig found none) and holds the environment while the
// provider returns, so the launch can neither claim nor record the resource.
// The resource must survive in durable storage, and a cleanup run by a new
// process (a restart) must destroy it under its own persisted claim and only
// then settle.
func TestArchiveDuringProvisionIsCleanedUpUnderItsOwnClaimAfterARestart(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	request := harness.request
	request.PluginExecutor.Profile.OwnershipGeneration = 7
	repo := seedArchiveRepository(t, request)
	ctx := context.Background()
	harness.runtime.SetRecoveryDependencies(nil, repo)
	harness.runtime.cleanupClaimWait, harness.runtime.cleanupClaimRetry = 50*time.Millisecond, 5*time.Millisecond

	var archiveClaim *models.TaskEnvironmentRecoveryClaim
	request.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		envelope, _ := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory)
		running := launchRunning(request, metadata)
		if envelope.Resource != nil && archiveClaim == nil {
			var err error
			if archiveClaim, err = repo.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
				TaskEnvironmentID: request.TaskEnvironmentID, OwnerTaskID: request.TaskID, OwnershipGeneration: 7, SessionID: request.SessionID,
				OperationID: "archive-job:cleanup", ExecutorType: string(models.ExecutorTypePluginRemote), AllowCurrentSessionRuntime: true,
			}); err != nil {
				t.Fatalf("archive claim: %v", err)
			}
			if err = repo.DeletePluginExecutorInventoryIfCurrent(recoveryclaim.WithClaim(ctx, archiveClaim), request.SessionID, request.InstanceID, 7); err != nil {
				t.Fatalf("archive inventory removal: %v", err)
			}
		}
		return repo.CheckpointPluginExecutorInventory(ctx, running)
	}
	request.RecordUnrecordedRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		envelope, _ := metadata[MetadataKeyPluginExecutor].(pluginExecutorInventory)
		envelope.NegotiatedCapabilities = []string{pluginExecutorCapabilityDrainReceipt + "@1"}
		metadata[MetadataKeyPluginExecutor] = envelope
		return repo.RecordUnrecordedPluginExecutor(ctx, &models.UnrecordedPluginExecutor{
			ExecutionID: request.InstanceID, SessionID: request.SessionID, TaskID: request.TaskID,
			EnvironmentID: request.TaskEnvironmentID, Running: launchRunning(request, metadata),
		})
	}

	if _, err := harness.runtime.CreateInstance(ctx, request); err == nil {
		t.Fatal("CreateInstance() unexpectedly succeeded")
	}
	if archiveClaim == nil {
		t.Fatal("the archive never took the environment; the scenario did not run")
	}
	if err := repo.ReleaseTaskEnvironmentRecoveryClaim(ctx, archiveClaim); err != nil {
		t.Fatal(err)
	}

	operations := &persistedClaimOperations{
		pluginExecutorOperationsFake: pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}},
		repo:                         repo,
	}
	loader := staticProfileLoader{profile: request.PluginExecutor.Profile}
	restarted := NewPluginRemoteExecutor(operations, newTestLogger())
	restarted.SetRecoveryDependencies(loader, repo)
	environment := &models.TaskEnvironment{ID: request.TaskEnvironmentID, TaskID: request.TaskID, OwnershipGeneration: 7}

	if err := restarted.DestroyTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("DestroyTaskEnvironment() error = %v", err)
	}
	if operations.destroyRequest == nil || operations.destroyRequest.GetResource().GetResourceHandle() != "opaque-resource" {
		t.Fatalf("destroy request = %v, want the provisioned handle", operations.destroyRequest)
	}
	// No runtime ever started for a handed-off launch: the destroy carries the
	// same launch cleanup reason and not_attempted outcome, with no receipt,
	// that a launch rollback sends, so a provider can treat it as no-runtime proof.
	if got := operations.destroyRequest.GetCleanupReason(); got != pluginExecutorCleanupReasonLaunch {
		t.Fatalf("destroy cleanup_reason = %q, want %q", got, pluginExecutorCleanupReasonLaunch)
	}
	if got := operations.destroyRequest.GetDrainOutcome(); got != pluginDrainOutcomeNotAttempted || operations.destroyRequest.GetDrainReceipt() != nil {
		t.Fatalf("destroy drain = (%q, %v), want (%q, no receipt)", got, operations.destroyRequest.GetDrainReceipt(), pluginDrainOutcomeNotAttempted)
	}
	if left, _ := repo.ListUnrecordedPluginExecutors(ctx, request.TaskID, request.TaskEnvironmentID); len(left) != 0 {
		t.Fatalf("handoff survived a confirmed destroy: %d", len(left))
	}
	if err := restarted.DestroyTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("a settled environment must clean up again without error: %v", err)
	}
}

// An unconfirmed destroy must leave the durable record in place so the next
// cleanup attempt still has the handle.
func TestUnconfirmedDestroyKeepsTheDurableRecord(t *testing.T) {
	harness := newWorkspaceLaunchHarness(t, testPluginExecutorLaunchProvider(), workspaceResource("", ""))
	request := harness.request
	request.PluginExecutor.Profile.OwnershipGeneration = 7
	repo := seedArchiveRepository(t, request)
	ctx := context.Background()
	if err := repo.RecordUnrecordedPluginExecutor(ctx, &models.UnrecordedPluginExecutor{
		ExecutionID: request.InstanceID, SessionID: request.SessionID, TaskID: request.TaskID, EnvironmentID: request.TaskEnvironmentID,
		Running: launchRunning(request, map[string]interface{}{MetadataKeyPluginExecutor: func() pluginExecutorInventory {
			inventory := pluginExecutorLaunchInventory(request, request.PluginExecutor.Profile, request.InstanceID, "digest")
			inventory.Resource = workspaceResource("", "")
			inventory.Phase = pluginExecutorPhaseCleanupPending
			return inventory
		}()}),
	}); err != nil {
		t.Fatal(err)
	}
	operations := &persistedClaimOperations{
		pluginExecutorOperationsFake: pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{}},
		repo:                         repo,
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(staticProfileLoader{profile: request.PluginExecutor.Profile}, repo)

	environment := &models.TaskEnvironment{ID: request.TaskEnvironmentID, TaskID: request.TaskID, OwnershipGeneration: 7}
	if err := runtime.DestroyTaskEnvironment(ctx, environment); err == nil {
		t.Fatal("an unconfirmed destroy reported success")
	}
	if left, _ := repo.ListUnrecordedPluginExecutors(ctx, request.TaskID, request.TaskEnvironmentID); len(left) != 1 {
		t.Fatalf("durable record = %d after an unconfirmed destroy, want 1", len(left))
	}
}
