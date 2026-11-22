package promptsync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/reposync"
)

func TestAutomaticRunDeferredByGitHubAdmissionIsNotAFailure(t *testing.T) {
	svc, gh, _ := newGitHubFixture(t)
	ctx := context.Background()
	gh.listErr = &github.AdmissionDeferredError{Delay: time.Minute, Reason: "test background pacing"}

	svc.SyncIfDue(ctx)

	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.ConsecutiveFailures)
	assert.False(t, cfg.PollSuspended)
	assert.Empty(t, cfg.LastError)
	assert.Nil(t, cfg.NextAttemptAt)

	svc.SyncIfDue(ctx)
	assert.Equal(t, 2, gh.lists, "the configuration stays due, so the next tick retries the run")
}

func TestManualRunDeferredByGitHubAdmissionStillRecordsAFailure(t *testing.T) {
	svc, gh, _ := newGitHubFixture(t)
	ctx := context.Background()
	gh.listErr = &github.AdmissionDeferredError{Delay: time.Minute, Reason: "test interactive pacing"}

	_, err := svc.Sync(ctx)
	require.Error(t, err)

	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.ConsecutiveFailures)
}

func TestPromptSyncManualQuotaDoesNotBlockVersionSave(t *testing.T) {
	for _, stage := range []string{"before listing", "after one file read"} {
		t.Run(stage, func(t *testing.T) {
			assertManualQuotaSaveScenario(t, stage)
		})
	}
}

func assertManualQuotaSaveScenario(t *testing.T, stage string) {
	t.Helper()
	ctx := context.Background()
	database := openTestDB(t)
	repo, repoCleanup, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, repoCleanup()) })
	prompts := promptservice.NewService(repo)
	_, err = prompts.CreatePrompt(ctx, "a", "A")
	require.NoError(t, err)
	_, err = prompts.ApplySyncedPrompts(ctx, promptservice.SyncApplyInput{
		Desired: []promptservice.SyncedPromptFile{{Name: "a", Content: "A", Path: DefaultPath + "/a.md"}},
	})
	require.NoError(t, err)
	prompt, err := prompts.GetPromptByName(ctx, "a")
	require.NoError(t, err)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	tracker := github.NewRateTracker(nil, nil)
	client := github.NewPATClient("fixture").WithRateTracker(tracker)
	coordinatorOwner := github.NewService(client, github.AuthMethodPAT, nil, nil, nil, log)
	t.Cleanup(coordinatorOwner.Stop)
	retryAt := time.Now().Add(5 * time.Minute).UTC()
	provider := &coordinatorBackedGitHub{
		client: client, tracker: tracker, retryAt: retryAt,
		blockListing: stage == "before listing", seedAfterFirstRead: stage == "after one file read",
	}
	svc := NewService(newTestStore(t, database), provider, nil, prompts, nil, log)
	_, err = svc.SetConfig(ctx, &SetConfigRequest{
		WorkspaceID: "ws-1", Provider: reposync.ProviderGitHub, RepoOwner: "acme", RepoName: "eng",
	})
	require.NoError(t, err)

	if stage == "before listing" {
		provider.seedQuotaWindow()
	}
	syncBound := make(chan struct{})
	provider.syncBound = syncBound

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	syncDone := make(chan error, 1)
	syncFinished := make(chan struct{})
	go func() {
		defer close(syncFinished)
		_, syncErr := svc.Sync(syncCtx)
		syncDone <- syncErr
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-syncFinished:
		case <-time.After(time.Second):
			t.Error("manual sync did not stop after the test ended")
		}
	})
	select {
	case <-syncBound:
	case <-time.After(time.Second):
		t.Fatal("manual sync did not acquire the Prompt Sync lock")
	}

	local := "Local edit"
	saveDone := make(chan error, 1)
	saveFinished := make(chan struct{})
	go func() {
		defer close(saveFinished)
		_, saveErr := svc.SaveVersions(ctx, prompt.ID, promptservice.VersionSave{
			UseLocal: true, LocalContent: &local,
		})
		saveDone <- saveErr
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-syncFinished:
		case <-time.After(time.Second):
			t.Error("manual sync did not stop after Save setup ended")
		}
		select {
		case <-saveFinished:
		case <-time.After(time.Second):
			t.Error("version save did not finish after the test ended")
		}
	})

	select {
	case syncErr := <-syncDone:
		require.ErrorIs(t, syncErr, github.ErrGitHubAdmissionWaitRequired)
		var waitErr *github.AdmissionWaitError
		require.ErrorAs(t, syncErr, &waitErr)
		require.Equal(t, retryAt, waitErr.RetryAt)
	case <-time.After(time.Second):
		t.Fatal("manual sync waited instead of returning the quota refusal")
	}
	select {
	case saveErr := <-saveDone:
		require.NoError(t, saveErr)
		assert.Positive(t, tracker.WaitDuration(github.ResourceCore), "the quota window is still active when Save completes")
	case <-time.After(time.Second):
		t.Fatal("version save remained blocked during the quota window")
	}

	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, github.RateLimitErrorCode, cfg.LastErrorClass)
	assert.Equal(t, quotaUnavailableMessage, cfg.LastError)
	require.NotNil(t, cfg.NextAttemptAt)
	assert.Equal(t, retryAt, *cfg.NextAttemptAt)
	updated, err := prompts.GetPromptByID(ctx, prompt.ID)
	require.NoError(t, err)
	assert.Equal(t, "Local edit", updated.Content)
	assert.Equal(t, "A", updated.AlternateContent, "an incomplete fetch never applies files")
	if stage == "before listing" {
		assert.True(t, provider.listingBlocked, "the listing passes through the coordinator-backed client")
	} else {
		assert.Equal(t, 1, provider.fixtureReads, "one file was read before the next admission was blocked")
		assert.True(t, provider.fileReadBlocked, "the blocked file read passes through the coordinator-backed client")
	}
}

