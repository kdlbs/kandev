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
	"github.com/kandev/kandev/internal/task/models"
)

// inMemorySecretStore implements secrets.SecretStore for testing.
type inMemorySecretStore struct {
	store     map[string]*secrets.SecretWithValue
	err       error
	revealErr error
}

var _ secrets.SecretStore = (*inMemorySecretStore)(nil)
var _ secrets.ScopedSecretStore = (*inMemorySecretStore)(nil)

// newInMemorySecretStore returns an empty in-memory secret store for testing.
func newInMemorySecretStore() *inMemorySecretStore {
	return &inMemorySecretStore{store: make(map[string]*secrets.SecretWithValue)}
}

// Create stores the secret, returning the injected error when set.
func (s *inMemorySecretStore) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	if s.err != nil {
		return s.err
	}
	if secret.ID == "" {
		secret.ID = fmt.Sprintf("secret-%d", len(s.store)+1)
	}
	s.store[secret.ID] = secret
	return nil
}

// Get returns the stored secret for the given ID, or an error when absent.
func (s *inMemorySecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	if sw, ok := s.store[id]; ok {
		return &sw.Secret, nil
	}
	return nil, fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
}

// Reveal returns the plaintext value for the given ID, or an error when absent.
func (s *inMemorySecretStore) Reveal(_ context.Context, id string) (string, error) {
	if s.revealErr != nil {
		return "", s.revealErr
	}
	if sw, ok := s.store[id]; ok {
		return sw.Value, nil
	}
	return "", fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
}

func (s *inMemorySecretStore) Update(_ context.Context, id string, req *secrets.UpdateSecretRequest) error {
	if s.err != nil {
		return s.err
	}
	stored := s.store[id]
	if stored == nil {
		return fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	if req.Name != nil {
		stored.Name = *req.Name
	}
	if req.Value != nil {
		stored.Value = *req.Value
	}
	return nil
}
func (s *inMemorySecretStore) Delete(_ context.Context, id string) error {
	if _, ok := s.store[id]; !ok {
		return fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	delete(s.store, id)
	return nil
}

// List returns no items for the in-memory store.
func (s *inMemorySecretStore) List(_ context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}

// Close is a no-op for the in-memory store.
func (s *inMemorySecretStore) Close() error { return nil }

// ListScoped returns stored secrets filtered by the requested scope and workspace.
func (s *inMemorySecretStore) ListScoped(_ context.Context, opts secrets.SecretListOptions) ([]*secrets.SecretListItem, error) {
	items := make([]*secrets.SecretListItem, 0, len(s.store))
	for _, stored := range s.store {
		scope := stored.Scope
		if scope == "" {
			scope = secrets.ScopeGlobal
		}
		if opts.Scope == secrets.ScopeWorkspace && scope != secrets.ScopeWorkspace {
			continue
		}
		if opts.Scope == secrets.ScopeGlobal && scope != secrets.ScopeGlobal {
			continue
		}
		if scope == secrets.ScopeWorkspace && stored.WorkspaceID != opts.WorkspaceID {
			continue
		}
		items = append(items, &secrets.SecretListItem{ID: stored.ID, Name: stored.Name, Scope: scope})
	}
	return items, nil
}

// GetForWorkspace returns the secret when it is global or belongs to the given workspace.
func (s *inMemorySecretStore) GetForWorkspace(_ context.Context, id, workspaceID string) (*secrets.Secret, error) {
	secret, err := s.Get(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if secret.Scope == secrets.ScopeWorkspace && secret.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("workspace secret unavailable")
	}
	return secret, nil
}

// RevealGlobal reveals a global secret's value, rejecting workspace-scoped secrets.
func (s *inMemorySecretStore) RevealGlobal(ctx context.Context, id string) (string, error) {
	secret, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if secret.Scope == secrets.ScopeWorkspace {
		return "", fmt.Errorf("workspace secret unavailable")
	}
	return s.Reveal(ctx, id)
}

// RevealForWorkspace reveals a secret's value after confirming workspace access.
func (s *inMemorySecretStore) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return "", err
	}
	return s.Reveal(ctx, id)
}

