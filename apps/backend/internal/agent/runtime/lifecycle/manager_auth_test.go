package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// inMemorySecretStore implements secrets.SecretStore for testing.
type inMemorySecretStore struct {
	mu                  sync.RWMutex
	store               map[string]*secrets.SecretWithValue
	err                 error
	revealErr           error
	rejectCanceledCalls bool
}

type failNthCreateSecretStore struct {
	*inMemorySecretStore
	failAt int
	calls  int
}

func (s *failNthCreateSecretStore) Create(ctx context.Context, secret *secrets.SecretWithValue) error {
	s.calls++
	if s.calls == s.failAt {
		return errors.New("injected secret create failure")
	}
	return s.inMemorySecretStore.Create(ctx, secret)
}

type stopTrackingExecutor struct {
	MockExecutor
	stopCalls int
	forces    []bool
}

func (e *stopTrackingExecutor) StopInstance(_ context.Context, _ *ExecutorInstance, force bool) error {
	e.stopCalls++
	e.forces = append(e.forces, force)
	return nil
}

var _ secrets.SecretStore = (*inMemorySecretStore)(nil)
var _ secrets.ScopedSecretStore = (*inMemorySecretStore)(nil)

// newInMemorySecretStore returns an empty in-memory secret store for testing.
func newInMemorySecretStore() *inMemorySecretStore {
	return &inMemorySecretStore{store: make(map[string]*secrets.SecretWithValue)}
}

// Create stores the secret, returning the injected error when set.
func (s *inMemorySecretStore) Create(ctx context.Context, secret *secrets.SecretWithValue) error {
	if s.rejectCanceledCalls && ctx.Err() != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
func (s *inMemorySecretStore) Get(ctx context.Context, id string) (*secrets.Secret, error) {
	if s.rejectCanceledCalls && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sw, ok := s.store[id]; ok {
		return &sw.Secret, nil
	}
	return nil, fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
}

// Reveal returns the plaintext value for the given ID, or an error when absent.
func (s *inMemorySecretStore) Reveal(ctx context.Context, id string) (string, error) {
	if s.rejectCanceledCalls && ctx.Err() != nil {
		return "", ctx.Err()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.revealErr != nil {
		return "", s.revealErr
	}
	if sw, ok := s.store[id]; ok {
		return sw.Value, nil
	}
	return "", fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
}

// Update changes the stored name/value.
func (s *inMemorySecretStore) Update(ctx context.Context, id string, req *secrets.UpdateSecretRequest) error {
	if s.rejectCanceledCalls && ctx.Err() != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.store[id]
	if !ok {
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

// Delete removes a stored secret.
func (s *inMemorySecretStore) Delete(ctx context.Context, id string) error {
	if s.rejectCanceledCalls && ctx.Err() != nil {
		return ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
		m := &Manager{logger: log, secretStore: store}

		instance := &ExecutorInstance{
			InstanceID: "exec-123456789012",
			AuthToken:  "handshake-token-abc",
		}
		execution := &AgentExecution{
			metadata: make(map[string]interface{}),
		}

		m.persistAuthToken(context.Background(), instance, execution)

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
		m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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
	m := &Manager{logger: log, secretStore: store}

	instance := &ExecutorInstance{
		InstanceID:     "exec-123456789012",
		ContainerID:    "container-123",
		AuthToken:      "agentctl-token",
		BootstrapNonce: "bootstrap-nonce",
	}
	execution := &AgentExecution{metadata: make(map[string]interface{})}

	m.persistRuntimeSecrets(context.Background(), instance, execution)

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
}

func TestRevealContainerControlAuthToken(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	store.store["session-auth"] = &secrets.SecretWithValue{Value: "legacy-session-token"}
	m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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
		m := &Manager{logger: log, secretStore: store}

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

func TestRegisterKubernetesExecutionPersistsRequiredSecretReferencesBeforeSuccess(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	writer := &captureExecutorRunningWriter{}
	mgr := newTestManager(t)
	mgr.SetSecretStore(store)
	mgr.SetExecutorRunningWriter(writer)
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusStarting, agentctl: newReadyAgentctlClient(t, log),
		metadata: map[string]interface{}{},
	}
	instance := &ExecutorInstance{
		InstanceID: "execution-1", AuthToken: "agentctl-token", BootstrapNonce: "bootstrap-nonce",
	}

	err := mgr.registerAndPublishExecution(
		context.Background(), execution,
		&MockExecutor{name: executor.NameKubernetes}, instance, execution.SessionID,
	)
	if err != nil {
		t.Fatalf("registerAndPublishExecution() error = %v", err)
	}
	if writer.running == nil {
		t.Fatal("executors_running row was not persisted")
	}
	for _, key := range []string{MetadataKeyAuthTokenSecret, MetadataKeyBootstrapNonceSecret} {
		secretID := getMetadataString(writer.running.Metadata, key)
		if secretID == "" {
			t.Fatalf("persisted %s = empty", key)
		}
		if !secrets.IsInternalID(secretID) {
			t.Fatalf("persisted %s = %q, want internal runtime secret ID", key, secretID)
		}
	}
}

func TestRegisterNonKubernetesExecutionRollsBackWhenRuntimeSecretPersistenceFails(t *testing.T) {
	store := newInMemorySecretStore()
	store.err = errors.New("secret store unavailable")
	mgr := newTestManager(t)
	mgr.SetSecretStore(store)
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeDocker,
		Status: v1.AgentStatusStarting, metadata: map[string]interface{}{},
	}
	instance := &ExecutorInstance{InstanceID: execution.ID, AuthToken: "agentctl-token"}
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}

	err := mgr.registerAndPublishExecution(
		context.Background(), execution, backend, instance, execution.SessionID,
	)

	require.ErrorContains(t, err, "persist runtime secret")
	require.Equal(t, 1, backend.stopCalls)
	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists)
}

func TestRegisterKubernetesExecutionRollsBackCreatedSecretsOnPersistenceFailure(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := &failNthCreateSecretStore{inMemorySecretStore: newInMemorySecretStore(), failAt: 2}
	mgr := newTestManager(t)
	mgr.SetSecretStore(store)
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{})
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusStarting, agentctl: newReadyAgentctlClient(t, log),
		metadata: map[string]interface{}{},
	}
	instance := &ExecutorInstance{
		InstanceID: "execution-1", AuthToken: "agentctl-token", BootstrapNonce: "bootstrap-nonce",
	}
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}

	err := mgr.registerAndPublishExecution(context.Background(), execution, backend, instance, execution.SessionID)
	if err == nil || !strings.Contains(err.Error(), "bootstrap nonce") {
		t.Fatalf("registerAndPublishExecution() error = %v, want bootstrap nonce persistence failure", err)
	}
	if backend.stopCalls != 1 {
		t.Fatalf("StopInstance() calls = %d, want 1", backend.stopCalls)
	}
	if len(store.store) != 0 {
		t.Fatalf("runtime secrets after rollback = %#v, want none", store.store)
	}
	if _, exists := mgr.executionStore.Get(execution.ID); exists {
		t.Fatal("failed Kubernetes launch remained registered")
	}
}

