package configsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func newTestService(t *testing.T) (*Service, *sqlite.Repository, *fakeGitHub) {
	t.Helper()
	repo, store := newReconcileTestRepo(t)
	fg := newFakeGitHub()
	runner := NewRunner(fg, nil, repo, store)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return NewService(runner, store, log), repo, fg
}

func testSetConfigRequest(path string) *SetConfigRequest {
	return &SetConfigRequest{
		Provider:  ProviderGitHub,
		RepoOwner: "acme",
		RepoName:  "kandev-config",
		Branch:    "main",
		Path:      &path,
	}
}

func TestService_SetConfigForWorkspace_ValidatesAndStores(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	cfg, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "cfg", cfg.Path)

	_, err = svc.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{RepoOwner: "o"})
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestService_GetConfigForWorkspace_MissingReturnsNil(t *testing.T) {
	svc, _, _ := newTestService(t)
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestService_HasActiveSource(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	active, err := svc.HasActiveSource(ctx, "ws-1")
	require.NoError(t, err)
	assert.False(t, active)

	_, err = svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	active, err = svc.HasActiveSource(ctx, "ws-1")
	require.NoError(t, err)
	assert.True(t, active)
}

func TestService_SyncWorkspace_NotConfiguredReturnsError(t *testing.T) {
	svc, _, _ := newTestService(t)
	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	assert.ErrorIs(t, err, ErrNotConfigured)
	assert.Nil(t, result)
}

func TestService_SyncWorkspace_HappyPathCreatesEntities(t *testing.T) {
	svc, repo, fg := newTestService(t)
	ctx := context.Background()

	_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")

	result, err := svc.SyncWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, []string{"ceo"}, result.Created)

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, agents, 1)
}

// TestService_DeleteConfigForWorkspace_ReleasesManifestButKeepsEntities
// verifies AC-OFFICE-CONFIG-SYNC-004.9: release drops every manifest row
// (the "managed" flag) without writing the underlying entity rows at all, so
// a previously-synced agent survives, editable, as an unmanaged row.
func TestService_DeleteConfigForWorkspace_ReleasesManifestButKeepsEntities(t *testing.T) {
	svc, repo, fg := newTestService(t)
	ctx := context.Background()

	_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	fg.dirs["cfg"] = []github.RepoContentEntry{}
	fg.dirs["cfg/agents"] = []github.RepoContentEntry{fileEntry("cfg/agents/ceo.yml")}
	fg.files["cfg/agents/ceo.yml"] = []byte("name: ceo\nrole: manager\n")
	fg.dirs["cfg/projects"] = []github.RepoContentEntry{fileEntry("cfg/projects/website.yml")}
	fg.files["cfg/projects/website.yml"] = []byte("name: website\ncolor: blue\n")

	_, err = svc.SyncWorkspace(ctx, "ws-1")
	require.NoError(t, err)

	manifest, err := svc.Store().ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, manifest, 2)

	require.NoError(t, svc.DeleteConfigForWorkspace(ctx, "ws-1"))

	manifest, err = svc.Store().ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, manifest)

	cfg, err := svc.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)

	agents, err := repo.ListAgentInstances(ctx, "ws-1")
	require.NoError(t, err)
	assert.Len(t, agents, 1, "release must not delete the entity, only its manifest ownership row")

	projects, err := repo.ListProjects(ctx, "ws-1")
	require.NoError(t, err)
	assert.Len(t, projects, 1)
}

func TestService_SyncDueConfigs_SkipsPollDisabledAndNotDue(t *testing.T) {
	svc, _, fg := newTestService(t)
	ctx := context.Background()

	disabled := false
	req := testSetConfigRequest("cfg")
	req.PollEnabled = &disabled
	_, err := svc.SetConfigForWorkspace(ctx, "ws-disabled", req)
	require.NoError(t, err)

	_, err = svc.SetConfigForWorkspace(ctx, "ws-due", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	fg.dirs["cfg"] = []github.RepoContentEntry{}

	svc.SyncDueConfigs(ctx)

	cfgDisabled, err := svc.GetConfigForWorkspace(ctx, "ws-disabled")
	require.NoError(t, err)
	assert.Nil(t, cfgDisabled.LastSyncedAt, "poll-disabled workspace must not sync")

	cfgDue, err := svc.GetConfigForWorkspace(ctx, "ws-due")
	require.NoError(t, err)
	assert.NotNil(t, cfgDue.LastSyncedAt, "never-synced workspace is due immediately")

	// Second tick: interval_seconds (default 300) has not elapsed, so this is
	// a no-op and must not error or re-sync.
	before := *cfgDue.LastSyncedAt
	svc.SyncDueConfigs(ctx)
	cfgDue, err = svc.GetConfigForWorkspace(ctx, "ws-due")
	require.NoError(t, err)
	assert.Equal(t, before, *cfgDue.LastSyncedAt)
}