// DeleteWorkspaceSecrets removes all secrets belonging to the given workspace.
func (s *inMemorySecretStore) DeleteWorkspaceSecrets(_ context.Context, workspaceID string) error {
	for id, stored := range s.store {
		if stored.Scope == secrets.ScopeWorkspace && stored.WorkspaceID == workspaceID {
			delete(s.store, id)
		}
	}
	return nil
}

// TestPersistAuthToken verifies persistAuthToken stores the instance auth token as a secret, records its ID in execution metadata, and no-ops when the token or store is absent.
func TestPersistAuthToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})

	t.Run("stores token and sets metadata", func(t *testing.T) {
		store := newInMemorySecretStore()
		m := &Manager{logger: log, runtimeSecretStore: store}

		instance := &ExecutorInstance{
			InstanceID: "exec-123456789012",
			AuthToken:  "handshake-token-abc",
		}
		execution := &AgentExecution{
			metadata: make(map[string]interface{}),
		}

		if err := m.persistAuthToken(context.Background(), instance, execution); err != nil {
			t.Fatal(err)
		}

		// Verify secret was stored
		if len(store.store) != 1 {
			t.Fatalf("expected 1 secret, got %d", len(store.store))
		}

		// Verify metadata has the secret ID
		raw, _ := execution.metadataValue(MetadataKeyAuthTokenSecret)
		secretID, ok := raw.(string)
		if !ok || secretID == "" {
			t.Fatalf("expected secret ID in metadata, got %v", raw)
		}

		// Verify we can retrieve the token
		revealed, err := store.Reveal(context.Background(), secretID)
		if err != nil {
			t.Fatalf("failed to reveal: %v", err)
		}
		if revealed != "handshake-token-abc" {
			t.Fatalf("expected handshake-token-abc, got %q", revealed)
		}
	})

	t.Run("no-op when auth token is empty", func(t *testing.T) {
		store := newInMemorySecretStore()
		m := &Manager{logger: log, runtimeSecretStore: store}

		instance := &ExecutorInstance{InstanceID: "exec-123456789012"}
		execution := &AgentExecution{metadata: make(map[string]interface{})}

		m.persistAuthToken(context.Background(), instance, execution)

		if len(store.store) != 0 {
			t.Fatal("expected no secrets stored")
		}
		if _, ok := execution.metadataValue(MetadataKeyAuthTokenSecret); ok {
			t.Fatal("expected no metadata key")
		}
	})

	t.Run("no-op when secret store is nil", func(t *testing.T) {
		m := &Manager{logger: log}

		instance := &ExecutorInstance{
			InstanceID: "exec-123456789012",
			AuthToken:  "some-token",
		}
		execution := &AgentExecution{metadata: make(map[string]interface{})}

		// Should not panic
		m.persistAuthToken(context.Background(), instance, execution)
	})

	t.Run("handles nil metadata in execution", func(t *testing.T) {
		store := newInMemorySecretStore()
		m := &Manager{logger: log, runtimeSecretStore: store}

		instance := &ExecutorInstance{
			InstanceID: "exec-123456789012",
			AuthToken:  "token-xyz",
		}
		execution := &AgentExecution{}

		m.persistAuthToken(context.Background(), instance, execution)

		if _, ok := execution.metadataValue(MetadataKeyAuthTokenSecret); !ok {
			t.Fatal("expected secret ID in metadata")
		}
	})
}

