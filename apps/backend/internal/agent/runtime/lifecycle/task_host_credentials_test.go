package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

func TestLocalRuntimeSecretRotationUsesStableInternalExecutionOwner(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	manager := &Manager{logger: log, runtimeSecretStore: store}
	execution := &AgentExecution{
		ID: "exec-local-1", RuntimeName: agentruntime.RuntimeStandalone, metadata: make(map[string]interface{}),
	}
	for _, token := range []string{"token-1", "token-2", "token-3"} {
		if err := manager.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
			InstanceID: execution.ID, AuthToken: token, BootstrapNonce: "nonce",
		}, execution); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.store) != 2 {
		t.Fatalf("rotated local runtime secret count = %d, want 2", len(store.store))
	}
	for id := range store.store {
		if !strings.HasPrefix(id, "runtime:execution:exec-local-1:") || !secrets.IsInternalID(id) {
			t.Fatalf("local runtime secret ID = %q", id)
		}
	}
	if got := manager.revealRuntimeSecret(
		context.Background(), execution.MetadataSnapshot(), MetadataKeyAuthTokenSecret,
	); got != "token-3" {
		t.Fatalf("rotated token = %q, want token-3", got)
	}
}

func TestPersistDockerRuntimeSecretsDeletesSupersededLegacyIDs(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	writer := &captureTaskEnvironmentRuntimeSecretWriter{}
	manager := &Manager{
		logger: log, runtimeSecretStore: store, taskEnvironmentRuntimeSecretWriter: writer,
	}
	for id, value := range map[string]string{
		"legacy-auth-id": "old-auth", "legacy-nonce-id": "old-nonce",
		"legacy-container-control-id": "old-container-control",
	} {
		store.store[id] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: id}, Value: value}
	}
	execution := &AgentExecution{
		ID: "exec-docker-1", RuntimeName: agentruntime.RuntimeDocker, TaskEnvironmentID: "env-1",
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:            "legacy-auth-id",
			MetadataKeyBootstrapNonceSecret:       "legacy-nonce-id",
			MetadataKeyContainerControlAuthSecret: "legacy-container-control-id",
		},
	}
	if err := manager.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
		InstanceID: execution.ID, ContainerID: "container-1",
		AuthToken: "new-auth", BootstrapNonce: "new-nonce",
	}, execution); err != nil {
		t.Fatal(err)
	}
	if len(store.store) != 2 {
		t.Fatalf("secrets after migration = %v, want two deterministic entries", store.store)
	}
	if _, exists := store.store["legacy-auth-id"]; exists {
		t.Fatal("legacy auth secret survived migration")
	}
	if _, exists := store.store["legacy-nonce-id"]; exists {
		t.Fatal("legacy nonce secret survived migration")
	}
	if _, exists := store.store["legacy-container-control-id"]; exists {
		t.Fatal("legacy container control secret survived migration")
	}
}

func TestPersistDockerRuntimeSecretsMigratesEveryLiveEnvironmentMember(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	manager := &Manager{
		logger: log, runtimeSecretStore: store, executionStore: NewExecutionStore(),
		taskEnvironmentRuntimeSecretWriter: &captureTaskEnvironmentRuntimeSecretWriter{},
	}
	var source *AgentExecution
	for index, entry := range []struct {
		id, sessionID string
		taskHost      bool
	}{
		{id: "source", sessionID: "session-source"},
		{id: "sibling", sessionID: "session-sibling"},
		{id: "task-host", sessionID: taskHostRuntimeSessionPrefix + "env-1", taskHost: true},
	} {
		authID := fmt.Sprintf("legacy-auth-%d", index)
		nonceID := fmt.Sprintf("legacy-nonce-%d", index)
		store.store[authID] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: authID}}
		store.store[nonceID] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: nonceID}}
		execution := &AgentExecution{
			ID: entry.id, SessionID: entry.sessionID, TaskID: "task-1",
			TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, IsTaskHost: entry.taskHost,
			metadata: map[string]interface{}{
				MetadataKeyAuthTokenSecret: authID, MetadataKeyBootstrapNonceSecret: nonceID,
			},
		}
		if err := manager.executionStore.Add(execution); err != nil {
			t.Fatal(err)
		}
		if entry.id == "source" {
			source = execution
		}
	}
	if err := manager.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
		InstanceID: source.ID, AuthToken: "new-auth", BootstrapNonce: "new-nonce",
	}, source); err != nil {
		t.Fatal(err)
	}
	if len(store.store) != 2 {
		t.Fatalf("secrets after live migration = %v, want two deterministic entries", store.store)
	}
	wantAuth := deterministicTaskEnvironmentRuntimeSecretID("env-1", MetadataKeyAuthTokenSecret)
	wantNonce := deterministicTaskEnvironmentRuntimeSecretID("env-1", MetadataKeyBootstrapNonceSecret)
	for _, execution := range manager.executionStore.List() {
		metadata := execution.MetadataSnapshot()
		if getMetadataString(metadata, MetadataKeyAuthTokenSecret) != wantAuth ||
			getMetadataString(metadata, MetadataKeyBootstrapNonceSecret) != wantNonce {
			t.Fatalf("%s migrated metadata = %v", execution.ID, metadata)
		}
	}
}

