package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type pluginExecutorRecoveryProfileLoaderFake struct {
	profile models.ExecutorProviderLaunchProfile
	called  bool
	config  map[string]string
	refs    map[string]string
}

func (f *pluginExecutorRecoveryProfileLoaderFake) ExecutorProviderProfileForLaunch(context.Context, string, string) (*models.ExecutorProviderLaunchProfile, error) {
	return nil, errors.New("unexpected launch profile lookup")
}

func (f *pluginExecutorRecoveryProfileLoaderFake) ExecutorProviderProfileForRecovery(
	_ context.Context,
	profileID, taskID, environmentID, providerIdentity string,
	generation int64,
	config, refs map[string]string,
) (*models.ExecutorProviderLaunchProfile, error) {
	f.called = true
	if profileID != f.profile.ProfileID || taskID != "task-plugin-recovery" || environmentID != "environment-plugin-recovery" ||
		providerIdentity != f.profile.Provider.Identity || generation != 7 {
		return nil, errors.New("recovery profile identity changed")
	}
	f.config = clonePluginExecutorStringMap(config)
	f.refs = clonePluginExecutorStringMap(refs)
	profile := f.profile
	profile.Config = clonePluginExecutorStringMap(config)
	profile.SecretReferences = clonePluginExecutorStringMap(refs)
	profile.OwnershipGeneration = generation
	return &profile, nil
}

type pluginExecutorInventoryStoreFake struct {
	record       *models.ExecutorRunning
	session      *models.TaskSession
	checkpoints  int
	claimRequest models.TaskEnvironmentRecoveryClaimRequest
	claimErr     error
	claim        *models.TaskEnvironmentRecoveryClaim
	deleted      bool
	releaseCount int
}

func (f *pluginExecutorInventoryStoreFake) CheckpointPluginExecutorInventory(_ context.Context, record *models.ExecutorRunning) error {
	if f.record != nil {
		current, err := decodePluginExecutorInventory(f.record.Metadata)
		if err != nil || f.record.AgentExecutionID != record.AgentExecutionID || record.ExpectedPluginExecutorRevision != current.Revision {
			return models.ErrExecutionRotated
		}
	} else if record.ExpectedPluginExecutorRevision != 0 {
		return models.ErrExecutionRotated
	}
	copy := *record
	data, err := json.Marshal(record.Metadata)
	if err != nil {
		return err
	}
	copy.Metadata = make(map[string]interface{})
	if err := json.Unmarshal(data, &copy.Metadata); err != nil {
		return err
	}
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = time.Now().UTC()
	}
	copy.UpdatedAt = time.Now().UTC()
	inventory, err := decodePluginExecutorInventory(copy.Metadata)
	if err != nil {
		return err
	}
	inventory.Revision = record.ExpectedPluginExecutorRevision + 1
	data, err = json.Marshal(map[string]interface{}{MetadataKeyPluginExecutor: inventory})
	if err != nil {
		return err
	}
	copy.Metadata = make(map[string]interface{})
	if err := json.Unmarshal(data, &copy.Metadata); err != nil {
		return err
	}
	f.record = &copy
	record.UpdatedAt = copy.UpdatedAt
	record.Metadata = copy.Metadata
	record.ExpectedPluginExecutorRevision = inventory.Revision
	f.checkpoints++
	return nil
}

func (f *pluginExecutorInventoryStoreFake) DeletePluginExecutorInventoryIfCurrent(_ context.Context, sessionID, executionID string, generation int64) error {
	if f.record == nil || f.record.SessionID != sessionID {
		return models.ErrExecutorRunningNotFound
	}
	if f.record.AgentExecutionID != executionID {
		return models.ErrExecutionRotated
	}
	inventory, err := decodePluginExecutorInventory(f.record.Metadata)
	if err != nil || inventory.EnvironmentGeneration != generation {
		return models.ErrExecutionRotated
	}
	f.deleted = true
	f.record = nil
	return nil
}

func (f *pluginExecutorInventoryStoreFake) AcquireTaskEnvironmentRecoveryClaim(_ context.Context, request models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	f.claimRequest = request
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID: request.TaskEnvironmentID, OwnerTaskID: request.OwnerTaskID,
		OwnershipGeneration: request.OwnershipGeneration, SessionID: request.SessionID,
		OperationID: request.OperationID, ExecutorType: request.ExecutorType,
	}
	f.claim = claim
	return claim, nil
}

func (f *pluginExecutorInventoryStoreFake) ReleaseTaskEnvironmentRecoveryClaim(context.Context, *models.TaskEnvironmentRecoveryClaim) error {
	f.releaseCount++
	return nil
}