// TestPersistRuntimeSecrets verifies persistRuntimeSecrets stores the auth token and bootstrap nonce as secrets and records both IDs so they can be revealed later.
func TestPersistRuntimeSecrets(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	m := &Manager{logger: log, runtimeSecretStore: store}

	instance := &ExecutorInstance{
		InstanceID:     "exec-123456789012",
		ContainerID:    "container-123",
		AuthToken:      "agentctl-token",
		BootstrapNonce: "bootstrap-nonce",
	}
	execution := &AgentExecution{metadata: make(map[string]interface{})}

	if err := m.persistRuntimeSecrets(context.Background(), instance, execution); err != nil {
		t.Fatal(err)
	}

	rawAuth, _ := execution.metadataValue(MetadataKeyAuthTokenSecret)
	authSecretID, ok := rawAuth.(string)
	if !ok || authSecretID == "" {
		t.Fatalf("expected auth token secret ID, got %v", rawAuth)
	}
	rawNonce, _ := execution.metadataValue(MetadataKeyBootstrapNonceSecret)
	nonceSecretID, ok := rawNonce.(string)
	if !ok || nonceSecretID == "" {
		t.Fatalf("expected bootstrap nonce secret ID, got %v", rawNonce)
	}
	rawControl, _ := execution.metadataValue(MetadataKeyContainerControlAuthSecret)
	controlSecretID, ok := rawControl.(string)
	if !ok || controlSecretID == "" {
		t.Fatalf("expected container control secret ID, got %v", rawControl)
	}
	if controlSecretID != authSecretID {
		t.Fatalf("container control secret = %q, want environment auth owner %q", controlSecretID, authSecretID)
	}

	if got := m.revealRuntimeSecret(context.Background(), execution.MetadataSnapshot(), MetadataKeyAuthTokenSecret); got != "agentctl-token" {
		t.Fatalf("revealed auth token = %q, want agentctl-token", got)
	}
	if got := m.revealRuntimeSecret(context.Background(), execution.MetadataSnapshot(), MetadataKeyBootstrapNonceSecret); got != "bootstrap-nonce" {
		t.Fatalf("revealed bootstrap nonce = %q, want bootstrap-nonce", got)
	}
	store.store[authSecretID].Value = "session-auth-token"
	store.store[controlSecretID].Value = "environment-control-token"
	if got, err := m.revealContainerControlAuthToken(context.Background(), execution.MetadataSnapshot(), false); err != nil || got != "environment-control-token" {
		t.Fatalf("revealed container control token = %q, want environment-control-token", got)
	}
	if len(store.store) != 2 {
		t.Fatalf("runtime secret count = %d, want 2", len(store.store))
	}
	for id := range store.store {
		if !secrets.IsInternalID(id) {
			t.Fatalf("runtime secret %q is not internally owned", id)
		}
	}
}

func TestRevealContainerControlAuthToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	store.store["session-auth"] = &secrets.SecretWithValue{Value: "legacy-session-token"}
	m := &Manager{logger: log, runtimeSecretStore: store}

	t.Run("uses the session token only when the environment control handle is absent", func(t *testing.T) {
		got, err := m.revealContainerControlAuthToken(context.Background(), map[string]interface{}{
			MetadataKeyAuthTokenSecret: "session-auth",
		}, true)
		if err != nil || got != "legacy-session-token" {
			t.Fatalf("revealContainerControlAuthToken() = %q, %v", got, err)
		}
	})

	t.Run("does not fall back when a configured environment handle cannot be revealed", func(t *testing.T) {
		got, err := m.revealContainerControlAuthToken(context.Background(), map[string]interface{}{
			MetadataKeyContainerControlAuthSecret: "missing-control-token",
			MetadataKeyAuthTokenSecret:            "session-auth",
		}, true)
		if err == nil {
			t.Fatal("revealContainerControlAuthToken() succeeded with an unreadable environment handle")
		}
		if got != "" {
			t.Fatalf("revealContainerControlAuthToken() token = %q, want empty", got)
		}
	})
}

