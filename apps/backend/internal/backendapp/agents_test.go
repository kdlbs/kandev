package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/secrets"
)

type agentSecretStoreStub struct {
	created []*secrets.SecretWithValue
}

func (s *agentSecretStoreStub) Create(_ context.Context, secret *secrets.SecretWithValue) error {
	s.created = append(s.created, secret)
	return nil
}

func (*agentSecretStoreStub) Get(context.Context, string) (*secrets.Secret, error) {
	return nil, secrets.ErrNotFound
}

func (*agentSecretStoreStub) Reveal(context.Context, string) (string, error) {
	return "", secrets.ErrNotFound
}

func (*agentSecretStoreStub) Update(context.Context, string, *secrets.UpdateSecretRequest) error {
	return secrets.ErrNotFound
}

func (*agentSecretStoreStub) Delete(context.Context, string) error { return nil }

func (*agentSecretStoreStub) List(context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}

func (*agentSecretStoreStub) Close() error { return nil }

func TestAgentSecretStoresKeepRuntimeCredentialsInternal(t *testing.T) {
	raw := &agentSecretStoreStub{}
	stores := newAgentSecretStores(raw)
	internal := &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "runtime:task-environment:env-1:agentctl-auth"},
		Value:  "token",
	}

	if err := stores.runtime.Create(context.Background(), internal); err != nil {
		t.Fatalf("runtime store rejected internal credential: %v", err)
	}
	if err := stores.userVisible.Create(context.Background(), internal); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("user-visible store internal credential error = %v, want not found", err)
	}
	if len(raw.created) != 1 || raw.created[0].ID != internal.ID {
		t.Fatalf("raw store created = %#v", raw.created)
	}
}