func TestRegisterKubernetesExecutionRollsBackSecretsWhenDurableRowWriteFails(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	mgr := newTestManager(t)
	mgr.SetSecretStore(store)
	mgr.SetExecutorRunningWriter(&captureExecutorRunningWriter{upsertErr: errors.New("injected row failure")})
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		AgentProfileID: "profile-1", RuntimeName: agentruntime.RuntimeKubernetes,
		Status: v1.AgentStatusStarting, agentctl: newReadyAgentctlClient(t, log),
		metadata: map[string]interface{}{},
	}
	instance := &ExecutorInstance{
		InstanceID: "execution-1", AuthToken: "agentctl-token", BootstrapNonce: "bootstrap-nonce",
	}
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}

	err := mgr.registerAndPublishExecution(context.Background(), execution, backend, instance, execution.SessionID)
	if err == nil || !strings.Contains(err.Error(), "persist execution registration") {
		t.Fatalf("registerAndPublishExecution() error = %v, want row persistence failure", err)
	}
	if backend.stopCalls != 1 {
		t.Fatalf("StopInstance() calls = %d, want 1", backend.stopCalls)
	}
	if len(store.store) != 0 {
		t.Fatalf("runtime secrets after row rollback = %#v, want none", store.store)
	}
}

func TestCreateWorkspaceKubernetesExecutionPersistsRequiredSecretReferencesBeforeSuccess(t *testing.T) {
	log := newTestLogger()
	backend := &createInstanceExecutor{
		MockExecutor: MockExecutor{name: executor.NameKubernetes},
		client:       newReadyAgentctlClient(t, log),
		authToken:    "agentctl-token",
		nonce:        "bootstrap-nonce",
	}
	executors := NewExecutorRegistry(log)
	executors.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, executors, &MockCredentialsManager{},
		&MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	mgr.SetSecretStore(newInMemorySecretStore())
	writer := &captureExecutorRunningWriter{}
	mgr.SetExecutorRunningWriter(writer)

	_, err := mgr.createExecution(context.Background(), "task-1", &WorkspaceInfo{
		SessionID: "session-1", TaskEnvironmentID: "environment-1",
		AgentID: "auggie", AgentProfileID: "profile-1",
		ExecutorType: string(models.ExecutorTypeKubernetes), WorkspacePath: "/workspace",
	})
	if err != nil {
		t.Fatalf("createExecution() error = %v", err)
	}
	for _, key := range []string{MetadataKeyAuthTokenSecret, MetadataKeyBootstrapNonceSecret} {
		if got := getMetadataString(writer.running.Metadata, key); got == "" {
			t.Fatalf("persisted %s = empty", key)
		}
	}
}

