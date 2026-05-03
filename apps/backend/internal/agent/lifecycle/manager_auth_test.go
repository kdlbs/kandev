package lifecycle

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

// inMemorySecretStore implements secrets.SecretStore for testing.
type inMemorySecretStore struct {
	store map[string]*secrets.SecretWithValue
	err   error
}

var _ secrets.SecretStore = (*inMemorySecretStore)(nil)

func newInMemorySecretStore() *inMemorySecretStore {
	return &inMemorySecretStore{store: make(map[string]*secrets.SecretWithValue)}
}

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

func (s *inMemorySecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	if sw, ok := s.store[id]; ok {
		return &sw.Secret, nil
	}
	return nil, fmt.Errorf("not found")
}

func (s *inMemorySecretStore) Reveal(_ context.Context, id string) (string, error) {
	if sw, ok := s.store[id]; ok {
		return sw.Value, nil
	}
	return "", fmt.Errorf("not found")
}

func (s *inMemorySecretStore) Update(_ context.Context, _ string, _ *secrets.UpdateSecretRequest) error {
	return nil
}
func (s *inMemorySecretStore) Delete(_ context.Context, _ string) error { return nil }
func (s *inMemorySecretStore) List(_ context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}
func (s *inMemorySecretStore) Close() error { return nil }

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
			Metadata: make(map[string]interface{}),
		}

		m.persistAuthToken(context.Background(), instance, execution)

		// Verify secret was stored
		if len(store.store) != 1 {
			t.Fatalf("expected 1 secret, got %d", len(store.store))
		}

		// Verify metadata has the secret ID
		secretID, ok := execution.Metadata[MetadataKeyAuthTokenSecret].(string)
		if !ok || secretID == "" {
			t.Fatalf("expected secret ID in metadata, got %v", execution.Metadata[MetadataKeyAuthTokenSecret])
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
		execution := &AgentExecution{Metadata: make(map[string]interface{})}

		m.persistAuthToken(context.Background(), instance, execution)

		if len(store.store) != 0 {
			t.Fatal("expected no secrets stored")
		}
		if _, ok := execution.Metadata[MetadataKeyAuthTokenSecret]; ok {
			t.Fatal("expected no metadata key")
		}
	})

	t.Run("no-op when secret store is nil", func(t *testing.T) {
		m := &Manager{logger: log}

		instance := &ExecutorInstance{
			InstanceID: "exec-123456789012",
			AuthToken:  "some-token",
		}
		execution := &AgentExecution{Metadata: make(map[string]interface{})}

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

		if execution.Metadata == nil {
			t.Fatal("expected metadata to be initialized")
		}
		if _, ok := execution.Metadata[MetadataKeyAuthTokenSecret]; !ok {
			t.Fatal("expected secret ID in metadata")
		}
	})
}

func TestPersistRuntimeSecrets(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	store := newInMemorySecretStore()
	m := &Manager{logger: log, secretStore: store}

	instance := &ExecutorInstance{
		InstanceID:     "exec-123456789012",
		AuthToken:      "agentctl-token",
		BootstrapNonce: "bootstrap-nonce",
	}
	execution := &AgentExecution{Metadata: make(map[string]interface{})}

	m.persistRuntimeSecrets(context.Background(), instance, execution)

	authSecretID, ok := execution.Metadata[MetadataKeyAuthTokenSecret].(string)
	if !ok || authSecretID == "" {
		t.Fatalf("expected auth token secret ID, got %v", execution.Metadata[MetadataKeyAuthTokenSecret])
	}
	nonceSecretID, ok := execution.Metadata[MetadataKeyBootstrapNonceSecret].(string)
	if !ok || nonceSecretID == "" {
		t.Fatalf("expected bootstrap nonce secret ID, got %v", execution.Metadata[MetadataKeyBootstrapNonceSecret])
	}

	if got := m.revealRuntimeSecret(context.Background(), execution.Metadata, MetadataKeyAuthTokenSecret); got != "agentctl-token" {
		t.Fatalf("revealed auth token = %q, want agentctl-token", got)
	}
	if got := m.revealRuntimeSecret(context.Background(), execution.Metadata, MetadataKeyBootstrapNonceSecret); got != "bootstrap-nonce" {
		t.Fatalf("revealed bootstrap nonce = %q, want bootstrap-nonce", got)
	}
}
