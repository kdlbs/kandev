package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	durableNonce = "seeded-nonce-6f1c"
	durableToken = "seeded-token-9a2e"
)

type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) index(event string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Index(l.events, event)
}

type runtimeSecretsFake struct {
	log    *eventLog
	values map[string]string
}

func newRuntimeSecretsFake(log *eventLog) *runtimeSecretsFake {
	return &runtimeSecretsFake{log: log, values: map[string]string{}}
}

func (f *runtimeSecretsFake) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	f.values[secret.ID] = secret.Value
	f.log.add("secret:create:" + secret.ID)
	return nil
}

func (f *runtimeSecretsFake) Get(_ context.Context, id string) (*secrets.Secret, error) {
	if _, ok := f.values[id]; !ok {
		return nil, secrets.ErrNotFound
	}
	return &secrets.Secret{ID: id}, nil
}

func (f *runtimeSecretsFake) Update(_ context.Context, id string, req *secrets.UpdateSecretRequest) error {
	f.values[id] = *req.Value
	f.log.add("secret:update:" + id)
	return nil
}

func (f *runtimeSecretsFake) Delete(_ context.Context, id string) error {
	delete(f.values, id)
	f.log.add("secret:delete:" + id)
	return nil
}

func durableLaunchProvider() models.ExecutorProvider {
	provider := testPluginExecutorLaunchProvider()
	provider.RequiredCapabilities = []models.ExecutorProviderCapabilityRequirement{{Name: "durable_bootstrap", Version: 1}}
	return provider
}

type durableLaunchFixture struct {
	runtime     *PluginRemoteExecutor
	operations  *capabilityConfirmerFake
	request     *ExecutorCreateRequest
	secrets     *runtimeSecretsFake
	log         *eventLog
	checkpoints []map[string]interface{}
	logs        *observer.ObservedLogs
}

func newDurableLaunchFixture(t *testing.T, provider models.ExecutorProvider) *durableLaunchFixture {
	t.Helper()
	fixture := &durableLaunchFixture{log: &eventLog{}}
	fixture.operations = &capabilityConfirmerFake{}
	fixture.operations.provisionResponse = &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "opaque-resource", StateJson: `{"resource_id":"fixture-1"}`, Platform: "linux-amd64", StateVersion: 1,
		Capabilities: &pluginsdk.ExecutorProviderCapabilities{Terminal: true, Files: true, Git: true},
	}}
	fixture.operations.connectionResponse = &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
		BaseUrl: "https://executor.example", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Generation: "generation-1",
	}}
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	fixture.logs = observed
	fixture.runtime = NewPluginRemoteExecutor(fixture.operations, log)
	fixture.secrets = newRuntimeSecretsFake(fixture.log)
	fixture.runtime.SetRuntimeSecretStore(fixture.secrets)
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var instance agentctl.CreateInstanceRequest
		_ = json.NewDecoder(r.Body).Decode(&instance)
		fixture.log.add("create_instance")
		_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: instance.ID, Port: testPluginExecutorInstancePort})
	}))
	t.Cleanup(control.Close)
	controlURL, _ := url.Parse(control.URL)
	controlPort, _ := strconv.Atoi(controlURL.Port())
	fixture.runtime.newAgentctlControlClient = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, log *logger.Logger, nonce string) (*agentctl.ControlClient, string, error) {
		fixture.log.add("handshake")
		if nonce != durableNonce && nonce != "" {
			fixture.log.add("handshake:unexpected-nonce")
		}
		return agentctl.NewControlClient(controlURL.Hostname(), controlPort, log), durableToken, nil
	}
	fixture.runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	fixture.operations.onProvision = func() { fixture.log.add("provision") }
	fixture.request = pluginExecutorLaunchRequest(provider)
	fixture.request.CheckpointRuntimeInventory = func(_ context.Context, checkpoint map[string]interface{}) error {
		encoded, _ := json.Marshal(checkpoint)
		var copied map[string]interface{}
		_ = json.Unmarshal(encoded, &copied)
		if refs, ok := fixture.request.Metadata[MetadataKeyBootstrapNonceSecret]; ok {
			copied["_nonce_ref"] = refs
		}
		fixture.checkpoints = append(fixture.checkpoints, copied)
		fixture.log.add("checkpoint")
		return nil
	}
	fixture.request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	return fixture
}

