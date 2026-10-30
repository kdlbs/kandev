package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

const recordedProfileSecret = "recorded-secret-value-0123"

// requireRecordedProfileSnapshot fails unless the request carried the recorded
// profile with its secret value revealed, as a provision request does.
func requireRecordedProfileSnapshot(t *testing.T, call string, snapshot *pluginsdk.ExecutorProfileSnapshot) {
	t.Helper()
	if snapshot == nil {
		t.Fatalf("%s carried no profile snapshot", call)
	}
	if snapshot.GetProfileId() != "profile-plugin-recovery" || snapshot.GetConfig()["region"] != "eu-west-1" ||
		snapshot.GetSecretValues()["credential"] != recordedProfileSecret {
		t.Fatalf("%s snapshot = %+v, want the recorded config and revealed secret", call, snapshot)
	}
}

func TestRestartRecoveryRequestsCarryTheRecordedProfile(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	operations := &pluginExecutorOperationsFake{
		attachResponse: &pluginsdk.AttachExecutorEnvironmentResponse{Resource: resource},
		inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{
			{State: "running", ExpiresAt: "2026-09-27T14:00:00Z"}, {State: "running", ExpiresAt: "2026-09-27T14:00:00Z"},
		},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: "2027-01-01T00:00:00Z", Generation: "lease-2",
		}},
	}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
		SecretValues: map[string]string{"credential": recordedProfileSecret},
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, "ready", resource),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateRunning},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
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
		t.Fatalf("RecoverInstances(): %v, %d instances", err, len(instances))
	}
	defer instances[0].Client.Close()
	requireRecordedProfileSnapshot(t, "attach", operations.attachRequest.GetProfile())
	if len(operations.inspectRequests) == 0 {
		t.Fatal("recovery issued no inspect")
	}
	for _, request := range operations.inspectRequests {
		requireRecordedProfileSnapshot(t, "inspect", request.GetProfile())
	}
	if len(operations.connectionRequests) == 0 {
		t.Fatal("recovery resolved no connection")
	}
	for _, request := range operations.connectionRequests {
		requireRecordedProfileSnapshot(t, "resolve connection", request.GetProfile())
	}
}

func TestTaskEnvironmentDestroyCarriesTheRecordedProfile(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, operations, _ := claimSettlementRuntime(t, "ready", nil, resource)
	loader := runtime.profileLoader.(*pluginExecutorRecoveryProfileLoaderFake)
	loader.profile.SecretValues = map[string]string{"credential": recordedProfileSecret}
	loader.profile.Config = map[string]string{"region": "eu-west-1"}
	store := runtime.inventoryStore.(*pluginExecutorInventoryStoreFake)
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	inventory.ProfileConfig = map[string]string{"region": "eu-west-1"}
	store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}

	_ = destroyClaimEnvironment(runtime)

	if operations.destroyRequest == nil {
		t.Fatal("destroy was not issued")
	}
	requireRecordedProfileSnapshot(t, "destroy", operations.destroyRequest.GetProfile())
}

func TestFailedLaunchRollbackDestroyCarriesTheLaunchProfile(t *testing.T) {
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	runtime := NewPluginRemoteExecutor(operations, logger.Default())
	runtime.SetRecoveryDependencies(nil, &pluginExecutorInventoryStoreFake{})
	request := pluginExecutorLaunchRequest(testPluginExecutorLaunchProvider())
	request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
	request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	profile := &request.PluginExecutor.Profile
	profile.ProfileID = "profile-plugin-recovery"
	profile.Config = map[string]string{"region": "eu-west-1"}
	profile.SecretValues = map[string]string{"credential": recordedProfileSecret}
	inventory := pluginExecutorLaunchInventory(request, *profile, request.InstanceID, "digest")
	operationContext := pluginExecutorRequestContext(context.Background(), request, *profile, request.InstanceID, "digest")

	_ = runtime.cleanupAfterPluginExecutorFailure(context.Background(), request, operationContext, inventory,
		&pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-1"}, "control_handshake", context.Canceled)

	if operations.destroyRequest == nil {
		t.Fatal("rollback sent no destroy request")
	}
	requireRecordedProfileSnapshot(t, "rollback destroy", operations.destroyRequest.GetProfile())
}

func TestAttachResponseEchoingTheRevealedSecretIsRejected(t *testing.T) {
	record := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	leaking := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery", StateJson: `{"resource":"` + recordedProfileSecret + `"}`, Platform: "linux-amd64", StateVersion: 1}
	operations := &pluginExecutorOperationsFake{
		attachResponse:   &pluginsdk.AttachExecutorEnvironmentResponse{Resource: leaking},
		inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{{State: "running"}},
	}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
		SecretValues: map[string]string{"credential": recordedProfileSecret},
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, "ready", record),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateRunning},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)

	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})

	if len(instances) != 0 {
		t.Fatalf("recovery adopted an attachment whose state echoes the secret: err=%v", err)
	}
	if len(operations.inspectRequests) != 1 {
		t.Fatalf("recovery inspected the leaking attachment (inspects=%d)", len(operations.inspectRequests))
	}
}
