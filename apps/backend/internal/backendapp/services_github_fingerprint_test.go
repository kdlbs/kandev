package backendapp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type githubFingerprintTransport struct{}

func (githubFingerprintTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"login":"alice"}`))}, nil
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-003.8
func TestGitHubLegacyFingerprintFollowsDurableSecretRevision(t *testing.T) {
	h := newBootStateTestHarness(t)
	ctx := context.Background()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	crypto, err := secrets.NewMasterKeyProvider(t.TempDir())
	require.NoError(t, err)
	store, closeSecrets, err := secrets.Provide(h.db, h.db, crypto)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closeSecrets()) })
	adapter := &githubSecretAdapter{store: store}
	githubStore, err := github.NewStore(h.db, h.db)
	require.NoError(t, err)
	require.NoError(t, h.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "fingerprint", Name: "Fingerprint"}))
	require.NoError(t, githubStore.UpsertWorkspaceConnection(ctx, &github.WorkspaceConnection{
		WorkspaceID: "fingerprint", Source: github.ConnectionSourceLegacyShared,
		GitHubHost: "github.com", Status: github.ConnectionStatusActive, CredentialGeneration: 1,
	}))
	original := http.DefaultTransport
	http.DefaultTransport = githubFingerprintTransport{}
	t.Cleanup(func() { http.DefaultTransport = original })
	svc := github.NewService(nil, github.AuthMethodNone, adapter, githubStore, nil, log)
	svc.SetSecretManager(adapter)
	t.Cleanup(svc.Stop)
	require.NoError(t, svc.ConfigureToken(ctx, "first-token"))
	before, err := svc.WorkspaceConnectionFingerprint(ctx, "fingerprint")
	require.NoError(t, err)
	require.NoError(t, svc.ConfigureToken(ctx, "replacement-token"))
	after, err := svc.WorkspaceConnectionFingerprint(ctx, "fingerprint")
	require.NoError(t, err)
	require.NotEqual(t, before, after, "persisted token replacement must change the recovery fingerprint")
	require.NotContains(t, after, "replacement-token")
	restarted := github.NewService(nil, github.AuthMethodNone, adapter, githubStore, nil, log)
	t.Cleanup(restarted.Stop)
	restored, err := restarted.WorkspaceConnectionFingerprint(ctx, "fingerprint")
	require.NoError(t, err)
	require.Equal(t, after, restored, "the unchanged secret revision must survive service restart")
	items, err := adapter.List(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, adapter.Delete(ctx, items[0].ID))
	cleared, err := restarted.WorkspaceConnectionFingerprint(ctx, "fingerprint")
	require.NoError(t, err)
	require.NotEqual(t, after, cleared, "secret removal must invalidate retained provider work")
}