func TestDurableBootstrapPersistsTheNonceBeforeTheProvisionCall(t *testing.T) {
	fixture := newDurableLaunchFixture(t, durableLaunchProvider())
	fixture.runtime.newAgentctlControlClient = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, _ *logger.Logger, nonce string) (*agentctl.ControlClient, string, error) {
		require.Equal(t, fixture.secrets.values["kandev-runtime:execution-plugin-1:agentctl-bootstrap"], nonce)
		return nil, "", errors.New("stop after the handshake")
	}
	fixture.operations.destroyResponse = &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true}
	_, _ = fixture.runtime.CreateInstance(context.Background(), fixture.request)

	nonceID := "kandev-runtime:execution-plugin-1:agentctl-bootstrap"
	secretAt := fixture.log.index("secret:create:" + nonceID)
	provisionAt := fixture.log.index("provision")
	require.GreaterOrEqual(t, secretAt, 0, "the nonce was never persisted")
	require.GreaterOrEqual(t, provisionAt, 0, "the provider was never asked to provision")
	require.Less(t, secretAt, provisionAt, "provision ran before the nonce reference was persisted")
	require.Equal(t, nonceID, fixture.checkpoints[0]["_nonce_ref"], "the allocation checkpoint does not carry the nonce reference")
}

func TestDurableBootstrapRecordsHandshakeStartedAndPersistsTheTokenBeforeInstanceCreation(t *testing.T) {
	fixture := newDurableLaunchFixture(t, durableLaunchProvider())
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()

	tokenID := "kandev-runtime:execution-plugin-1:agentctl-auth"
	require.Less(t, fixture.log.index("secret:create:"+tokenID), fixture.log.index("create_instance"))
	require.Less(t, fixture.log.index("checkpoint"), fixture.log.index("handshake"))
	started := false
	for _, checkpoint := range fixture.checkpoints {
		inventory, err := decodePluginExecutorInventory(checkpoint)
		require.NoError(t, err)
		started = started || inventory.HandshakeStarted
	}
	require.True(t, started, "handshake_started was never recorded")
	require.Equal(t, tokenID, fixture.request.Metadata[MetadataKeyAuthTokenSecret])
	require.NotContains(t, instance.Metadata, MetadataKeyBootstrapNonceSecret, "a spent nonce reference must not follow the instance")
	require.NotContains(t, fixture.secrets.values, "kandev-runtime:execution-plugin-1:agentctl-bootstrap", "the spent nonce was not deleted")
}

func TestDurableBootstrapSecretsNeverReachTheEnvelopeOrLogs(t *testing.T) {
	fixture := newDurableLaunchFixture(t, durableLaunchProvider())
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	fixture.operations.provisionRequest.GetBootstrapJson()

	var seen strings.Builder
	for _, checkpoint := range fixture.checkpoints {
		encoded, _ := json.Marshal(checkpoint)
		seen.Write(encoded)
	}
	for _, entry := range fixture.logs.All() {
		seen.WriteString(entry.Message)
		fmt.Fprint(&seen, entry.ContextMap())
	}
	require.NotContains(t, seen.String(), durableToken)
	for _, value := range fixture.secrets.values {
		require.NotContains(t, seen.String(), value)
	}
	for id, value := range fixture.secrets.values {
		_ = id
		require.NotEmpty(t, value)
	}
}

func TestProviderWithoutDurableBootstrapPersistsNoRuntimeSecrets(t *testing.T) {
	fixture := newDurableLaunchFixture(t, testPluginExecutorLaunchProvider())
	instance, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.NoError(t, err)
	defer instance.Client.Close()
	require.Empty(t, fixture.secrets.values)
	require.NotContains(t, fixture.request.Metadata, MetadataKeyBootstrapNonceSecret)
	require.NotContains(t, fixture.request.Metadata, MetadataKeyAuthTokenSecret)
}