func TestDeleteExecutionRuntimeSecretsRemovesLegacyAndDeterministicIDs(t *testing.T) {
	store := newInMemorySecretStore()
	execution := &AgentExecution{
		ID: "exec-local-1", RuntimeName: agentruntime.RuntimeStandalone,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "legacy-auth-id",
			MetadataKeyBootstrapNonceSecret: "legacy-nonce-id",
		},
	}
	for _, id := range []string{
		"legacy-auth-id",
		"legacy-nonce-id",
		deterministicRuntimeSecretID(execution, execution.ID, MetadataKeyAuthTokenSecret),
		deterministicRuntimeSecretID(execution, execution.ID, MetadataKeyBootstrapNonceSecret),
	} {
		store.store[id] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: id}}
	}
	store.store["user-visible"] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: "user-visible"}}
	manager := &Manager{runtimeSecretStore: store}
	if err := manager.deleteExecutionRuntimeSecrets(context.Background(), execution); err != nil {
		t.Fatal(err)
	}
	if len(store.store) != 1 || store.store["user-visible"] == nil {
		t.Fatalf("remaining secrets = %v, want only user-visible", store.store)
	}
}

func TestRuntimeSecretsCannotResolveAsUserCredentials(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	raw := newInMemorySecretStore()
	m := &Manager{
		logger:             log,
		secretStore:        secrets.NewUserVisibleStore(raw),
		runtimeSecretStore: raw,
	}
	execution := &AgentExecution{
		RuntimeName: agentruntime.RuntimeDocker, TaskEnvironmentID: "env-1",
		metadata: make(map[string]interface{}),
	}
	if err := m.persistAuthToken(context.Background(), &ExecutorInstance{
		InstanceID: "exec-1", AuthToken: "agentctl-token",
	}, execution); err != nil {
		t.Fatal(err)
	}
	secretID, _ := execution.metadataValue(MetadataKeyAuthTokenSecret)
	if _, err := m.revealGlobalSecret(context.Background(), secretID.(string)); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("runtime secret resolved through user store: %v", err)
	}
	if got := m.revealRuntimeSecret(
		context.Background(), execution.MetadataSnapshot(), MetadataKeyAuthTokenSecret,
	); got != "agentctl-token" {
		t.Fatalf("runtime secret = %q, want agentctl-token", got)
	}
}

func TestPersistRuntimeSecretsPropagatesRotatedDockerTokenAcrossTaskEnvironment(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	writer := &captureExecutorRunningWriter{}
	m := &Manager{
		logger: log, runtimeSecretStore: store, executionStore: NewExecutionStore(), runningWriter: writer,
		taskEnvironmentRuntimeSecretWriter: &captureTaskEnvironmentRuntimeSecretWriter{},
	}

	sessionClient := agentctl.NewClient("127.0.0.1", 41001, log, agentctl.WithAuthToken("stale-token"))
	session := &AgentExecution{
		ID: "session-execution", TaskID: "task-1", SessionID: "session-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, agentctl: sessionClient,
	}
	hostClient := agentctl.NewClient("127.0.0.1", 41002, log, agentctl.WithAuthToken("stale-token"))
	host := &AgentExecution{
		ID: "task-host", TaskID: "task-1", SessionID: taskHostRuntimeSessionPrefix + "env-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, IsTaskHost: true, agentctl: hostClient,
	}
	if err := m.executionStore.Add(session); err != nil {
		t.Fatal(err)
	}
	if err := m.executionStore.Add(host); err != nil {
		t.Fatal(err)
	}

	if err := m.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
		InstanceID: "resumed-session", AuthToken: "rotated-token",
	}, session); err != nil {
		t.Fatal(err)
	}

	for name, client := range map[string]*agentctl.Client{"session": sessionClient, "task host": hostClient} {
		if got := client.AuthToken(); got != "rotated-token" {
			t.Errorf("%s auth token = %q, want rotated token", name, got)
		}
	}
	if writer.running == nil {
		t.Fatal("rotated token was not mirrored to durable executor metadata")
	}
	if got := m.revealRuntimeSecret(context.Background(), writer.running.Metadata, MetadataKeyAuthTokenSecret); got != "rotated-token" {
		t.Fatalf("durable auth token = %q, want rotated token", got)
	}
}

