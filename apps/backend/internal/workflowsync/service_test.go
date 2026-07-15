package workflowsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/github"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

type fakeClients struct {
	client github.Client
}

func (f fakeClients) Client() github.Client { return f.client }

type fakeApplier struct {
	calls  [][]workflowservice.SyncFileExport
	result *workflowservice.SyncApplyResult
}

func (f *fakeApplier) ApplySyncedWorkflows(_ context.Context, _ string, files []workflowservice.SyncFileExport) (*workflowservice.SyncApplyResult, error) {
	f.calls = append(f.calls, files)
	if f.result != nil {
		return f.result, nil
	}
	return &workflowservice.SyncApplyResult{}, nil
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
	return NewService(store, fakeClients{client: client}, applier, log), applier
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
	_, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestSyncWorkspace_AppliesParsedFiles(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")
	applier.result = &workflowservice.SyncApplyResult{Created: []string{"Dev Flow"}}

	result, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
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

func TestSyncWorkspace_SkipsUnchangedContent(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
	require.NoError(t, err)
	result, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
	require.NoError(t, err)
	assert.True(t, result.Unchanged)
	assert.Len(t, applier.calls, 1, "apply skipped when content hash is unchanged")
}

func TestSyncWorkspace_ForceBypassesHashCheck(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
	require.NoError(t, err)
	result, err := svc.SyncWorkspace(context.Background(), "ws-1", true)
	require.NoError(t, err)
	assert.False(t, result.Unchanged)
	assert.Len(t, applier.calls, 2, "force sync re-applies even when unchanged")
}

func TestSyncWorkspace_BrokenFileBecomesWarningAndNilExport(t *testing.T) {
	mock := seededMockClient()
	mock.SeedRepoFile("acme", "flows", "main", DefaultPath+"/broken.yml", []byte("::not yaml::"))
	svc, applier := setupTestService(t, mock)
	configureWorkspace(t, svc, "ws-1")

	result, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
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

	_, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
	require.NoError(t, err)
	require.Len(t, applier.calls, 1)
	assert.Len(t, applier.calls[0], 1, "only .yml/.yaml/.json files are synced")
}

func TestSyncWorkspace_MissingDirectoryRecordsFailure(t *testing.T) {
	svc, _ := setupTestService(t, github.NewMockClient()) // nothing seeded → 404
	configureWorkspace(t, svc, "ws-1")

	_, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
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

	_, err := svc.SyncWorkspace(context.Background(), "ws-1", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")

	cfg, cfgErr := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, cfgErr)
	assert.False(t, cfg.LastOk)
}

func TestSyncDueConfigs_HonorsInterval(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	svc.SyncDueConfigs(context.Background())
	require.Len(t, applier.calls, 1, "first sync runs immediately (never synced)")

	svc.SyncDueConfigs(context.Background())
	assert.Len(t, applier.calls, 1, "second run within the interval is skipped entirely")
}