func TestStopKubernetesExecutionDeletesRuntimeSecretsAfterTerminalCleanup(t *testing.T) {
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:execution-1:agentctl-auth":      "agentctl-token",
		"kandev-runtime:execution-1:agentctl-bootstrap": "bootstrap-nonce",
	} {
		if err := store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}); err != nil {
			t.Fatalf("seed runtime secret: %v", err)
		}
	}
	log := newTestLogger()
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	executors := NewExecutorRegistry(log)
	executors.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, executors, nil, nil, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	mgr.SetSecretStore(store)
	if err := mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes, Status: v1.AgentStatusRunning,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "kandev-runtime:execution-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret: "kandev-runtime:execution-1:agentctl-bootstrap",
		},
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	err := mgr.StopAgentWithReason(context.Background(), "execution-1", StopReasonTaskDeleted, true)

	if err != nil {
		t.Fatalf("StopAgentWithReason() error = %v", err)
	}
	if len(store.store) != 0 {
		t.Fatalf("runtime secrets after cleanup = %#v, want none", store.store)
	}
}

func TestStopKubernetesExecutionDeletesRuntimeSecretsAfterRequestCancellation(t *testing.T) {
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:execution-1:agentctl-auth":      "agentctl-token",
		"kandev-runtime:execution-1:agentctl-bootstrap": "bootstrap-nonce",
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	store.rejectCanceledCalls = true
	log := newTestLogger()
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	executors := NewExecutorRegistry(log)
	executors.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, executors, nil, nil, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	mgr.SetSecretStore(store)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes, Status: v1.AgentStatusRunning,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "kandev-runtime:execution-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret: "kandev-runtime:execution-1:agentctl-bootstrap",
		},
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.StopAgentWithReason(ctx, "execution-1", StopReasonTaskDeleted, true)

	require.NoError(t, err)
	require.Empty(t, store.store)
}

func TestForceStopKubernetesExecutionDeletesRuntimeSecretsWithoutTerminalReason(t *testing.T) {
	store := newInMemorySecretStore()
	for id, value := range map[string]string{
		"kandev-runtime:execution-1:agentctl-auth":      "agentctl-token",
		"kandev-runtime:execution-1:agentctl-bootstrap": "bootstrap-nonce",
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: value,
		}))
	}
	log := newTestLogger()
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	executors := NewExecutorRegistry(log)
	executors.Register(backend)
	mgr := NewManager(
		newTestRegistry(), &MockEventBus{}, executors, nil, nil, nil,
		ExecutorFallbackWarn, "", log,
	)
	cleanupManagerStopCh(t, mgr)
	mgr.SetSecretStore(store)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes, Status: v1.AgentStatusRunning,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "kandev-runtime:execution-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret: "kandev-runtime:execution-1:agentctl-bootstrap",
		},
	}))

	err := mgr.StopAgent(context.Background(), "execution-1", true)

	require.NoError(t, err)
	require.Empty(t, store.store, "forced resource cleanup must also remove runtime secrets")
}

