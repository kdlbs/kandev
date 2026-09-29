package lifecycle

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type pluginExecutorManagerInventoryStore struct {
	*pluginExecutorInventoryStoreFake
	upserts int
}

func (s *pluginExecutorManagerInventoryStore) UpsertExecutorRunning(_ context.Context, running *models.ExecutorRunning) error {
	copy := *running
	copy.Metadata = clonePluginExecutorMetadata(running.Metadata)
	s.record = &copy
	s.upserts++
	return nil
}

func (s *pluginExecutorManagerInventoryStore) UpdateExecutorRunningStatusIfCurrent(
	_ context.Context, sessionID, executionID, status string,
) error {
	if s.record == nil || s.record.SessionID != sessionID || s.record.AgentExecutionID != executionID {
		return models.ErrExecutionRotated
	}
	s.record.Status = status
	return nil
}

func (s *pluginExecutorManagerInventoryStore) DeleteExecutorRunningBySessionID(_ context.Context, sessionID string) error {
	if s.record != nil && s.record.SessionID == sessionID {
		s.record = nil
	}
	return nil
}

func (s *pluginExecutorManagerInventoryStore) RepairExecutorRunningDead(context.Context, string) error {
	return nil
}

func TestManagerReopensStoppedPluginExecutorByAttachingRetainedEnvironment(t *testing.T) {
	const (
		sessionID        = "session-plugin-reopen"
		taskID           = "task-plugin-recovery"
		environmentID    = "environment-plugin-recovery"
		priorExecutionID = "execution-plugin-reopen-prior"
		runtimeIdentity  = "agentctl-original-runtime"
		authSecretID     = "plugin-reopen-auth-secret"
		profileID        = "profile-plugin-recovery"
	)
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-plugin-reopen", StateJson: `{"resource":"retained"}`, Platform: "linux-amd64", StateVersion: 1,
	}
	record := pluginExecutorRecoveryRecord(t, "ready", resource)
	record.ID = sessionID
	record.SessionID = sessionID
	record.TaskID = taskID
	record.AgentExecutionID = priorExecutionID
	record.Status = models.ExecutorRunningStatusStopped
	record.Metadata[MetadataKeyAuthTokenSecret] = authSecretID
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	inventory.EnvironmentID = environmentID
	inventory.ProfileID = profileID
	inventory.RuntimeIdentity = runtimeIdentity
	record.Metadata[MetadataKeyPluginExecutor] = inventory
	store := &pluginExecutorManagerInventoryStore{pluginExecutorInventoryStoreFake: &pluginExecutorInventoryStoreFake{
		record:  record,
		session: &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateWaitingForInput},
	}}
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		sessionID: {
			TaskID: taskID, SessionID: sessionID, TaskEnvironmentID: environmentID,
			WorkspacePath: "/workspace", AgentID: "auggie", ExecutorType: string(models.ExecutorTypePluginRemote),
			AgentExecutionID: priorExecutionID, ExecutionProfileID: record.ExecutionProfileID,
			Metadata: clonePluginExecutorMetadata(record.Metadata),
		},
	}}

	log := newTestLogger()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agentctl-secret" || r.Header.Get("X-Instance-ID") != runtimeIdentity {
			http.Error(w, "invalid retained runtime identity", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portValue, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		t.Fatal(err)
	}

	operations := &pluginExecutorOperationsFake{
		attachResponse: &pluginsdk.AttachExecutorEnvironmentResponse{Resource: resource},
		inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{
			{State: "running"}, {State: "running"},
			{State: "running"}, {State: "running"},
		},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: "https://executor.example", ExpiresAt: "2027-01-01T00:00:00Z", Generation: "lease-reopen",
		}},
	}
	runtime := NewPluginRemoteExecutor(operations, log)
	runtime.SetRecoveryDependencies(&pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
		Provider: testPluginExecutorLaunchProvider(), ProfileID: profileID,
	}}, store)
	runtime.newRecoveredAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		if executionID != runtimeIdentity || token != "agentctl-secret" {
			return nil, fmt.Errorf("runtime identity=%q token=%q", executionID, token)
		}
		if _, err := resolver(ctx); err != nil {
			return nil, err
		}
		return agentctl.NewClient(host, port, log, agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
	}
	runtime.ready = func(ctx context.Context, client *agentctl.Client) error { return client.Health(ctx) }

	registry := NewExecutorRegistry(log)
	registry.Register(runtime)
	mgr := NewManager(newTestRegistry(), &MockEventBusWithTracking{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	mgr.workspaceInfoProvider = provider
	mgr.SetExecutorRunningWriter(store)
	secretStore := newInMemorySecretStore()
	secretStore.store[authSecretID] = &secrets.SecretWithValue{Value: "agentctl-secret"}
	mgr.secretStore = secretStore
	cleanupManagerStopCh(t, mgr)

	execution, err := mgr.GetOrEnsureExecution(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetOrEnsureExecution after stop: %v", err)
	}
	if execution.ID == priorExecutionID || store.record.AgentExecutionID != execution.ID {
		t.Fatalf("manager execution id=%q persisted id=%q prior=%q", execution.ID, store.record.AgentExecutionID, priorExecutionID)
	}
	if operations.provisionRequest != nil || operations.attachRequest == nil || operations.attachRequest.GetExpectedRuntimeIdentity() != runtimeIdentity {
		t.Fatalf("normal reopen provision=%#v attach=%#v", operations.provisionRequest, operations.attachRequest)
	}
	if store.record.ResumeToken != "resume-preserved" || store.record.LastMessageUUID != "message-preserved" {
		t.Fatalf("normal reopen changed conversation state: %+v", store.record)
	}

	if err := mgr.StopAgentWithReason(context.Background(), execution.ID, "user stop", false); err != nil {
		t.Fatalf("StopAgentWithReason(): %v", err)
	}
	if operations.destroyRequest != nil {
		t.Fatalf("ordinary stop destroyed retained provider resource: %#v", operations.destroyRequest)
	}
	info := provider.infos[sessionID]
	info.AgentExecutionID = store.record.AgentExecutionID
	info.Metadata = clonePluginExecutorMetadata(store.record.Metadata)
	execution, err = mgr.GetOrEnsureExecution(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetOrEnsureExecution after second stop: %v", err)
	}
	if execution.ID == priorExecutionID || operations.attachRequest.GetExpectedRuntimeIdentity() != runtimeIdentity ||
		len(operations.inspectRequests) != 4 || operations.provisionRequest != nil || operations.destroyRequest != nil {
		t.Fatalf("second reopen execution=%q attach=%#v inspections=%d provision=%#v destroy=%#v",
			execution.ID, operations.attachRequest, len(operations.inspectRequests), operations.provisionRequest, operations.destroyRequest)
	}
	if got := execution.MetadataSnapshot()[MetadataKeyPluginExecutor]; got == nil {
		t.Fatal("reopened execution lost plugin inventory metadata")
	}
	if runtime.Name() != executor.NamePluginRemote {
		t.Fatalf("registered runtime = %q", runtime.Name())
	}
}
