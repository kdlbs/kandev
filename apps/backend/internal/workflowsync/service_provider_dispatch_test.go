package workflowsync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type blockingGitHubClients struct {
	started       chan struct{}
	release       chan struct{}
	unblockedCall chan struct{}
}

func (f *blockingGitHubClients) ListRepoDirectoryForWorkspace(
	ctx context.Context, workspaceID, _, _, _, _ string,
) ([]github.RepoContentEntry, error) {
	if workspaceID == "a-throttled" {
		close(f.started)
		select {
		case <-f.release:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	close(f.unblockedCall)
	return nil, nil
}

func (f *blockingGitHubClients) GetRepoFileContentForWorkspace(
	_ context.Context, _, _, _, _, _ string,
) ([]byte, error) {
	return nil, nil
}

type observingGitLabClients struct {
	called chan struct{}
}

func (f *observingGitLabClients) ListRepoTreeForWorkspace(
	_ context.Context, _, _, _, _ string,
) ([]gitlab.RepoTreeEntry, error) {
	close(f.called)
	return nil, nil
}

func (f *observingGitLabClients) GetRepoFileContentForWorkspace(
	_ context.Context, _, _, path, _ string,
) ([]byte, error) {
	return nil, nil
}

// fakeGitLabClients is a minimal GitLabClientProvider double — it does not
// need to model the real GitLab API, only the shape workflowsync consumes.
type fakeGitLabClients struct {
	entries []gitlab.RepoTreeEntry
	content map[string][]byte
	listErr error
	getErr  error
}

func (f fakeGitLabClients) ListRepoTreeForWorkspace(
	_ context.Context, _, _, _, _ string,
) ([]gitlab.RepoTreeEntry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.entries, nil
}

func (f fakeGitLabClients) GetRepoFileContentForWorkspace(
	_ context.Context, _, _, path, _ string,
) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.content[path], nil
}

func setupGitLabTestService(t *testing.T, gitlabClients GitLabClientProvider) (*Service, *fakeApplier) {
	t.Helper()
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return NewService(store, nil, gitlabClients, applier, log), applier
}

func configureGitLabWorkspace(t *testing.T, svc *Service, workspaceID string) {
	t.Helper()
	_, err := svc.SetConfigForWorkspace(context.Background(), workspaceID, &SetConfigRequest{
		Provider:    ProviderGitLab,
		ProjectPath: "acme/team/project",
	})
	require.NoError(t, err)
}

func TestSyncWorkspace_GitLabProvider_AppliesParsedFiles(t *testing.T) {
	gl := fakeGitLabClients{
		entries: []gitlab.RepoTreeEntry{
			{Name: "dev.yml", Path: DefaultPath + "/dev.yml", Type: gitlab.TreeEntryTypeBlob},
			{Name: "nested", Path: DefaultPath + "/nested", Type: gitlab.TreeEntryTypeTree},
		},
		content: map[string][]byte{DefaultPath + "/dev.yml": []byte(validExportYAML)},
	}
	svc, applier := setupGitLabTestService(t, gl)
	configureGitLabWorkspace(t, svc, "ws-1")
	applier.result = &workflowservice.SyncApplyResult{Created: []string{"Dev Flow"}}

	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.False(t, result.Unchanged)

	require.Len(t, applier.calls, 1)
	require.Len(t, applier.calls[0], 1, "the tree entry must be excluded, only the blob synced")
	assert.Equal(t, DefaultPath+"/dev.yml", applier.calls[0][0].Path)
}

func TestSyncWorkspace_GitLabProvider_NilClientRecordsProviderSpecificFailure(t *testing.T) {
	svc, _ := setupGitLabTestService(t, nil)
	configureGitLabWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GitLab")

	cfg, cfgErr := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, cfgErr)
	assert.False(t, cfg.LastOk)
}

// A GitHub-provider config with a nil GitHub client must not be affected by
// the GitLab client also being nil (and vice versa) — the two providers are
// dispatched independently.
func TestSyncWorkspace_GitHubProvider_UnaffectedByNilGitLabClient(t *testing.T) {
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(store, fakeGitHubClients{client: seededMockClient()}, nil, applier, log)
	configureWorkspace(t, svc, "ws-1")
	applier.result = &workflowservice.SyncApplyResult{Created: []string{"Dev Flow"}}

	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.False(t, result.Unchanged)
}

func TestSyncWorkspace_GitLabProvider_ListErrorIsWrappedWithProjectContext(t *testing.T) {
	gl := fakeGitLabClients{listErr: assertErr("boom")}
	svc, _ := setupGitLabTestService(t, gl)
	configureGitLabWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acme/team/project")
}

func TestSyncDueConfigs_DoesNotHeadOfLineBlockAcrossProviders(t *testing.T) {
	githubClients := &blockingGitHubClients{
		started:       make(chan struct{}),
		release:       make(chan struct{}),
		unblockedCall: make(chan struct{}),
	}
	gitlabClients := &observingGitLabClients{called: make(chan struct{})}
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(store, githubClients, gitlabClients, applier, log)
	configureWorkspace(t, svc, "a-throttled")
	configureWorkspace(t, svc, "b-unblocked")
	configureGitLabWorkspace(t, svc, "c-gitlab")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer close(githubClients.release)

	done := make(chan struct{})
	go func() {
		svc.SyncDueConfigs(ctx)
		close(done)
	}()

	select {
	case <-githubClients.started:
	case <-time.After(time.Second):
		t.Fatal("GitHub sync did not reach the blocking provider")
	}
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("SyncDueConfigs remained blocked by a throttled GitHub config")
	}
	select {
	case <-gitlabClients.called:
	case <-time.After(time.Second):
		t.Fatal("GitLab sync was delayed by the blocked GitHub config")
	}
	select {
	case <-githubClients.unblockedCall:
	case <-time.After(time.Second):
		t.Fatal("unblocked GitHub sync was delayed by the blocked GitHub config")
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
