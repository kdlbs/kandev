package promptsync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/gitlab"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	"github.com/kandev/kandev/internal/reposync"
)

type fakeGitLab struct {
	reposync.GitLabClientProvider
	files   map[string]string
	listErr error
	getErr  error
	lists   int
}

func (f *fakeGitLab) ListRepoTreeForWorkspace(_ context.Context, _, _, path, _ string) ([]gitlab.RepoTreeEntry, error) {
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	entries := []gitlab.RepoTreeEntry{{Name: "sub", Path: path + "/sub", Type: gitlab.TreeEntryTypeTree}}
	for name := range f.files {
		entries = append(entries, gitlab.RepoTreeEntry{Name: name, Path: path + "/" + name, Type: gitlab.TreeEntryTypeBlob})
	}
	return entries, nil
}

func (f *fakeGitLab) GetRepoFileContentForWorkspace(_ context.Context, _, _, path, _ string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for name, content := range f.files {
		if path == DefaultPath+"/"+name {
			return []byte(content), nil
		}
	}
	return nil, errors.New("missing file")
}

type fakeReferences struct {
	texts []promptservice.PromptReferenceText
}

func (f *fakeReferences) ListPromptReferenceTexts(context.Context) ([]promptservice.PromptReferenceText, error) {
	return f.texts, nil
}

type syncFixture struct {
	svc     *Service
	prompts *promptservice.Service
	gitlab  *fakeGitLab
	refs    *fakeReferences
	now     time.Time
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	database := openTestDB(t)
	repo, _, err := promptstore.Provide(database, database)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	f := &syncFixture{
		prompts: promptservice.NewService(repo),
		gitlab:  &fakeGitLab{files: map[string]string{}},
		refs:    &fakeReferences{},
		now:     time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC),
	}
	f.svc = NewService(newTestStore(t, database), nil, f.gitlab, f.prompts, f.refs, log)
	f.svc.now = func() time.Time { return f.now }
	f.svc.jitter = func(time.Duration) time.Duration { return 0 }
	f.svc.workspaceExists = func(context.Context, string) (bool, error) { return true, nil }
	_, err = f.svc.SetConfig(context.Background(), &SetConfigRequest{
		WorkspaceID: "ws-1", Provider: reposync.ProviderGitLab, ProjectPath: "acme/eng",
	})
	require.NoError(t, err)
	return f
}

func TestSyncCreatesUpdatesAndReportsUnresolvedReferences(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["principles.md"] = "Be simple. See @missing-rule."
	f.gitlab.files["README.txt"] = "ignored"

	result, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"principles"}, result.Created)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "@missing-rule")
	prompt, err := f.prompts.GetPromptByName(ctx, "principles")
	require.NoError(t, err)
	assert.True(t, prompt.IsSynced())
	assert.Equal(t, DefaultPath+"/principles.md", prompt.SourcePath)

	cfg, err := f.svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.True(t, cfg.LastOk)
	assert.Len(t, cfg.LastWarnings, 1)

	f.gitlab.files["principles.md"] = "Be simple."
	result, err = f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"principles"}, result.Updated)
	result, err = f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.True(t, result.Unchanged)
}

func TestSyncInvalidFileFreezesAndEmptySourceRemovesUnreferenced(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files = map[string]string{"keep.md": "Keep", "used.md": "Used", "drop.md": "Drop"}
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)

	f.gitlab.files = map[string]string{"keep.md": "   "}
	f.refs.texts = []promptservice.PromptReferenceText{{Owner: "wf-1", Text: "uses @used"}}
	result, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"drop"}, result.Deleted)
	assert.Len(t, result.Warnings, 2, "invalid keep.md and referenced used.md")
	keep, err := f.prompts.GetPromptByName(ctx, "keep")
	require.NoError(t, err)
	assert.Equal(t, "Keep", keep.Content)
}

func TestSyncFetchFailureChangesNothingAndBacksOff(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["a.md"] = "A"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)

	f.gitlab.files["a.md"] = "changed"
	f.gitlab.getErr = &gitlab.APIError{StatusCode: 502, Endpoint: "/projects/x", Body: "secret upstream body"}
	_, err = f.svc.Sync(ctx)
	require.Error(t, err)
	prompt, err := f.prompts.GetPromptByName(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "A", prompt.Content)
	cfg, err := f.svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.False(t, cfg.LastOk)
	assert.Equal(t, "GitLab request failed with HTTP status 502", cfg.LastError)
	require.NotNil(t, cfg.NextAttemptAt)
	assert.True(t, cfg.NextAttemptAt.After(f.now))
}

func TestSyncMissingCredentialWorkspaceSuspendsPolling(t *testing.T) {
	f := newSyncFixture(t)
	f.svc.workspaceExists = func(context.Context, string) (bool, error) { return false, nil }
	_, err := f.svc.Sync(context.Background())
	require.ErrorIs(t, err, ErrCredentialWorkspaceMissing)
	cfg, err := f.svc.GetConfig(context.Background())
	require.NoError(t, err)
	assert.True(t, cfg.PollSuspended)
	assert.Contains(t, cfg.PollSuspensionReason, "credential workspace")
	assert.Zero(t, f.gitlab.lists, "no provider call without a credential workspace")
}

func TestDeleteConfigReleasesSyncedPrompts(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files["a.md"] = "A"
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)

	require.NoError(t, f.svc.DeleteConfig(ctx, Removal{}))
	cfg, err := f.svc.GetConfig(ctx)
	require.NoError(t, err)
	assert.Nil(t, cfg)
	prompt, err := f.prompts.GetPromptByName(ctx, "a")
	require.NoError(t, err)
	assert.False(t, prompt.IsSynced())
	_, err = f.svc.Sync(ctx)
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestManualEntryPointsAuthorizeCredentialWorkspace(t *testing.T) {
	f := newSyncFixture(t)
	denied := errors.New("denied")
	var checked []string
	f.svc.SetWorkspaceAuthorizer(func(_ context.Context, id string) error {
		checked = append(checked, id)
		if id == "foreign" {
			return denied
		}
		return nil
	})
	ctx := context.Background()
	_, err := f.svc.SetConfig(ctx, &SetConfigRequest{WorkspaceID: "foreign", Provider: reposync.ProviderGitLab, ProjectPath: "a/b"})
	assert.ErrorIs(t, err, denied)
	_, err = f.svc.Sync(ctx)
	assert.NoError(t, err)
	assert.NoError(t, f.svc.DeleteConfig(ctx, Removal{}))
	assert.Equal(t, []string{"foreign", "ws-1", "ws-1"}, checked)
}

func TestSyncIfDueRespectsIntervalAndSuspension(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.svc.SyncIfDue(ctx)
	assert.Equal(t, 1, f.gitlab.lists, "a never-synced config is due")
	f.svc.SyncIfDue(ctx)
	assert.Equal(t, 1, f.gitlab.lists, "not due before the interval")
	f.now = f.now.Add(301 * time.Second)
	f.svc.SyncIfDue(ctx)
	assert.Equal(t, 2, f.gitlab.lists)

	f.svc.workspaceExists = func(context.Context, string) (bool, error) { return false, nil }
	f.now = f.now.Add(time.Hour)
	f.svc.SyncIfDue(ctx)
	f.now = f.now.Add(time.Hour)
	f.svc.SyncIfDue(ctx)
	assert.Equal(t, 2, f.gitlab.lists, "a suspended config makes no provider calls")
}