func TestDeleteKubernetesRuntimeSecretsDerivesCanonicalRefsForProvisionalInventory(t *testing.T) {
	for _, tc := range []struct {
		name        string
		state       string
		wantDeleted bool
	}{
		{name: "crash window", state: KubernetesInventoryStatePodAdmitted, wantDeleted: true},
		{name: "ready row without refs", state: KubernetesInventoryStateReady, wantDeleted: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newInMemorySecretStore()
			for _, id := range []string{
				"kandev-runtime:resource-instance-1:agentctl-auth",
				"kandev-runtime:resource-instance-1:agentctl-bootstrap",
			} {
				require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
					Secret: secrets.Secret{ID: id, Name: id}, Value: "secret",
				}))
			}
			mgr := newTestManager(t)
			mgr.SetSecretStore(store)

			err := mgr.deleteKubernetesRuntimeSecrets(context.Background(), map[string]interface{}{
				MetadataKeyKubernetesInventoryState:     tc.state,
				MetadataKeyKubernetesResourceInstanceID: "resource-instance-1",
			})

			require.NoError(t, err)
			store.mu.RLock()
			defer store.mu.RUnlock()
			if tc.wantDeleted {
				require.Empty(t, store.store)
			} else {
				require.Len(t, store.store, 2)
			}
		})
	}
}

func TestRegisteredKubernetesLaunchRollbackDeletesSecretsBeforeReleasingInventory(t *testing.T) {
	store := newInMemorySecretStore()
	for id := range map[string]struct{}{
		"kandev-runtime:execution-1:agentctl-auth": {}, "kandev-runtime:execution-1:agentctl-bootstrap": {},
	} {
		if err := store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: "secret",
		}); err != nil {
			t.Fatalf("seed runtime secret: %v", err)
		}
	}
	mgr := newTestManager(t)
	mgr.SetSecretStore(store)
	execution := &AgentExecution{
		ID: "execution-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeKubernetes,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "kandev-runtime:execution-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret: "kandev-runtime:execution-1:agentctl-bootstrap",
		},
	}
	releases := 0
	instance := &ExecutorInstance{ReleaseRuntimeInventory: func(context.Context) error {
		releases++
		if len(store.store) != 0 {
			return errors.New("inventory released before secrets were deleted")
		}
		return nil
	}}

	err := mgr.stopRegisteredLaunchRuntime(
		&stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}},
		instance,
		execution,
	)

	if err != nil {
		t.Fatalf("stopRegisteredLaunchRuntime() error = %v", err)
	}
	if releases != 1 {
		t.Fatalf("runtime inventory releases = %d, want 1", releases)
	}
}

func TestRegisteredKubernetesResumeRollbackPreservesRetainedRuntimeAndSecrets(t *testing.T) {
	store := newInMemorySecretStore()
	for id := range map[string]struct{}{
		"kandev-runtime:execution-1:agentctl-auth": {}, "kandev-runtime:execution-1:agentctl-bootstrap": {},
	} {
		require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: "secret",
		}))
	}
	mgr := newTestManager(t)
	mgr.SetSecretStore(store)
	execution := &AgentExecution{
		ID: "execution-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeKubernetes,
		isResumedSession: true,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "kandev-runtime:execution-1:agentctl-auth",
			MetadataKeyBootstrapNonceSecret: "kandev-runtime:execution-1:agentctl-bootstrap",
		},
	}
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}

	err := mgr.stopRegisteredLaunchRuntime(backend, &ExecutorInstance{}, execution)

	require.NoError(t, err)
	require.Equal(t, []bool{false}, backend.forces, "resume rollback must only close its local client/forward")
	require.Len(t, store.store, 2, "retained runtime secrets remain authoritative")
}

func TestFinishedKubernetesResumeRollbackPreservesDurableInventoryRow(t *testing.T) {
	mgr := newTestManager(t)
	writer := &captureExecutorRunningWriter{prior: &models.ExecutorRunning{
		SessionID:        "session-1",
		AgentExecutionID: "execution-1",
		Runtime:          agentruntime.RuntimeKubernetes,
	}}
	mgr.SetExecutorRunningWriter(writer)
	execution := &AgentExecution{
		ID: "execution-1", SessionID: "session-1", RuntimeName: agentruntime.RuntimeKubernetes,
		isResumedSession: true,
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	mgr.finishRegisteredLaunchRollback(execution, true, true)

	require.Zero(t, writer.deleteCalls, "retained Kubernetes inventory must remain available to terminal cleanup")
	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists)
}