func (f *pluginExecutorInventoryStoreFake) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	if f.session == nil {
		return nil, errors.New("session not found")
	}
	copy := *f.session
	return &copy, nil
}

func (f *pluginExecutorInventoryStoreFake) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	if f.record == nil {
		return nil, models.ErrExecutorRunningNotFound
	}
	copy := *f.record
	return &copy, nil
}

func (f *pluginExecutorInventoryStoreFake) ListExecutorsRunningPluginRemote(context.Context) ([]*models.ExecutorRunning, error) {
	if f.record == nil {
		return nil, nil
	}
	copy := *f.record
	return []*models.ExecutorRunning{&copy}, nil
}

func TestPluginExecutorRestartRecovery(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	operations := &pluginExecutorOperationsFake{
		attachResponse: &pluginsdk.AttachExecutorEnvironmentResponse{Resource: resource},
		inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{
			{State: "running", ExpiresAt: "2026-09-27T14:00:00Z"},
			{State: "running", ExpiresAt: "2026-09-27T14:00:00Z"},
		},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: "2027-01-01T00:00:00Z", Generation: "lease-2",
		}},
	}
	profile := models.ExecutorProviderLaunchProfile{Provider: provider, ProfileID: "profile-plugin-recovery"}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: profile}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, "ready", resource),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateRunning},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	setPluginRecoveredInstanceInfoFixture(t, runtime)
	runtime.newRecoveredAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		if executionID != "execution-plugin-recovery" || token != "agentctl-secret" {
			t.Fatalf("recovered client identity = %q token=%q", executionID, token)
		}
		if _, err := resolver(ctx); err != nil {
			return nil, err
		}
		return agentctl.NewClient("unused", 0, log, agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
	}
	runtime.ready = func(_ context.Context, client *agentctl.Client) error {
		if client.AuthToken() != "agentctl-secret" {
			t.Fatalf("recovered agentctl token = %q", client.AuthToken())
		}
		return nil
	}

	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
	if err != nil {
		t.Fatalf("RecoverInstances(): %v", err)
	}
	if len(instances) != 1 || instances[0].InstanceID != "execution-plugin-recovery" || instances[0].RuntimeName != agentruntime.RuntimePluginRemote {
		t.Fatalf("recovered instances = %#v", instances)
	}
	defer instances[0].Client.Close()
	if instances[0].WorkspacePath != pluginExecutorWorkspacePath ||
		!slices.Equal(instances[0].WorkspaceSourceRoots, []string{"/workspace", "/workspace/src"}) ||
		instances[0].Env["KANDEV_RUN_ID"] != "run-plugin-recovery" {
		t.Fatalf("recovered live instance metadata = path %q roots %v env %v", instances[0].WorkspacePath, instances[0].WorkspaceSourceRoots, instances[0].Env)
	}
	if !loader.called || loader.config["region"] != "eu-west-1" || loader.refs["credential"] != "vault-ref-2" {
		t.Fatalf("recorded profile snapshot was not restored: called=%v config=%v refs=%v", loader.called, loader.config, loader.refs)
	}
	if operations.attachRequest == nil || operations.attachRequest.GetResource().GetResourceHandle() != "resource-recovery" ||
		operations.attachRequest.GetExpectedRuntimeIdentity() != "execution-plugin-recovery" {
		t.Fatalf("attach request = %#v", operations.attachRequest)
	}
	if len(operations.inspectRequests) != 2 || store.checkpoints < 3 {
		t.Fatalf("inspect count=%d checkpoints=%d", len(operations.inspectRequests), store.checkpoints)
	}
	if want := []int{testPluginExecutorInstancePort}; !slices.Equal(operations.connectionPorts, want) {
		t.Fatalf("recovery leased runtime ports = %v, want the recorded instance port %v", operations.connectionPorts, want)
	}
	if store.record.ResumeToken != "resume-preserved" || store.record.LastMessageUUID != "message-preserved" {
		t.Fatalf("recovery changed conversation state: %+v", store.record)
	}
}

