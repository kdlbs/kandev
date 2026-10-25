package configsync

import (
	"context"
	"testing"
	"time"

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

func TestService_SyncWorkspace_AbandonedRunIsRecordedWhenContextAlreadyExpired(t *testing.T) {
	// AC-OFFICE-CONFIG-SYNC-004.4a: a run abandoned before Runner.Reconcile
	// ever starts (the caller's context expired while queued behind the
	// per-workspace lock, or was already done on entry) must still be
	// recorded as a failure, not silently return nothing.
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	result, syncErr := svc.SyncWorkspace(canceledCtx, "ws-1")
	require.Error(t, syncErr)
	assert.Nil(t, result)

	cfg, getErr := svc.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, getErr)
	require.NotNil(t, cfg)
	assert.False(t, cfg.LastOk)
	assert.NotEmpty(t, cfg.LastError, "an abandoned run must still be recorded as a failure")
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

// TestService_PurgeForWorkspaceDeletion_SerializesAgainstInFlightRun proves
// PurgeForWorkspaceDeletion waits for the per-workspace lock rather than
// racing an in-flight run: a SyncWorkspace-shaped holder of the lock blocks
// the purge until it releases, so a run's writes can never land after the
// bulk delete it should have been serialized behind.
func TestService_PurgeForWorkspaceDeletion_SerializesAgainstInFlightRun(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	lock := svc.workspaceLock("ws-1")
	lock.Lock()

	type purgeResult struct {
		unlock func()
		err    error
	}
	purgeDone := make(chan purgeResult, 1)
	go func() {
		unlock, err := svc.PurgeForWorkspaceDeletion(context.Background(), "ws-1")
		purgeDone <- purgeResult{unlock: unlock, err: err}
	}()

	select {
	case <-purgeDone:
		t.Fatal("PurgeForWorkspaceDeletion returned while the workspace lock was held by an in-flight run")
	case <-time.After(100 * time.Millisecond):
		// Still blocked, as expected.
	}

	lock.Unlock()

	var res purgeResult
	select {
	case res = <-purgeDone:
		require.NoError(t, res.err)
	case <-time.After(2 * time.Second):
		t.Fatal("PurgeForWorkspaceDeletion did not complete after the lock was released")
	}
	require.NotNil(t, res.unlock)
	res.unlock()

	cfg, err := svc.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

// TestService_PurgeForWorkspaceDeletion_ReturnsLockStillHeld proves the lock
// PurgeForWorkspaceDeletion takes is not released until the caller invokes
// the returned unlock func, so a caller can extend the lock's scope across
// its own later teardown steps — exactly what office/service.DeleteWorkspace
// does to keep an in-flight sync run from writing rows back in after it
// deletes the workspace's other data.
func TestService_PurgeForWorkspaceDeletion_ReturnsLockStillHeld(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	unlock, err := svc.PurgeForWorkspaceDeletion(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, unlock)

	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.PurgeForWorkspaceDeletion(context.Background(), "ws-1")
		secondDone <- err
	}()

	select {
	case <-secondDone:
		t.Fatal("a second PurgeForWorkspaceDeletion call returned before the first call's unlock was invoked")
	case <-time.After(100 * time.Millisecond):
		// Still blocked, as expected.
	}

	unlock()

	select {
	case err := <-secondDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("second PurgeForWorkspaceDeletion did not complete after unlock was called")
	}
}

// TestService_SetConfigForWorkspace_RefusesAfterConcurrentWorkspaceDeletion
// proves the race a same-name config row would otherwise resurrect: a
// SetConfigForWorkspace call queued behind the same per-workspace lock as an
// in-flight PurgeForWorkspaceDeletion must not upsert a config row once the
// deletion has completed, even though PurgeForWorkspaceDeletion's own unlock
// has not been called yet by the caller's later teardown steps (mirroring
// office/service.DeleteWorkspace holding it through DeleteWorkspaceData).
func TestService_SetConfigForWorkspace_RefusesAfterConcurrentWorkspaceDeletion(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	_, err := svc.SetConfigForWorkspace(ctx, "ws-1", testSetConfigRequest("cfg"))
	require.NoError(t, err)

	unlock, err := svc.PurgeForWorkspaceDeletion(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, unlock)

	setDone := make(chan error, 1)
	go func() {
		_, err := svc.SetConfigForWorkspace(context.Background(), "ws-1", testSetConfigRequest("cfg"))
		setDone <- err
	}()

	select {
	case <-setDone:
		t.Fatal("SetConfigForWorkspace returned before PurgeForWorkspaceDeletion's unlock was invoked")
	case <-time.After(100 * time.Millisecond):
		// Still blocked, as expected.
	}

	unlock()

	select {
	case err := <-setDone:
		require.ErrorIs(t, err, ErrWorkspaceGone)
	case <-time.After(2 * time.Second):
		t.Fatal("SetConfigForWorkspace did not complete after unlock was called")
	}

	cfg, err := svc.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg, "no config row must be resurrected for the deleted workspace")
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
