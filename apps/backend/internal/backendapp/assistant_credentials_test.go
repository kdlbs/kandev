package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/stretchr/testify/require"
)

type assistantMetadataStore struct {
	secrets.ScopedSecretStore
	metadata  *secrets.Secret
	err       error
	reads     int
	reference string
	workspace string
}

func (s *assistantMetadataStore) GetForWorkspace(_ context.Context, id, workspace string) (*secrets.Secret, error) {
	s.reads++
	s.reference = id
	s.workspace = workspace
	return s.metadata, s.err
}

func TestAssistantCredentialValidationMetadataOnly(t *testing.T) {
	store := &assistantMetadataStore{metadata: &secrets.Secret{ID: "example-reference", UpdatedAt: time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)}}
	reader := assistantCredentialReader{store: store}
	descriptor := models.CredentialDescriptor{Reference: "example-reference", Resolver: "kandev"}
	for _, test := range []struct {
		err    error
		status string
	}{
		{nil, "ready"}, {secrets.ErrNotFound, "missing"}, {secrets.ErrWorkspaceAccessDenied, "missing"}, {errors.New("SYNTHETIC_SECRET_CANARY"), "unavailable"},
	} {
		store.err = test.err
		observation := reader.CredentialHealth(context.Background(), "ws", descriptor)
		require.Equal(t, test.status, observation.Status)
		require.NotNil(t, observation.ValidatedAt)
		require.Equal(t, "example-reference", store.reference)
		require.Equal(t, "ws", store.workspace)
		require.Empty(t, observation.Reason)
	}
	require.Equal(t, 4, store.reads)
	descriptor.Resolver = "bitwarden"
	unavailable := reader.CredentialHealth(context.Background(), "ws", descriptor)
	require.Nil(t, unavailable.ValidatedAt)
	require.Equal(t, "unavailable", unavailable.Status)
	require.Equal(t, 4, store.reads)
}