func TestDurableBootstrapRefusesToProvisionWithoutASecretStore(t *testing.T) {
	fixture := newDurableLaunchFixture(t, durableLaunchProvider())
	fixture.runtime.SetRuntimeSecretStore(nil)
	_, err := fixture.runtime.CreateInstance(context.Background(), fixture.request)
	require.Error(t, err)
	require.Equal(t, -1, fixture.log.index("provision"), "a provider was called although the nonce could not be stored")
}

type publisherStoreFake struct {
	pluginExecutorInventoryStoreFake
	published  []models.PublishRecoveredTaskEnvironmentParams
	publishErr error
}

func (f *publisherStoreFake) PublishRecoveredTaskEnvironment(_ context.Context, params models.PublishRecoveredTaskEnvironmentParams) (bool, error) {
	f.published = append(f.published, params)
	return f.publishErr == nil, f.publishErr
}

type durableRecoveryFixture struct {
	runtime    *PluginRemoteExecutor
	operations *pluginExecutorOperationsFake
	store      *publisherStoreFake
	secrets    *runtimeSecretsFake
	log        *eventLog
	handshake  func() error
}

func newDurableRecoveryFixture(t *testing.T, phase string) *durableRecoveryFixture {
	t.Helper()
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-durable", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	record := pluginExecutorRecoveryRecord(t, phase, resource)
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	require.NoError(t, err)
	inventory.NegotiatedCapabilities = []string{"durable_bootstrap@1"}
	inventory.InstancePort = 0
	record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	record.TransientAuthToken = ""
	record.TransientBootstrapNonce = durableNonce
	fixture := &durableRecoveryFixture{log: &eventLog{}, handshake: func() error { return nil }}
	fixture.operations = &pluginExecutorOperationsFake{
		destroyResponse: &pluginsdk.DestroyExecutorEnvironmentResponse{ConfirmedAbsent: true},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: "2099-01-01T00:00:00Z", Generation: "lease-1",
		}},
	}
	loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: "profile-plugin-recovery",
	}}
	fixture.store = &publisherStoreFake{pluginExecutorInventoryStoreFake: pluginExecutorInventoryStoreFake{
		record:  record,
		session: &models.TaskSession{ID: "session-plugin-recovery", TaskID: "task-plugin-recovery", State: models.TaskSessionStateRunning},
	}}
	fixture.secrets = newRuntimeSecretsFake(fixture.log)
	fixture.runtime = NewPluginRemoteExecutor(fixture.operations, newTestLogger())
	fixture.runtime.SetRecoveryDependencies(loader, fixture.store)
	fixture.runtime.SetRuntimeSecretStore(fixture.secrets)
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			http.NotFound(w, r)
		default:
			var instance agentctl.CreateInstanceRequest
			_ = json.NewDecoder(r.Body).Decode(&instance)
			fixture.log.add("create_instance")
			_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: instance.ID, Port: testPluginExecutorInstancePort})
		}
	}))
	t.Cleanup(control.Close)
	controlURL, _ := url.Parse(control.URL)
	controlPort, _ := strconv.Atoi(controlURL.Port())
	fixture.runtime.newAgentctlControlClient = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, log *logger.Logger, nonce string) (*agentctl.ControlClient, string, error) {
		fixture.log.add("handshake:" + nonce)
		if err := fixture.handshake(); err != nil {
			return nil, "", err
		}
		return agentctl.NewControlClient(controlURL.Hostname(), controlPort, log), durableToken, nil
	}
	fixture.runtime.newRecoveredAgentctlClient = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		return agentctl.NewClient("unused", 0, log, agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
	}
	fixture.runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	fixture.runtime.recoveredAgentctlInfoReader = func(_ context.Context, _ agentctl.ConnectionLeaseResolver, _ *logger.Logger, executionID, _ string) (*agentctl.InstanceInfo, error) {
		return &agentctl.InstanceInfo{
			ID: executionID, TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
			Port: testPluginExecutorInstancePort, WorkspacePath: pluginExecutorWorkspacePath,
		}, nil
	}
	return fixture
}