type coordinatorBackedGitHub struct {
	client             *github.PATClient
	tracker            *github.RateTracker
	retryAt            time.Time
	blockListing       bool
	seedAfterFirstRead bool
	syncBound          chan struct{}
	listingBlocked     bool
	fileReadBlocked    bool
	fixtureReads       int
}

func (c *coordinatorBackedGitHub) BindWorkspaceGitHubHost(ctx context.Context, _, _ string) (context.Context, error) {
	if c.syncBound != nil {
		close(c.syncBound)
		c.syncBound = nil
	}
	return ctx, nil
}

func (*coordinatorBackedGitHub) CommitWorkspaceGitHubHost(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}

func (c *coordinatorBackedGitHub) ListRepoDirectoryForWorkspace(
	ctx context.Context, _, owner, repo, path, ref string,
) ([]github.RepoContentEntry, error) {
	if c.blockListing {
		c.listingBlocked = true
		return c.client.ListRepoDirectory(ctx, owner, repo, path, ref)
	}
	return []github.RepoContentEntry{
		{Name: "a.md", Path: path + "/a.md", Type: "file"},
		{Name: "b.md", Path: path + "/b.md", Type: "file"},
	}, nil
}

func (c *coordinatorBackedGitHub) GetRepoFileContentForWorkspace(
	ctx context.Context, _, owner, repo, path, ref string,
) ([]byte, error) {
	if c.seedAfterFirstRead {
		if c.fixtureReads == 0 {
			c.fixtureReads = 1
			c.seedQuotaWindow()
			return []byte("A changed"), nil
		}
		c.fileReadBlocked = true
	}
	return c.client.GetRepoFileContent(ctx, owner, repo, path, ref)
}

func (c *coordinatorBackedGitHub) seedQuotaWindow() {
	c.tracker.Record(github.RateSnapshot{
		Resource: github.ResourceCore, Limit: 5000, Remaining: 0,
		RemainingObserved: true, ParsedFromHeaders: true,
		ResetAt: c.retryAt, UpdatedAt: time.Now().UTC(),
	})
	c.tracker.ObservePrimary(github.ResourceCore, c.retryAt, github.RetrySourcePrimaryReset)
}

func (*coordinatorBackedGitHub) WorkspaceConnectionFingerprint(context.Context, string) (string, error) {
	return "coordinator-backed-fixture", nil
}
