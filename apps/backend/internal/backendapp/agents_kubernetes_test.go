package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRegisterKubernetesBackendPreventsStandaloneFallback(t *testing.T) {
	t.Parallel()

	log := newTestLogger()
	registry := lifecycle.NewExecutorRegistry(log)
	registerKubernetesBackend(registry, lifecycle.NewAgentctlResolver(log), log)
	manager := lifecycle.NewManager(nil, nil, registry, nil, nil, nil, lifecycle.ExecutorFallbackWarn, "", log)
	if !manager.RequiresCloneURL(string(models.ExecutorTypeKubernetes)) {
		t.Fatal("k8s executor resolved as missing/standalone instead of its registered remote backend")
	}
	backend, err := registry.GetBackend(executor.NameKubernetes)
	if err != nil {
		t.Fatalf("GetBackend(k8s) error = %v", err)
	}
	if _, err := backend.CreateInstance(context.Background(), &lifecycle.ExecutorCreateRequest{}); err == nil {
		t.Fatal("Kubernetes backend accepted incomplete configuration")
	}
}

func TestRegisterKubernetesPreparerUsesRealLifecyclePreparer(t *testing.T) {
	t.Parallel()

	log := newTestLogger()
	registry := lifecycle.NewPreparerRegistry(log)
	registerKubernetesPreparer(registry, log)
	preparer := registry.Get(models.ExecutorTypeKubernetes)
	if preparer == nil {
		t.Fatal("k8s preparer was not registered")
	}
	result, err := preparer.Prepare(context.Background(), &lifecycle.EnvPrepareRequest{}, nil)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("Prepare() = %#v, want successful Kubernetes validation result", result)
	}
}

func TestLifecycleSecretStoresSeparateRuntimeAndUserCredentialSecrets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rawStore := newBackendappSecretStore()
	stores := newLifecycleSecretStores(rawStore)
	internal := &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "kandev-runtime:instance-1:agentctl-auth", Name: "runtime auth"},
		Value:  "runtime-token",
	}
	visible := &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "user-api-key", Name: "user API key"},
		Value:  "user-token",
	}
	if err := stores.runtime.Create(ctx, internal); err != nil {
		t.Fatal(err)
	}
	if err := stores.credentials.Create(ctx, visible); err != nil {
		t.Fatal(err)
	}
	if got, err := stores.runtime.Reveal(ctx, internal.ID); err != nil || got != internal.Value {
		t.Fatalf("runtime secret reveal = %q, %v; want raw internal value", got, err)
	}
	if got, err := stores.credentials.Reveal(ctx, internal.ID); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("internal runtime credential was revealed to agents: %q", got)
	}
	if got, err := stores.credentials.Reveal(ctx, visible.ID); err != nil || got != visible.Value {
		t.Fatalf("user credential reveal = %q, %v", got, err)
	}
	items, err := stores.credentials.List(ctx)
	if err != nil {
		t.Fatalf("list user credentials: %v", err)
	}
	if len(items) != 1 || items[0].ID != visible.ID {
		t.Fatalf("visible credentials = %#v; want only %q", items, visible.ID)
	}
	for _, item := range items {
		if item.ID == internal.ID {
			t.Fatalf("internal runtime credential appeared in agent credential list: %q", item.ID)
		}
	}
}

type backendappSecretStore struct {
	mu      sync.Mutex
	secrets map[string]*secrets.SecretWithValue
}

func newBackendappSecretStore() *backendappSecretStore {
	return &backendappSecretStore{secrets: make(map[string]*secrets.SecretWithValue)}
}

func (s *backendappSecretStore) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *secret
	s.secrets[secret.ID] = &copy
	return nil
}

func (s *backendappSecretStore) Get(_ context.Context, id string) (*secrets.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret := s.secrets[id]
	if secret == nil {
		return nil, fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	copy := secret.Secret
	return &copy, nil
}

func (s *backendappSecretStore) Reveal(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret := s.secrets[id]
	if secret == nil {
		return "", fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	return secret.Value, nil
}

func (s *backendappSecretStore) Update(
	_ context.Context,
	id string,
	req *secrets.UpdateSecretRequest,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret := s.secrets[id]
	if secret == nil {
		return fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	if req.Name != nil {
		secret.Name = *req.Name
	}
	if req.Value != nil {
		secret.Value = *req.Value
	}
	return nil
}

func (s *backendappSecretStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.secrets[id]; !ok {
		return fmt.Errorf("%w: %s", secrets.ErrNotFound, id)
	}
	delete(s.secrets, id)
	return nil
}

func (s *backendappSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]*secrets.SecretListItem, 0, len(s.secrets))
	for _, secret := range s.secrets {
		items = append(items, &secrets.SecretListItem{ID: secret.ID, Name: secret.Name, HasValue: true})
	}
	return items, nil
}

func (*backendappSecretStore) Close() error { return nil }
