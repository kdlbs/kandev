package workflowsync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type fakeGitHubClients struct {
	client github.Client
}

type failingGitHubClients struct {
	err error
}

func (f failingGitHubClients) ListRepoDirectoryForWorkspace(
	context.Context, string, string, string, string, string,
) ([]github.RepoContentEntry, error) {
	return nil, f.err
}

type countingGitHubClients struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *countingGitHubClients) ListRepoDirectoryForWorkspace(
	context.Context, string, string, string, string, string,
) ([]github.RepoContentEntry, error) {
	f.mu.Lock()
	f.calls++
	err := f.err
	f.mu.Unlock()
	return nil, err
}

func (f *countingGitHubClients) GetRepoFileContentForWorkspace(
	context.Context, string, string, string, string, string,
) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil, f.err
}

func (f *countingGitHubClients) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *countingGitHubClients) setError(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func (f failingGitHubClients) GetRepoFileContentForWorkspace(
	context.Context, string, string, string, string, string,
) ([]byte, error) {
	return nil, f.err
}

func (f fakeGitHubClients) ListRepoDirectoryForWorkspace(
	ctx context.Context, _ string, owner, repo, path, ref string,
) ([]github.RepoContentEntry, error) {
	if f.client == nil {
		return nil, github.ErrNoClient
	}
	return f.client.ListRepoDirectory(ctx, owner, repo, path, ref)
}

func (f fakeGitHubClients) GetRepoFileContentForWorkspace(
	ctx context.Context, _ string, owner, repo, path, ref string,
) ([]byte, error) {
	if f.client == nil {
		return nil, github.ErrNoClient
	}
	return f.client.GetRepoFileContent(ctx, owner, repo, path, ref)
}

// fakeApplier is mutex-guarded because the poller invokes it from its own
// goroutine while tests assert on call counts (-race catches unguarded use).
type fakeApplier struct {
	mu       sync.Mutex
	calls    [][]workflowservice.SyncFileExport
	result   *workflowservice.SyncApplyResult
	released []string
}

func (f *fakeApplier) ApplySyncedWorkflows(_ context.Context, _ string, files []workflowservice.SyncFileExport) (*workflowservice.SyncApplyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, files)
	if f.result != nil {
		return f.result, nil
	}
	return &workflowservice.SyncApplyResult{}, nil
}

func (f *fakeApplier) ReleaseSyncedWorkflows(_ context.Context, workspaceID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, workspaceID)
	return []string{"Dev Flow"}, nil
}

func (f *fakeApplier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

const validExportYAML = `version: 1
type: kandev_workflow
workflows:
  - name: Dev Flow
    steps:
      - name: Todo
        position: 0
        is_start_step: true
`

func setupTestService(t *testing.T, client github.Client) (*Service, *fakeApplier) {
	t.Helper()
	store := setupTestStore(t)
	applier := &fakeApplier{}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return NewService(store, fakeGitHubClients{client: client}, nil, applier, log), applier
}

func configureWorkspace(t *testing.T, svc *Service, workspaceID string) {
	t.Helper()
	_, err := svc.SetConfigForWorkspace(context.Background(), workspaceID, &SetConfigRequest{
		RepoOwner: "acme",
		RepoName:  "flows",
	})
	require.NoError(t, err)
}

func seededMockClient() *github.MockClient {
	mock := github.NewMockClient()
	mock.SeedRepoFile("acme", "flows", "main", DefaultPath+"/dev.yml", []byte(validExportYAML))
	return mock
}

func TestSyncWorkspace_NotConfigured(t *testing.T) {
	svc, _ := setupTestService(t, github.NewMockClient())
	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestSyncWorkspace_AppliesParsedFiles(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")
	applier.result = &workflowservice.SyncApplyResult{Created: []string{"Dev Flow"}}

	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"Dev Flow"}, result.Created)
	assert.False(t, result.Unchanged)

	require.Len(t, applier.calls, 1)
	require.Len(t, applier.calls[0], 1)
	file := applier.calls[0][0]
	assert.Equal(t, DefaultPath+"/dev.yml", file.Path)
	require.NotNil(t, file.Export)
	assert.Equal(t, "Dev Flow", file.Export.Workflows[0].Name)

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.True(t, cfg.LastOk)
	assert.NotNil(t, cfg.LastSyncedAt)
	assert.NotEmpty(t, cfg.LastHash)
	assert.Empty(t, cfg.LastError)
}

func TestSyncWorkspace_AlwaysReconciles(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	// Every sync applies (repairing local edits to synced workflows); the
	// applier reports nothing changed, which surfaces as Unchanged.
	assert.Len(t, applier.calls, 2)
	assert.True(t, result.Unchanged)
}