func TestPersistRuntimeSecretsPropagatesDockerTokenBeforeDurableWrite(t *testing.T) {
	persistErr := errors.New("durable credential write failed")
	tests := []struct {
		name      string
		configure func(*Manager, *inMemorySecretStore)
	}{
		{
			name: "secret store failure",
			configure: func(_ *Manager, store *inMemorySecretStore) {
				store.err = persistErr
			},
		},
		{
			name: "task environment writer failure",
			configure: func(manager *Manager, _ *inMemorySecretStore) {
				manager.taskEnvironmentRuntimeSecretWriter = errorTaskEnvironmentRuntimeSecretWriter{err: persistErr}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			store := newInMemorySecretStore()
			manager := &Manager{
				logger: log, runtimeSecretStore: store, executionStore: NewExecutionStore(),
				taskEnvironmentRuntimeSecretWriter: &captureTaskEnvironmentRuntimeSecretWriter{},
			}
			tt.configure(manager, store)

			clients := make(map[string]*agentctl.Client)
			var source *AgentExecution
			for _, entry := range []struct {
				id, sessionID string
				isTaskHost    bool
			}{
				{id: "source", sessionID: "session-1"},
				{id: "sibling", sessionID: "session-2"},
				{id: "task-host", sessionID: taskHostRuntimeSessionPrefix + "env-1", isTaskHost: true},
			} {
				client := agentctl.NewClient("127.0.0.1", 41001, log, agentctl.WithAuthToken("stale-token"))
				execution := &AgentExecution{
					ID: entry.id, TaskID: "task-1", SessionID: entry.sessionID,
					TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker,
					IsTaskHost: entry.isTaskHost, agentctl: client,
				}
				if err := manager.executionStore.Add(execution); err != nil {
					t.Fatal(err)
				}
				clients[entry.id] = client
				if entry.id == "source" {
					source = execution
				}
			}

			err := manager.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
				InstanceID: source.ID, AuthToken: "rotated-token",
			}, source)
			if !errors.Is(err, persistErr) {
				t.Fatalf("persist runtime secrets error = %v, want %v", err, persistErr)
			}
			for name, client := range clients {
				if got := client.AuthToken(); got != "rotated-token" {
					t.Errorf("%s auth token = %q, want rotated token", name, got)
				}
			}
		})
	}
}

func TestTaskHostRotatedDockerTokenUpdatesSessionCredentialOwner(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	writer := &captureExecutorRunningWriter{}
	m := &Manager{
		logger: log, runtimeSecretStore: store, executionStore: NewExecutionStore(), runningWriter: writer,
		taskEnvironmentRuntimeSecretWriter: &captureTaskEnvironmentRuntimeSecretWriter{},
	}

	sessionClient := agentctl.NewClient("127.0.0.1", 41001, log, agentctl.WithAuthToken("stale-token"))
	session := &AgentExecution{
		ID: "session-execution", TaskID: "task-1", SessionID: "session-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, agentctl: sessionClient,
	}
	hostClient := agentctl.NewClient("127.0.0.1", 41002, log, agentctl.WithAuthToken("rotated-token"))
	host := &AgentExecution{
		ID: "task-host", TaskID: "task-1", SessionID: taskHostRuntimeSessionPrefix + "env-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, IsTaskHost: true, agentctl: hostClient,
	}
	if err := m.executionStore.Add(session); err != nil {
		t.Fatal(err)
	}
	if err := m.executionStore.Add(host); err != nil {
		t.Fatal(err)
	}

	if err := m.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
		InstanceID: "task-host", AuthToken: "rotated-token",
	}, host); err != nil {
		t.Fatal(err)
	}

	if got := sessionClient.AuthToken(); got != "rotated-token" {
		t.Fatalf("session auth token = %q, want task-host rotation", got)
	}
	if writer.running == nil || writer.running.SessionID != "session-1" {
		t.Fatalf("durable credential owner = %+v, want session-1", writer.running)
	}
	if got := m.revealRuntimeSecret(context.Background(), writer.running.Metadata, MetadataKeyAuthTokenSecret); got != "rotated-token" {
		t.Fatalf("durable auth token = %q, want rotated token", got)
	}
}

