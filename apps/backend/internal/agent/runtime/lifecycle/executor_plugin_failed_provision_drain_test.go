package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
)

const failedProvisionGeneration = 7

// failedProvisionFixture is a plugin launch against the real inventory
// repository, so a checkpoint is judged by the phase transitions the store
// enforces and not by a fake that accepts anything.
type failedProvisionFixture struct {
	workspaceLaunchHarness
	repo *tasksqlite.Repository
}

func newFailedProvisionFixture(t *testing.T) *failedProvisionFixture {
	t.Helper()
	provider := testPluginExecutorLaunchProvider()
	provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{{Name: pluginExecutorCapabilityDrainReceipt, Version: 1}}
	base := newWorkspaceLaunchHarness(t, provider, workspaceResource("", ""))
	fixture := &failedProvisionFixture{workspaceLaunchHarness: *base}
	request := fixture.request
	request.PluginExecutor.Profile.OwnershipGeneration = failedProvisionGeneration
	fixture.repo, _ = openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "kandev.db"))
	seedFailedProvisionRows(t, fixture.repo, request)
	manager := &Manager{}
	manager.SetExecutorRunningWriter(fixture.repo)
	manager.wirePluginExecutorInventoryPersistence(request, string(models.ExecutorTypePluginRemote))
	fixture.runtime.SetRecoveryDependencies(staticProfileLoader{profile: request.PluginExecutor.Profile}, fixture.repo)
	fixture.runtime.drainRuntime = (&pluginDrainerFake{}).drain
	return fixture
}

func seedFailedProvisionRows(t *testing.T, repo *tasksqlite.Repository, request *ExecutorCreateRequest) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: request.TaskID, Title: "launch fails"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: request.TaskEnvironmentID, TaskID: request.TaskID, ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: failedProvisionGeneration,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: request.SessionID, TaskID: request.TaskID, TaskEnvironmentID: request.TaskEnvironmentID}); err != nil {
		t.Fatal(err)
	}
}

// providerCheckpointsBootstrapping is what a provider's early launch
// checkpoint does to the stored inventory while its provision call is still
// running: the row moves ahead of the launch's own copy of it.
func (f *failedProvisionFixture) providerCheckpointsBootstrapping(t *testing.T) {
	t.Helper()
	inventory := pluginExecutorInventoryForLaunch(f.request, f.operations.provisionRequest.GetContext())
	inventory.Phase = pluginExecutorPhaseBootstrapping
	inventory.Resource = workspaceResource("", "")
	if err := checkpointPluginExecutor(context.Background(), f.request, inventory); err != nil {
		t.Fatalf("provider early checkpoint: %v", err)
	}
}

func (f *failedProvisionFixture) storedPhase(t *testing.T) string {
	t.Helper()
	row, err := f.repo.GetExecutorRunningBySessionID(context.Background(), f.request.SessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return "released"
	}
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := decodePluginExecutorInventory(row.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	return inventory.Phase
}

// A provider that fails its own provision call after it checkpointed the
// launch (its agentctl died before the control port answered) leaves a
// resource the launch recovers and rolls back. No handshake ever started, so
// the rollback owes no drain: it must record that, destroy under the launch
// reason and settle absent. Recording the drain outcome used to fail on the
// stale phase of the launch's copy, which left the cleanup pending for the
// environment reset to retry with a reason that cannot prove "nothing ran".
func TestFailedProvisionAfterTheProviderCheckpointedRollsBackAsAbsent(t *testing.T) {
	f := newFailedProvisionFixture(t)
	f.operations.provisionErr = &pluginExecutorProviderFailure{code: "control_unready", messageID: "provider.control_unready"}
	f.operations.onProvision = func() { f.providerCheckpointsBootstrapping(t) }
	f.operations.recoverResponse = &pluginsdk.RecoverExecutorOperationResponse{Outcome: "found", Resource: workspaceResource("", "")}

	_, err := f.runtime.CreateInstance(context.Background(), f.request)

	if err == nil || !strings.Contains(err.Error(), "allocated resource was removed") {
		t.Fatalf("CreateInstance() error = %v, want the rollback to remove the resource", err)
	}
	destroy := f.operations.destroyRequest
	if destroy == nil {
		t.Fatal("the rollback never reached the provider's destroy: the drain outcome was not persisted")
	}
	if destroy.GetCleanupReason() != pluginExecutorCleanupReasonLaunch || destroy.GetDrainOutcome() != pluginDrainOutcomeNotAttempted {
		t.Fatalf("destroy = reason %q drain %q, want %q / %q", destroy.GetCleanupReason(), destroy.GetDrainOutcome(),
			pluginExecutorCleanupReasonLaunch, pluginDrainOutcomeNotAttempted)
	}
	if phase := f.storedPhase(t); phase != "released" {
		t.Fatalf("stored inventory phase = %q, want the absent environment released", phase)
	}
}

// The drain evidence a failed launch sends is unchanged by the stored phase
// having moved: a launch that did handshake and cannot drain still reports an
// unreachable runtime, and one that holds its credential still drains it.
func TestFailedLaunchDrainEvidenceHoldsWhenTheStoredPhaseMovedAhead(t *testing.T) {
	cases := []struct {
		name      string
		handshake bool
		token     string
		want      string
	}{
		{"no handshake", false, "", pluginDrainOutcomeNotAttempted},
		{"handshake without a credential", true, "", pluginDrainOutcomeUnreachable},
		{"handshake with a credential", true, "token-1", pluginDrainOutcomeDrained},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFailedProvisionFixture(t)
			f.operations.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: workspaceResource("", "")}
			launch, err := f.runtime.preparePluginExecutorLaunch(context.Background(), f.request)
			if err != nil {
				t.Fatal(err)
			}
			f.operations.provisionRequest = &pluginsdk.ProvisionExecutorEnvironmentRequest{Context: launch.operationContext}
			f.providerCheckpointsBootstrapping(t)
			launch.inventory.HandshakeStarted = tc.handshake
			f.request.rollbackToken = tc.token

			_ = f.runtime.cleanupAfterPluginExecutorFailure(context.Background(), f.request, launch.operationContext, launch.inventory,
				workspaceResource("", ""), "control_handshake", errors.New("boom"))

			destroy := f.operations.destroyRequest
			if destroy == nil {
				t.Fatal("the rollback never reached the provider's destroy")
			}
			if got := destroy.GetDrainOutcome(); got != tc.want {
				t.Fatalf("drain_outcome = %q, want %q", got, tc.want)
			}
		})
	}
}