func TestPluginExecutorRecoveredClientRequiresRecordedInstancePort(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery"}
	record := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.InstancePort = 0
	var clientFactoryCalled bool
	operations := &pluginExecutorOperationsFake{}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.newRecoveredAgentctlClient = func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.Client, error) {
		clientFactoryCalled = true
		return nil, errors.New("unexpected client creation")
	}
	state := &pluginExecutorRecoveryState{
		inventory:        inventory,
		operationContext: &pluginsdk.ExecutorProviderRequestContext{},
		resource:         resource,
	}

	_, err = runtime.newRecoveredPluginExecutorInstance(context.Background(), record, state)
	if err == nil || !strings.Contains(err.Error(), "no agentctl instance port") {
		t.Fatalf("newRecoveredPluginExecutorInstance() error = %v, want missing port", err)
	}
	if clientFactoryCalled || len(operations.connectionPorts) != 0 {
		t.Fatalf("missing port reached connection setup: factory=%v ports=%v", clientFactoryCalled, operations.connectionPorts)
	}
}

func TestPluginExecutorRetainedAttachmentRequiresRecordedInstancePort(t *testing.T) {
	inventory := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-recovery",
	})
	recovered, err := decodePluginExecutorInventory(inventory.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	recovered.InstancePort = 0
	if pluginExecutorInventoryRetainedAttachable(recovered, recovered.EnvironmentID) {
		t.Fatal("retained inventory without an instance port was attachable")
	}
}

func TestPluginExecutorUnknownOperation(t *testing.T) {
	provider := testPluginExecutorLaunchProvider()
	operations := &pluginExecutorOperationsFake{
		recoverResponse: &pluginsdk.RecoverExecutorOperationResponse{Outcome: "unknown"},
	}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: provider, ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{record: pluginExecutorRecoveryRecord(t, "allocating", nil)}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
	if err != nil {
		t.Fatalf("RecoverInstances(): %v", err)
	}
	if len(instances) != 0 || operations.recoverRequest == nil || operations.recoverRequest.GetContext().GetOperationId() != "operation-plugin-recovery" {
		t.Fatalf("instances=%#v recover request=%#v", instances, operations.recoverRequest)
	}
	if operations.provisionRequest != nil || operations.attachRequest != nil || store.deleted || store.checkpoints != 0 {
		t.Fatalf("unknown allocation outcome mutated inventory or retried allocation: %#v", operations)
	}
}

func TestPluginExecutorPreHandshakeRecoveryCleansKnownResources(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-partial-bootstrap", StateJson: `{"resource":"partial"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	for _, phase := range []string{"artifact_staging", "bootstrapping", "provisioned"} {
		t.Run(phase, func(t *testing.T) {
			operations := &pluginExecutorOperationsFake{
				destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
			}
			loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
				Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
			}}
			store := &pluginExecutorInventoryStoreFake{
				record:  pluginExecutorRecoveryRecord(t, phase, resource),
				session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
			}
			store.record.TransientAuthToken = ""
			runtime := NewPluginRemoteExecutor(operations, newTestLogger())
			runtime.SetRecoveryDependencies(loader, store)

			instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
			if err != nil {
				t.Fatalf("RecoverInstances(): %v", err)
			}
			if len(instances) != 0 || operations.destroyRequest == nil || operations.attachRequest != nil {
				t.Fatalf("recovery instances=%#v destroy=%#v attach=%#v", instances, operations.destroyRequest, operations.attachRequest)
			}
			persisted, err := decodePluginExecutorInventory(store.record.Metadata)
			if err != nil || persisted.Phase != "absent" || persisted.Resource != nil {
				t.Fatalf("partial bootstrap inventory=%+v err=%v", persisted, err)
			}
		})
	}
}

func TestPluginExecutorReadyRecoveryWithoutTokenPreservesEstablishedResource(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-established", StateJson: `{"resource":"established"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	operations := &pluginExecutorOperationsFake{}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, "ready", resource),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	store.record.TransientAuthToken = ""
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)

	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})

	require.NoError(t, err)
	require.Empty(t, instances, "a missing local token must leave the established provider resource blocked")
	require.Nil(t, operations.destroyRequest, "missing credentials do not prove that remote compute is incomplete")
	require.Nil(t, operations.attachRequest, "the provider must not be contacted without the saved credential")
	require.False(t, store.deleted, "the established inventory row remains available for later retry")
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	require.NoError(t, err)
	require.Equal(t, "ready", persisted.Phase)
	require.NotNil(t, persisted.Resource)
	require.Equal(t, resource.GetResourceHandle(), persisted.Resource.GetResourceHandle())
}

func TestPluginExecutorPreHandshakeUnknownDestroyRetainsCleanupInventory(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-partial-bootstrap", StateJson: `{"resource":"partial"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, "bootstrapping", resource),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	store.record.TransientAuthToken = ""
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	if _, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record}); err != nil {
		t.Fatalf("RecoverInstances(): %v", err)
	}
	persisted, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || persisted.Phase != "cleanup_pending" || persisted.Resource == nil || persisted.Resource.GetResourceHandle() != resource.GetResourceHandle() {
		t.Fatalf("unconfirmed partial bootstrap cleanup inventory=%+v err=%v", persisted, err)
	}
}

