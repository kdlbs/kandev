package promptsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/reposync"
)

type fakeGitHub struct {
	reposync.GitHubClientProvider
	files       map[string]string
	listErr     error
	fingerprint string
	binds       int
	commits     int
	lists       int
}

func (f *fakeGitHub) BindWorkspaceGitHubHost(ctx context.Context, _, _ string) (context.Context, error) {
	f.binds++
	return ctx, nil
}

func (f *fakeGitHub) CommitWorkspaceGitHubHost(ctx context.Context, _ string, fn func(context.Context) error) error {
	f.commits++
	return fn(ctx)
}

func (f *fakeGitHub) ListRepoDirectoryForWorkspace(_ context.Context, _, _, _, path, _ string) ([]github.RepoContentEntry, error) {
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	var entries []github.RepoContentEntry
	for name := range f.files {
		entries = append(entries, github.RepoContentEntry{Name: name, Path: path + "/" + name, Type: "file"})
	}
	return entries, nil
}

func (f *fakeGitHub) GetRepoFileContentForWorkspace(_ context.Context, _, _, _, path, _ string) ([]byte, error) {
	return []byte(f.files[path[len(DefaultPath)+1:]]), nil
}

func (f *fakeGitHub) WorkspaceConnectionFingerprint(context.Context, string) (string, error) {
	return f.fingerprint, nil
}

func newGitHubFixture(t *testing.T) (*Service, *fakeGitHub, *promptservice.Service) {
	t.Helper()
	database := openTestDB(t)
	repo, _, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	gh := &fakeGitHub{files: map[string]string{"a.md": "A"}, fingerprint: "fp-1"}
	prompts := promptservice.NewService(repo)
	svc := NewService(newTestStore(t, database), gh, nil, prompts, nil, log)
	_, err = svc.SetConfig(context.Background(), &SetConfigRequest{WorkspaceID: "ws-1", RepoOwner: "acme", RepoName: "eng"})
	require.NoError(t, err)
	return svc, gh, prompts
}

func TestGitHubSyncBindsHostAndAppliesInsideCommit(t *testing.T) {
	svc, gh, prompts := newGitHubFixture(t)
	result, err := svc.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, result.Created)
	assert.Equal(t, 2, gh.binds, "save and sync both bind the workspace host")
	assert.Equal(t, 2, gh.commits)
	_, err = prompts.GetPromptByName(context.Background(), "a")
	require.NoError(t, err)
}

func TestGitHubCredentialFailureSuspendsUntilCredentialChanges(t *testing.T) {
	svc, gh, _ := newGitHubFixture(t)
	ctx := context.Background()
	svc.SyncIfDue(ctx) // records the first fingerprint and syncs
	require.Equal(t, 1, gh.lists)

	gh.listErr = &github.GitHubAPIError{StatusCode: 401, FailureKind: github.FailureInvalidCredentials}
	_, err := svc.Sync(ctx)
	require.Error(t, err)
	cfg, err := svc.GetConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.PollSuspended)
	assert.Equal(t, "GitHub request failed with HTTP status 401", cfg.PollSuspensionReason)

	svc.SyncIfDue(ctx)
	assert.Equal(t, 2, gh.lists, "suspended polling makes no provider call")

	gh.listErr = nil
	gh.fingerprint = "fp-2"
	svc.SyncIfDue(ctx)
	assert.Equal(t, 3, gh.lists, "a changed credential resumes polling at once")
	cfg, err = svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.False(t, cfg.PollSuspended)
	assert.True(t, cfg.LastOk)
}
