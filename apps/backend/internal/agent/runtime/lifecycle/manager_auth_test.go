package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
	return nil, fmt.Errorf("not found")
}

// Reveal returns the plaintext value for the given ID, or an error when absent.
func (s *inMemorySecretStore) Reveal(_ context.Context, id string) (string, error) {
	if s.revealErr != nil {
		return "", s.revealErr
	}
	if sw, ok := s.store[id]; ok {
		return sw.Value, nil
	}
	return "", fmt.Errorf("not found")
}

// Update is a no-op for the in-memory store.
func (s *inMemorySecretStore) Update(_ context.Context, _ string, _ *secrets.UpdateSecretRequest) error {
	return nil
}

// Delete is a no-op for the in-memory store.
func (s *inMemorySecretStore) Delete(_ context.Context, _ string) error { return nil }

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