func (f *durableRecoveryFixture) recover() ([]*ExecutorInstance, error) {
	return f.runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{f.store.record})
}

func TestRepeatedHandshakeAcceptedContinuesToReadinessAndPublishes(t *testing.T) {
	for _, phase := range []string{"bootstrapping", "provisioned"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newDurableRecoveryFixture(t, phase)
			instances, err := fixture.recover()
			require.NoError(t, err)
			require.Len(t, instances, 1)
			defer instances[0].Client.Close()

			require.Nil(t, fixture.operations.provisionRequest, "recovery allocated a second environment")
			require.Equal(t, pluginExecutorClaimOperationID("operation-plugin-recovery", models.PluginClaimKindRecovery), fixture.store.claimRequest.OperationID)
			require.Equal(t, 1, fixture.store.releaseCount, "the recovery claim was not released")
			require.Len(t, fixture.store.published, 1)
			published := fixture.store.published[0]
			require.Equal(t, "operation-plugin-recovery", published.OperationID)
			require.Equal(t, "session-plugin-recovery", published.SessionID)
			require.Equal(t, int64(7), published.OwnershipGeneration)
			require.Same(t, fixture.store.claim, published.Claim)
			persisted, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
			require.NoError(t, err)
			require.Equal(t, "ready", persisted.Phase)
			require.True(t, persisted.HandshakeStarted)
			require.False(t, persisted.PublishPending, "the publication marker outlived the publication")
			require.Equal(t, persisted.Revision, published.ExpectedRevision+1, "only the marker clearing follows the published revision")
			require.Equal(t, durableToken, instances[0].AuthToken)
			require.NotContains(t, instances[0].Metadata, MetadataKeyBootstrapNonceSecret)
			require.NotContains(t, fixture.secrets.values, "kandev-runtime:execution-plugin-recovery:agentctl-bootstrap")
		})
	}
}

func TestRepeatedHandshakeRejectedRunsBootstrapConsumedCleanup(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	fixture.handshake = func() error { return fmt.Errorf("%w: spent", agentctl.ErrHandshakeRejected) }
	instances, err := fixture.recover()
	require.NoError(t, err)
	require.Empty(t, instances)
	require.NotNil(t, fixture.operations.destroyRequest)
	require.Equal(t, "bootstrap_consumed", fixture.operations.destroyRequest.GetCleanupReason())
	require.Nil(t, fixture.operations.provisionRequest, "a replacement was allocated")
	require.Empty(t, fixture.store.published, "ready was published for a spent nonce")
	require.Equal(t, 2, fixture.store.releaseCount, "the recovery claim must be released before the cleanup claim is taken")
	require.Equal(t, pluginExecutorClaimOperationID("operation-plugin-recovery", models.PluginClaimKindCleanup), fixture.store.claimRequest.OperationID)
}

func TestCanceledOrUnreachableRecoveryStaysCreatingAndRetryable(t *testing.T) {
	for name, handshakeErr := range map[string]error{
		"canceled":  context.Canceled,
		"transport": errors.New("connection refused"),
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDurableRecoveryFixture(t, "provisioned")
			fixture.handshake = func() error { return handshakeErr }
			instances, _ := fixture.recover()
			require.Empty(t, instances)
			require.Nil(t, fixture.operations.destroyRequest, "a retryable failure destroyed the environment")
			require.Empty(t, fixture.store.published)
			require.False(t, fixture.store.deleted)
			persisted, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
			require.NoError(t, err)
			require.Equal(t, "provisioned", persisted.Phase)

			fixture.handshake = func() error { return nil }
			fixture.store.record.TransientBootstrapNonce = durableNonce
			instances, err = fixture.recover()
			require.NoError(t, err)
			require.Len(t, instances, 1, "the retry did not complete")
			instances[0].Client.Close()
		})
	}
}