func TestPluginExecutorPostHandshakeRecoveryAttachesEveryCheckpointPhase(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-partial-bootstrap", StateJson: `{"resource":"partial"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	for _, phase := range []string{"artifact_staging", "bootstrapping", "provisioned", "ready"} {
		t.Run(phase, func(t *testing.T) {
			operations := &pluginExecutorOperationsFake{
				attachResponse: &pluginsdk.AttachExecutorEnvironmentResponse{Resource: resource},
				inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{
					{State: "running"}, {State: "running"},
				},
				connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
					BaseUrl: "https://executor.example", ExpiresAt: "2027-01-01T00:00:00Z", Generation: "lease-2",
				}},
			}
			loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
				Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
			}}
			store := &pluginExecutorInventoryStoreFake{
				record:  pluginExecutorRecoveryRecord(t, phase, resource),
				session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateRunning},
			}
			runtime := NewPluginRemoteExecutor(operations, newTestLogger())
			runtime.SetRecoveryDependencies(loader, store)
			setPluginRecoveredInstanceInfoFixture(t, runtime)
			runtime.newRecoveredAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
				if executionID != "execution-plugin-recovery" || token != "agentctl-secret" {
					t.Fatalf("recovered client execution=%q token=%q", executionID, token)
				}
				if _, err := resolver(ctx); err != nil {
					return nil, err
				}
				return agentctl.NewClient("unused", 0, log, agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
			}
			runtime.ready = func(_ context.Context, client *agentctl.Client) error {
				if client.AuthToken() != "agentctl-secret" {
					return errors.New("recovered token was not applied")
				}
				return nil
			}

			instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
			if err != nil || len(instances) != 1 {
				t.Fatalf("RecoverInstances() = %d instances, %v", len(instances), err)
			}
			defer instances[0].Client.Close()
			if operations.attachRequest == nil || operations.destroyRequest != nil {
				t.Fatalf("attach request=%#v destroy request=%#v", operations.attachRequest, operations.destroyRequest)
			}
			persisted, err := decodePluginExecutorInventory(store.record.Metadata)
			if err != nil || persisted.Phase != "ready" {
				t.Fatalf("recovered inventory=%+v err=%v", persisted, err)
			}
		})
	}
}

func setPluginRecoveredInstanceInfoFixture(t *testing.T, runtime *PluginRemoteExecutor) {
	t.Helper()
	runtime.recoveredAgentctlInfoReader = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, _ *logger.Logger, executionID, token string) (*agentctl.InstanceInfo, error) {
		if executionID != "execution-plugin-recovery" || token != "agentctl-secret" {
			t.Fatalf("instance metadata read identity = %q token=%q", executionID, token)
		}
		return &agentctl.InstanceInfo{
			ID: executionID, TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
			Port: testPluginExecutorInstancePort, WorkspacePath: pluginExecutorWorkspacePath,
			Env:                  map[string]string{"KANDEV_RUN_ID": "run-plugin-recovery"},
			WorkspaceSourceRoots: []string{"/workspace", "/workspace/src"}, ProviderSessionID: "native-plugin-session",
		}, nil
	}
}