func TestSyncWorkspace_BrokenFileBecomesWarningAndNilExport(t *testing.T) {
	mock := seededMockClient()
	mock.SeedRepoFile("acme", "flows", "main", DefaultPath+"/broken.yml", []byte("::not yaml::"))
	svc, applier := setupTestService(t, mock)
	configureWorkspace(t, svc, "ws-1")

	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "broken.yml")

	require.Len(t, applier.calls, 1)
	require.Len(t, applier.calls[0], 2)
	byPath := map[string]workflowservice.SyncFileExport{}
	for _, f := range applier.calls[0] {
		byPath[f.Path] = f
	}
	assert.Nil(t, byPath[DefaultPath+"/broken.yml"].Export)
	assert.NotNil(t, byPath[DefaultPath+"/dev.yml"].Export)

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.True(t, cfg.LastOk, "parse warnings do not fail the sync")
	require.Len(t, cfg.LastWarnings, 1)
}

func TestSyncWorkspace_IgnoresNonWorkflowFiles(t *testing.T) {
	mock := seededMockClient()
	mock.SeedRepoFile("acme", "flows", "main", DefaultPath+"/README.md", []byte("# docs"))
	svc, applier := setupTestService(t, mock)
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Len(t, applier.calls, 1)
	assert.Len(t, applier.calls[0], 1, "only .yml/.yaml/.json files are synced")
}

func TestSyncWorkspace_MissingDirectoryRecordsFailure(t *testing.T) {
	svc, _ := setupTestService(t, github.NewMockClient()) // nothing seeded → 404
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.Error(t, err)

	cfg, cfgErr := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, cfgErr)
	assert.False(t, cfg.LastOk)
	assert.NotEmpty(t, cfg.LastError)
	assert.NotNil(t, cfg.LastSyncedAt)
}

func TestSyncWorkspace_NilClientRecordsFailure(t *testing.T) {
	svc, _ := setupTestService(t, nil)
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")

	cfg, cfgErr := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, cfgErr)
	assert.False(t, cfg.LastOk)
}

func TestSyncDueConfigs_HonorsInterval(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool { return applier.callCount() == 1 }, time.Second, time.Millisecond,
		"first sync runs immediately (never synced)")
	require.Len(t, applier.calls, 1, "first sync runs immediately (never synced)")

	svc.SyncDueConfigs(context.Background())
	assert.Len(t, applier.calls, 1, "second run within the interval is skipped entirely")
}

func TestSyncDueConfigs_SkipsPollingDisabled(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	disabled := false
	_, err := svc.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{
		RepoOwner:   "acme",
		RepoName:    "flows",
		PollEnabled: &disabled,
	})
	require.NoError(t, err)

	svc.SyncDueConfigs(context.Background())
	assert.Empty(t, applier.calls, "polling-disabled configs never auto-sync")

	// Manual sync still works.
	_, err = svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Len(t, applier.calls, 1)
}

func TestIsSyncDueSkipsSuspendedAndFutureBackoff(t *testing.T) {
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	cfg := &Config{PollEnabled: true, IntervalSeconds: 60}
	cfg.PollSuspended = true
	assert.False(t, isSyncDue(cfg, now), "suspended polling must not issue a provider request")

	cfg.PollSuspended = false
	nextAttempt := now.Add(time.Minute)
	cfg.NextAttemptAt = &nextAttempt
	assert.False(t, isSyncDue(cfg, now), "backoff window must not issue a provider request")
	assert.True(t, isSyncDue(cfg, nextAttempt), "the config should be eligible at next_attempt_at")
}

func TestSyncDueConfigsPersistsEqualJitterBackoff(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	svc := NewService(store, failingGitHubClients{err: &github.GitHubAPIError{
		StatusCode: 500, Endpoint: "/repos/acme/flows/contents", FailureKind: github.FailureTransient,
	}}, nil, &fakeApplier{}, log)
	configureWorkspace(t, svc, "ws-1")
	before := time.Now().UTC()

	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool {
		cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
		return err == nil && cfg != nil && cfg.ConsecutiveFailures > 0
	}, time.Second, time.Millisecond)

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Equal(t, 1, cfg.ConsecutiveFailures)
	if assert.NotNil(t, cfg.NextAttemptAt) {
		assert.False(t, cfg.NextAttemptAt.Before(before.Add(150*time.Second)))
		assert.False(t, cfg.NextAttemptAt.After(time.Now().UTC().Add(5*time.Minute)))
	}
	assert.Equal(t, string(github.FailureTransient), cfg.LastErrorClass)
}