// TestResolveLaunchAuthToken is the regression test for the SSH resume
// preflight failing with "missing agentctl auth token": #2843 made every
// launch/resume consume the Docker-only container control token, so a
// non-Docker executor (SSH) with a real session auth secret got "" instead.
func TestResolveLaunchAuthToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})

	t.Run("ssh with a session auth secret returns that token", func(t *testing.T) {
		store := newInMemorySecretStore()
		store.store["session-auth"] = &secrets.SecretWithValue{Value: "the-ssh-token"}
		m := &Manager{logger: log, runtimeSecretStore: store}

		req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH)}
		metadata := map[string]interface{}{MetadataKeyAuthTokenSecret: "session-auth"}

		got, err := m.resolveLaunchAuthToken(context.Background(), req, metadata)
		if err != nil {
			t.Fatalf("resolveLaunchAuthToken() error = %v", err)
		}
		if got != "the-ssh-token" {
			t.Fatalf("resolveLaunchAuthToken() = %q, want %q", got, "the-ssh-token")
		}
	})

	t.Run("ssh reports a session secret reveal failure", func(t *testing.T) {
		store := newInMemorySecretStore()
		revealErr := errors.New("secret backend unavailable")
		store.revealErr = revealErr
		store.store["session-auth"] = &secrets.SecretWithValue{Value: "the-ssh-token"}
		m := &Manager{logger: log, runtimeSecretStore: store}

		req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH)}
		metadata := map[string]interface{}{MetadataKeyAuthTokenSecret: "session-auth"}

		got, err := m.resolveLaunchAuthToken(context.Background(), req, metadata)
		if !errors.Is(err, revealErr) {
			t.Fatalf("resolveLaunchAuthToken() error = %v, want %v", err, revealErr)
		}
		if got != "" {
			t.Fatalf("resolveLaunchAuthToken() = %q, want empty", got)
		}
	})

	t.Run("ssh with no secret anywhere returns empty and resume still rejects it", func(t *testing.T) {
		store := newInMemorySecretStore()
		m := &Manager{logger: log, runtimeSecretStore: store}

		req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH)}

		got, err := m.resolveLaunchAuthToken(context.Background(), req, map[string]interface{}{})
		if err != nil {
			t.Fatalf("resolveLaunchAuthToken() error = %v", err)
		}
		if got != "" {
			t.Fatalf("resolveLaunchAuthToken() = %q, want empty", got)
		}
		if err := requireSSHAgentctlAuthToken(got); err == nil {
			t.Fatal("requireSSHAgentctlAuthToken() accepted an empty token")
		}
	})

	t.Run("docker with workspace reuse still prefers the container control token", func(t *testing.T) {
		store := newInMemorySecretStore()
		store.store["control-secret"] = &secrets.SecretWithValue{Value: "environment-control-token"}
		m := &Manager{logger: log, runtimeSecretStore: store}

		req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeLocalDocker), WorkspaceReuseRequired: true}
		metadata := map[string]interface{}{MetadataKeyContainerControlAuthSecret: "control-secret"}

		got, err := m.resolveLaunchAuthToken(context.Background(), req, metadata)
		if err != nil {
			t.Fatalf("resolveLaunchAuthToken() error = %v", err)
		}
		if got != "environment-control-token" {
			t.Fatalf("resolveLaunchAuthToken() = %q, want %q", got, "environment-control-token")
		}
	})

	t.Run("docker with workspace reuse falls back to the session token when no control handle exists", func(t *testing.T) {
		store := newInMemorySecretStore()
		store.store["session-auth"] = &secrets.SecretWithValue{Value: "legacy-session-token"}
		m := &Manager{logger: log, runtimeSecretStore: store}

		req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeRemoteDocker), WorkspaceReuseRequired: true}
		metadata := map[string]interface{}{MetadataKeyAuthTokenSecret: "session-auth"}

		got, err := m.resolveLaunchAuthToken(context.Background(), req, metadata)
		if err != nil {
			t.Fatalf("resolveLaunchAuthToken() error = %v", err)
		}
		if got != "legacy-session-token" {
			t.Fatalf("resolveLaunchAuthToken() = %q, want %q", got, "legacy-session-token")
		}
	})

	t.Run("docker without workspace reuse returns empty when no control handle exists", func(t *testing.T) {
		store := newInMemorySecretStore()
		store.store["session-auth"] = &secrets.SecretWithValue{Value: "legacy-session-token"}
		m := &Manager{logger: log, runtimeSecretStore: store}

		req := &LaunchRequest{ExecutorType: string(models.ExecutorTypeLocalDocker), WorkspaceReuseRequired: false}
		metadata := map[string]interface{}{MetadataKeyAuthTokenSecret: "session-auth"}

		got, err := m.resolveLaunchAuthToken(context.Background(), req, metadata)
		if err != nil {
			t.Fatalf("resolveLaunchAuthToken() error = %v", err)
		}
		if got != "" {
			t.Fatalf("resolveLaunchAuthToken() = %q, want empty", got)
		}
	})
}

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