func TestPluginRecoveredInstanceMetadataUsesAuthenticatedControlReader(t *testing.T) {
	var gotExecutionID string
	var gotAuthToken string
	responseSession := "session-plugin-recovery"
	operations := &pluginExecutorOperationsFake{connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{
		Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Generation: "lease-read-only",
		},
	}}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.recoveredAgentctlInfoReader = func(
		ctx context.Context,
		resolver agentctl.ConnectionLeaseResolver,
		_ *logger.Logger,
		executionID string,
		token string,
	) (*agentctl.InstanceInfo, error) {
		gotExecutionID = executionID
		gotAuthToken = token
		_, err := resolver(ctx)
		if err != nil {
			return nil, err
		}
		return &agentctl.InstanceInfo{
			ID: "execution-plugin-recovery", TaskID: "task-plugin-recovery", SessionID: responseSession,
			Port: testPluginExecutorInstancePort, WorkspacePath: pluginExecutorWorkspacePath,
			Env:                  map[string]string{"KANDEV_RUN_ID": "run-plugin-recovery"},
			WorkspaceSourceRoots: []string{"/workspace/src"}, ProviderSessionID: "native-plugin-session",
		}, nil
	}
	record := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery"})
	state := &pluginExecutorRecoveryState{
		inventory: pluginExecutorInventory{
			Resource:     &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-recovery"},
			InstancePort: testPluginExecutorInstancePort,
		},
		operationContext: &pluginsdk.ExecutorProviderRequestContext{},
	}
	info, err := runtime.readRecoveredAgentctlInstance(context.Background(), record, state)
	require.NoError(t, err)
	require.Equal(t, "execution-plugin-recovery", gotExecutionID)
	require.Equal(t, "agentctl-secret", gotAuthToken)
	require.Equal(t, int(pluginExecutorRuntimePort), operations.connectionPorts[0])
	require.Equal(t, "run-plugin-recovery", info.Env["KANDEV_RUN_ID"])
	require.Equal(t, []string{"/workspace/src"}, info.WorkspaceSourceRoots)
	responseSession = "different-session"
	_, err = runtime.readRecoveredAgentctlInstance(context.Background(), record, state)
	require.Error(t, err, "a control endpoint must not hydrate metadata from a different task session")
}

func TestPluginExecutorResetCleanupUsesTaskClaim(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-recovery", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	operations := &pluginExecutorOperationsFake{destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	store := &pluginExecutorInventoryStoreFake{
		record:  pluginExecutorRecoveryRecord(t, "ready", resource),
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateWaitingForInput},
	}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.SetRecoveryDependencies(loader, store)
	ctx := recoveryclaim.WithTaskCleanupJob(context.Background(), recoveryclaim.TaskCleanupJob{
		ID: "reset-job-plugin-recovery", TaskID: "task-plugin-recovery",
	})
	if err := runtime.DestroyTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-plugin-recovery", TaskID: "task-plugin-recovery", OwnershipGeneration: 7,
	}); err != nil {
		t.Fatalf("DestroyTaskEnvironment(): %v", err)
	}
	if operations.destroyRequest == nil || operations.destroyRequest.GetResource().GetResourceHandle() != "resource-recovery" {
		t.Fatalf("destroy request = %#v", operations.destroyRequest)
	}
	if store.claimRequest.CleanupJobID != "reset-job-plugin-recovery" || !store.claimRequest.AllowCurrentSessionRuntime {
		t.Fatalf("cleanup claim request = %+v", store.claimRequest)
	}
	if store.record == nil || store.record.ResumeToken != "resume-preserved" {
		t.Fatalf("reset cleanup lost conversation state: %+v", store.record)
	}
	inventory, err := decodePluginExecutorInventory(store.record.Metadata)
	if err != nil || inventory.Phase != "absent" || inventory.Resource != nil {
		t.Fatalf("reset inventory = %+v, err=%v", inventory, err)
	}
}

const testPluginExecutorInstancePort = 41234

func pluginExecutorRecoveryRecord(t *testing.T, phase string, resource *pluginsdk.ExecutorResourceDescriptor) *models.ExecutorRunning {
	t.Helper()
	inventory := pluginExecutorInventory{
		PluginID: "example", InstallationID: "install-1", ProviderKey: "remote", ProviderIdentity: "plugin:example:remote",
		ContractVersion: 1, SupportedStateVersions: []int{1}, StateVersion: 1,
		EnvironmentID: "environment-plugin-recovery", ProfileID: "profile-plugin-recovery",
		ProfileConfig: map[string]string{"region": "eu-west-1"}, SecretReferences: map[string]string{"credential": "vault-ref-2"},
		OperationID: "operation-plugin-recovery", InputDigest: "digest-plugin-recovery", Phase: phase,
		Resource: resource, InstancePort: testPluginExecutorInstancePort, EnvironmentGeneration: 7,
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("marshal plugin inventory: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode plugin inventory metadata: %v", err)
	}
	metadata := map[string]interface{}{MetadataKeyPluginExecutor: envelope}
	return &models.ExecutorRunning{
		ID: "session-plugin-recovery", SessionID: "session-plugin-recovery", TaskID: "task-plugin-recovery",
		ExecutionProfileID: "agent-profile-plugin-recovery", ExecutorID: "exec-plugin-1", Runtime: agentruntime.RuntimePluginRemote,
		Status: models.ExecutorRunningStatusRunning, Resumable: true, ResumeToken: "resume-preserved", LastMessageUUID: "message-preserved",
		AgentExecutionID: "execution-plugin-recovery", TransientAuthToken: "agentctl-secret", Metadata: metadata,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}