func TestSyncDueConfigsSkipsProviderUntilNextAttempt(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	provider := &countingGitHubClients{err: &github.GitHubAPIError{
		StatusCode: 500, Endpoint: "/repos/acme/flows/contents", FailureKind: github.FailureTransient,
	}}
	svc := NewService(store, provider, nil, &fakeApplier{}, log)
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	svc.jitter = func(time.Duration) time.Duration { return 0 }
	configureWorkspace(t, svc, "ws-1")

	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool { return provider.callCount() == 1 }, time.Second, time.Millisecond)
	require.Equal(t, 1, provider.callCount())
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	require.NotNil(t, cfg.NextAttemptAt)
	require.Equal(t, now.Add(150*time.Second), *cfg.NextAttemptAt)

	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool { return provider.callCount() == 1 }, time.Second, time.Millisecond)
	require.Equal(t, 1, provider.callCount(), "same-tick retry reached the provider")
	now = *cfg.NextAttemptAt
	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool { return provider.callCount() == 2 }, time.Second, time.Millisecond)
	require.Equal(t, 2, provider.callCount(), "retry did not run at next_attempt_at")
}

func TestPermanentFailureSuspendsPollingUntilManualRecovery(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	provider := &countingGitHubClients{err: &github.GitHubAPIError{
		StatusCode: 404, Endpoint: "/repos/acme/flows/contents", FailureKind: github.FailureMissingResource,
	}}
	svc := NewService(store, provider, nil, &fakeApplier{}, log)
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	configureWorkspace(t, svc, "ws-1")
	suspendedLabel := workflowSyncMetricLabel(
		"transition", "suspended", "provider", ProviderGitHub,
		"failure_class", string(github.FailureMissingResource), "retry_source", "",
	)
	recoveredLabel := workflowSyncMetricLabel(
		"transition", "recovered", "provider", ProviderGitHub,
		"failure_class", string(github.FailureMissingResource), "retry_source", "",
	)
	suspendedBefore := workflowSyncCounterValue(t, suspendedLabel)
	recoveredBefore := workflowSyncCounterValue(t, recoveredLabel)

	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool {
		cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
		return err == nil && cfg != nil && cfg.PollSuspended
	}, time.Second, time.Millisecond)
	require.Equal(t, 1, provider.callCount())
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.True(t, cfg.PollSuspended)
	assert.Equal(t, string(github.FailureMissingResource), cfg.LastErrorClass)
	assert.NotEmpty(t, cfg.PollSuspensionReason)
	assert.Equal(t, int64(1), workflowSyncCounterValue(t, suspendedLabel)-suspendedBefore)

	now = now.Add(24 * time.Hour)
	svc.SyncDueConfigs(context.Background())
	require.Equal(t, 1, provider.callCount(), "suspended tick repeated the provider request")
	assert.Equal(t, int64(1), workflowSyncCounterValue(t, suspendedLabel)-suspendedBefore,
		"skipped suspended ticks must not repeat transition telemetry")

	provider.setError(nil)
	_, err = svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	require.Equal(t, 2, provider.callCount())
	cfg, err = svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.False(t, cfg.PollSuspended)
	assert.Zero(t, cfg.ConsecutiveFailures)
	assert.Nil(t, cfg.NextAttemptAt)
	assert.Empty(t, cfg.LastErrorClass)
	assert.Equal(t, int64(1), workflowSyncCounterValue(t, recoveredLabel)-recoveredBefore)
}

func TestConfigSaveRearmsSuspendedPolling(t *testing.T) {
	store := setupTestStore(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	provider := &countingGitHubClients{err: &github.GitHubAPIError{
		StatusCode: 404, Endpoint: "/repos/acme/flows/contents", FailureKind: github.FailureMissingResource,
	}}
	svc := NewService(store, provider, nil, &fakeApplier{}, log)
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	configureWorkspace(t, svc, "ws-1")

	svc.SyncDueConfigs(context.Background())
	assert.Eventually(t, func() bool {
		cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
		return err == nil && cfg != nil && cfg.PollSuspended
	}, time.Second, time.Millisecond)
	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	require.True(t, cfg.PollSuspended)

	_, err = svc.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{
		RepoOwner: "acme", RepoName: "flows",
	})
	require.NoError(t, err)
	cfg, err = svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.False(t, cfg.PollSuspended)
	assert.Zero(t, cfg.ConsecutiveFailures)
}

func TestDeleteConfigForWorkspace_ReleasesSyncedWorkflows(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	require.NoError(t, svc.DeleteConfigForWorkspace(context.Background(), "ws-1"))
	assert.Equal(t, []string{"ws-1"}, applier.released, "deleting the config releases its workflows")

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}