type captureTaskEnvironmentRuntimeSecretWriter struct {
	environmentID string
	authSecretID  string
	nonceSecretID string
}

type errorTaskEnvironmentRuntimeSecretWriter struct {
	err error
}

func (w errorTaskEnvironmentRuntimeSecretWriter) UpdateTaskEnvironmentRuntimeSecretRefs(
	context.Context,
	string,
	string,
	string,
) error {
	return w.err
}

func (w *captureTaskEnvironmentRuntimeSecretWriter) UpdateTaskEnvironmentRuntimeSecretRefs(
	_ context.Context,
	environmentID, authSecretID, nonceSecretID string,
) error {
	w.environmentID = environmentID
	w.authSecretID = authSecretID
	w.nonceSecretID = nonceSecretID
	return nil
}

func TestTaskHostPersistsDockerCredentialsToTaskEnvironmentOwner(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	writer := &captureTaskEnvironmentRuntimeSecretWriter{}
	m := &Manager{
		logger: log, runtimeSecretStore: store, executionStore: NewExecutionStore(),
		taskEnvironmentRuntimeSecretWriter: writer,
	}
	host := &AgentExecution{
		ID: "task-host", TaskID: "task-1", SessionID: taskHostRuntimeSessionPrefix + "env-1",
		TaskEnvironmentID: "env-1", RuntimeName: agentruntime.RuntimeDocker, IsTaskHost: true,
	}
	for _, token := range []string{"rotated-token-1", "rotated-token-2", "rotated-token"} {
		if err := m.persistRuntimeSecrets(context.Background(), &ExecutorInstance{
			InstanceID: "task-host", AuthToken: token, BootstrapNonce: "bootstrap-nonce",
		}, host); err != nil {
			t.Fatal(err)
		}
	}

	if writer.environmentID != "env-1" || writer.authSecretID == "" || writer.nonceSecretID == "" {
		t.Fatalf("task-environment credential refs = %#v", writer)
	}
	metadata := host.MetadataSnapshot()
	if got := m.revealRuntimeSecret(context.Background(), metadata, MetadataKeyAuthTokenSecret); got != "rotated-token" {
		t.Fatalf("persisted task-host auth token = %q", got)
	}
	if got := m.revealRuntimeSecret(context.Background(), metadata, MetadataKeyBootstrapNonceSecret); got != "bootstrap-nonce" {
		t.Fatalf("persisted task-host nonce = %q", got)
	}
	if len(store.store) != 2 {
		t.Fatalf("runtime secret count after repeated rotation = %d, want 2 stable entries", len(store.store))
	}
	for id := range store.store {
		if !secrets.IsInternalID(id) {
			t.Fatalf("runtime credential %q is user-visible", id)
		}
	}
}

func TestDeleteTaskEnvironmentRuntimeSecretsRemovesOnlyDeterministicCredentials(t *testing.T) {
	store := newInMemorySecretStore()
	authSecretID := "legacy-task-environment-auth"
	bootstrapSecretID := "legacy-task-environment-nonce"
	for id, value := range map[string]string{
		authSecretID:      "legacy-auth",
		bootstrapSecretID: "legacy-nonce",
		deterministicTaskEnvironmentRuntimeSecretID("env-1", MetadataKeyAuthTokenSecret):      "auth",
		deterministicTaskEnvironmentRuntimeSecretID("env-1", MetadataKeyBootstrapNonceSecret): "nonce",
		"user-visible": "keep",
	} {
		store.store[id] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: id}, Value: value}
	}
	manager := &Manager{runtimeSecretStore: store}

	if err := manager.DeleteTaskEnvironmentRuntimeSecrets(
		context.Background(), "env-1", authSecretID, bootstrapSecretID,
	); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.store["user-visible"]; !exists || len(store.store) != 1 {
		t.Fatalf("remaining secrets = %v, want only user-visible", store.store)
	}
	if err := manager.DeleteTaskEnvironmentRuntimeSecrets(context.Background(), "env-1", "", ""); err != nil {
		t.Fatalf("idempotent cleanup: %v", err)
	}
}