func TestProviderWithoutDurableBootstrapKeepsTheIncompleteBootstrapCleanup(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	inventory.NegotiatedCapabilities = nil
	fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	_, err = fixture.recover()
	require.NoError(t, err)
	require.NotNil(t, fixture.operations.destroyRequest)
	require.Equal(t, "incomplete_bootstrap", fixture.operations.destroyRequest.GetCleanupReason())
	require.Equal(t, -1, fixture.log.index("handshake:"+durableNonce))
}

func TestMismatchedPublicationAdoptsNothing(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "provisioned")
	fixture.store.publishErr = &models.PluginPublishMismatchError{Reason: models.PluginPublishReasonWorkspace}
	instance, err := fixture.runtime.recoverPluginExecutor(context.Background(), fixture.store.record)
	require.ErrorIs(t, err, models.ErrExecutorPublishMismatch)
	require.Nil(t, instance)
	require.Nil(t, fixture.operations.destroyRequest, "the environment must be retained")
}

type confirmingOperations struct{ *pluginExecutorOperationsFake }

func (confirmingOperations) ConfirmExecutorProviderCapabilities(context.Context, string, string, models.ExecutorProviderProfile) error {
	return nil
}

func TestRetainedAttachDoesNotCopyTheNonceReferenceToAnotherSession(t *testing.T) {
	fixture := newDurableRecoveryFixture(t, "ready")
	inventory, err := decodePluginExecutorInventory(fixture.store.record.Metadata)
	require.NoError(t, err)
	inventory.InstancePort = testPluginExecutorInstancePort
	fixture.store.record.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	fixture.store.record.TransientAuthToken = "agentctl-secret"
	fixture.store.record.TransientBootstrapNonce = ""
	fixture.operations.attachResponse = &pluginsdk.AttachExecutorEnvironmentResponse{Resource: inventory.Resource}
	fixture.operations.inspectResponses = []*pluginsdk.InspectExecutorEnvironmentResponse{{State: "running"}, {State: "running"}}
	fixture.runtime.operations = confirmingOperations{fixture.operations}
	fixture.runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	fixture.runtime.newRecoveredAgentctlClient = func(context.Context, agentctl.ConnectionLeaseResolver, *logger.Logger, string, string) (*agentctl.Client, error) {
		return agentctl.NewClient("127.0.0.1", testPluginExecutorInstancePort, newTestLogger()), nil
	}
	instance, err := fixture.runtime.CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-next", TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
		TaskEnvironmentID: inventory.EnvironmentID, PreviousExecutionID: fixture.store.record.AgentExecutionID,
		WorkspaceReuseRequired: true,
		Metadata:               map[string]interface{}{MetadataKeyBootstrapNonceSecret: "kandev-runtime:prior:agentctl-bootstrap"},
	})
	require.NoError(t, err)
	defer instance.Client.Close()
	require.NotContains(t, instance.Metadata, MetadataKeyBootstrapNonceSecret)
}

func TestCleanupThatLosesTheClaimRaceIsRecordedAndRunAfterRelease(t *testing.T) {
	resource := &pluginsdk.ExecutorResourceDescriptor{ResourceHandle: "resource-race", StateJson: `{"resource":"one"}`, Platform: "linux-amd64", StateVersion: 1}
	runtime, operations, store := claimSettlementRuntime(t, "provisioned", nil, resource)
	store.claimErr = recoveryclaim.ErrBusy
	store.checkpointErr = recoveryclaim.ErrBusy

	err := destroyClaimEnvironment(runtime)
	require.Error(t, err)
	require.Nil(t, operations.destroyRequest, "the lost race destroyed anyway")
	inventory, decodeErr := decodePluginExecutorInventory(store.record.Metadata)
	require.NoError(t, decodeErr)
	require.True(t, inventory.CleanupRequested, "the lost cleanup was not recorded")
	require.Equal(t, "provisioned", inventory.Phase)

	store.claimErr = nil
	store.checkpointErr = nil
	store.record.TransientAuthToken = ""
	instances, err := runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{store.record})
	require.NoError(t, err)
	require.Empty(t, instances)
	require.NotNil(t, operations.destroyRequest, "the sweep did not run the requested cleanup")
}
