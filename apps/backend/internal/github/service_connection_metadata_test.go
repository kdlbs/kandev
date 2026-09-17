package github

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssistantCapabilitiesConnectionMetadataNeverResolvesCredentials(t *testing.T) {
	s, _ := newWorkspaceConnectionService(t, "example")
	s.connectionSecrets = nil
	s.tokenClientFactory = func(string) Client { t.Fatal("metadata lookup must not create provider client"); return nil }
	require.NoError(t, s.store.UpsertWorkspaceConnection(context.Background(), activeAppWorkspace("ws-1", 123)))
	row, err := s.WorkspaceConnectionMetadata(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, ConnectionStatusActive, row.Status)
	s.SetWorkspaceAuthorizer(func(context.Context, string) error { return errors.New("denied") })
	_, err = s.WorkspaceConnectionMetadata(context.Background(), "ws-1")
	require.Error(t, err)
}
