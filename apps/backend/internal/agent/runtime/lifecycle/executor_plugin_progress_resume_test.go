package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

func progressResumeResource() *pluginsdk.ExecutorResourceDescriptor {
	return &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-saved-progress", StateJson: `{"resource":"partial"}`, Platform: "linux-amd64", StateVersion: 1,
	}
}

func progressResumeStore(t *testing.T, phase string, sessionState models.TaskSessionState) *pluginExecutorInventoryStoreFake {
	t.Helper()
	return &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, phase, progressResumeResource()),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: sessionState},
	}
}

func progressResumeLoader() *pluginExecutorRecoveryProfileLoaderFake {
	return &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
}

// A provider that checkpointed cleanup_progress under a cleanup claim owes the
// destroy. After a restart the saved row must resume it, not fail as an
// unattachable phase for ever.
func TestRestartRecoveryResumesSavedCleanupProgress(t *testing.T) {
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	store := progressResumeStore(t, pluginExecutorPhaseCleanupProgress, models.TaskSessionStateWaitingForInput)
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(progressResumeLoader(), store)

	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
	if err != nil {
		t.Fatalf("RecoverInstances(): %v", err)
	}
	if len(instances) != 0 || operations.destroyRequest == nil || operations.attachRequest != nil {
		t.Fatalf("instances=%d destroy=%v attach=%v, want the saved cleanup resumed", len(instances), operations.destroyRequest, operations.attachRequest)
	}
	if got := operations.destroyRequest.GetResource().GetResourceHandle(); got != "resource-saved-progress" {
		t.Fatalf("destroyed %q, want the checkpointed resource", got)
	}
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || persisted.Phase != pluginExecutorPhaseAbsent {
		t.Fatalf("inventory=%+v err=%v, want absent after a confirmed destroy", persisted, err)
	}
}

func TestSweepResumesSavedCleanupProgress(t *testing.T) {
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	store := progressResumeStore(t, pluginExecutorPhaseCleanupProgress, models.TaskSessionStateWaitingForInput)
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(progressResumeLoader(), store)

	if err := runtime.SweepPendingCleanup(context.Background()); err != nil {
		t.Fatalf("SweepPendingCleanup(): %v", err)
	}
	if operations.destroyRequest == nil {
		t.Fatal("the sweep ignored a row left in cleanup_progress")
	}
}

// recovery_progress is a recovery claim's checkpoint of a live resource. A
// restart re-enters recovery through the normal attach path.
func TestRestartRecoveryReentersSavedRecoveryProgress(t *testing.T) {
	operations := &pluginExecutorOperationsFake{
		attachResponse:   &pluginsdk.AttachExecutorEnvironmentResponse{Resource: progressResumeResource()},
		inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{{State: "running"}, {State: "running"}},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: "2027-01-01T00:00:00Z", Generation: "lease-2",
		}},
	}
	store := progressResumeStore(t, pluginExecutorPhaseRecoveryProgress, models.TaskSessionStateRunning)
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(progressResumeLoader(), store)
	setPluginRecoveredInstanceInfoFixture(t, runtime)
	runtime.newAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		if _, err := resolver(ctx); err != nil {
			return nil, err
		}
		return agentctl.NewClient("unused", 0, log, agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
	}
	runtime.ready = func(context.Context, *agentctl.Client) error { return nil }

	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
	if err != nil || len(instances) != 1 {
		t.Fatalf("RecoverInstances() = %d instances, %v", len(instances), err)
	}
	defer instances[0].Client.Close()
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || persisted.Phase != pluginExecutorPhaseReady {
		t.Fatalf("inventory=%+v err=%v, want ready after re-entering recovery", persisted, err)
	}
}
