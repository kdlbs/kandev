package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/secrets"
)

type profileSecretStore struct {
	secret *secrets.Secret
}

func (s *profileSecretStore) Create(context.Context, *secrets.SecretWithValue) error { return nil }
func (s *profileSecretStore) Get(context.Context, string) (*secrets.Secret, error) {
	if s.secret == nil {
		return nil, errors.New("secret not found")
	}
	return s.secret, nil
}
func (s *profileSecretStore) Reveal(context.Context, string) (string, error) {
	return "", errors.New("not used")
}
func (s *profileSecretStore) Update(context.Context, string, *secrets.UpdateSecretRequest) error {
	return nil
}
func (s *profileSecretStore) Delete(context.Context, string) error { return nil }
func (s *profileSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	return nil, nil
}
func (s *profileSecretStore) Close() error { return nil }

func TestValidateGlobalSecretRefsRejectsWorkspaceSecret(t *testing.T) {
	controller := &Controller{secretStore: &profileSecretStore{secret: &secrets.Secret{
		ID: "secret-1", Scope: secrets.ScopeWorkspace, WorkspaceID: "workspace-1",
	}}}

	err := controller.validateGlobalSecretRefs(context.Background(), []dto.ProfileEnvVarDTO{{
		Key: "TOKEN", SecretID: "secret-1",
	}})
	if err == nil {
		t.Fatal("workspace secret accepted in shared profile")
	}
}

func TestValidateGlobalSecretRefsAcceptsGlobalSecret(t *testing.T) {
	controller := &Controller{secretStore: &profileSecretStore{secret: &secrets.Secret{
		ID: "secret-1", Scope: secrets.ScopeGlobal,
	}}}

	if err := controller.validateGlobalSecretRefs(context.Background(), []dto.ProfileEnvVarDTO{{
		Key: "TOKEN", SecretID: "secret-1",
	}}); err != nil {
		t.Fatalf("global secret rejected: %v", err)
	}
}

func TestValidateGlobalSecretRefsRejectsBackendOwnedIDThroughUserVisibleStore(t *testing.T) {
	controller := &Controller{secretStore: secrets.NewUserVisibleStore(&profileSecretStore{secret: &secrets.Secret{
		ID: "github:user:workspace:user:access", Scope: secrets.ScopeGlobal,
	}})}

	err := controller.validateGlobalSecretRefs(context.Background(), []dto.ProfileEnvVarDTO{{
		Key: "TOKEN", SecretID: "github:user:workspace:user:access",
	}})
	if err == nil {
		t.Fatal("backend-owned secret ID accepted in shared profile")
	}
}
