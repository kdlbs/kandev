package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestAttachWorkspaceSourcesCompensatesOnMaterializationFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	svc.SetWorkspaceSourceMaterializer(failingWorkspaceSourceMaterializer{})
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-rollback", Name: "Rollback"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-rollback", WorkspaceID: "ws-rollback", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-rollback", WorkspaceID: "ws-rollback", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-rollback", WorkflowID: "wf-rollback", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-rollback", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}}); err == nil {
		t.Fatal("AttachWorkspaceSources succeeded, want materialization error")
	}
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Fatalf("folders after rollback = %#v", folders)
	}
}

func TestAttachWorkspaceSourcesAllowsFolderForLocalPCAlias(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-local-pc", Name: "Local PC"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-local-pc", WorkspaceID: "ws-local-pc", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-local-pc", WorkspaceID: "ws-local-pc", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-local-pc", WorkflowID: "wf-local-pc", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-local-pc", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-local-pc", TaskID: task.ID, ExecutorType: "local_pc"}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}}); err != nil {
		t.Fatalf("AttachWorkspaceSources for local_pc: %v", err)
	}
}

func TestAttachWorkspaceSourcesRejectsFolderForRemoteExecutorBeforeResolvingPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-remote-folder", Name: "Remote Folder"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-remote-folder", WorkspaceID: "ws-remote-folder", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-remote-folder", WorkspaceID: "ws-remote-folder", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-remote-folder", WorkflowID: "wf-remote-folder", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-remote-folder", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-remote-folder", TaskID: task.ID, ExecutorType: "remote_docker"}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: filepath.Join(t.TempDir(), "not-present"), DisplayName: "docs"}}})
	if !errors.Is(err, ErrUnsupportedWorkspaceSource) {
		t.Fatalf("error = %v, want unsupported workspace source", err)
	}
}

func TestAttachWorkspaceSources_PersistsPrelaunchSourcesWithExplicitDeferredResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	svc.SetWorkspaceSourceMaterializer(resultWorkspaceSourceMaterializer{result: &WorkspaceSourceMaterializationResult{}})
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-prelaunch", Name: "Prelaunch"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-prelaunch", WorkspaceID: "ws-prelaunch", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-prelaunch", WorkspaceID: "ws-prelaunch", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-prelaunch", WorkflowID: "wf-prelaunch", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-prelaunch", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}}); err != nil {
		t.Fatalf("AttachWorkspaceSources before launch: %v", err)
	}
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].DisplayName != "docs" {
		t.Fatalf("deferred folders = %#v", folders)
	}
}

func TestAttachWorkspaceSources_RejectsCheckoutBranchForLocalRuntime(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-local-checkout", Name: "Local"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-local-checkout", WorkspaceID: "ws-local-checkout", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	for _, repository := range []*models.Repository{
		{ID: "repo-local-primary", WorkspaceID: "ws-local-checkout", Name: "primary", DefaultBranch: "main"},
		{ID: "repo-local-added", WorkspaceID: "ws-local-checkout", Name: "added", DefaultBranch: "main"},
	} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatal(err)
		}
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-local-checkout", WorkflowID: "wf-local-checkout", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-local-primary", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-local-checkout", TaskID: task.ID, ExecutorType: string(models.ExecutorTypeLocal)}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RepositoryID: "repo-local-added", BaseBranch: "main", CheckoutBranch: "feature/source"}}})
	if !errors.Is(err, ErrInvalidWorkspaceSource) {
		t.Fatalf("error = %v, want invalid workspace source", err)
	}
}

func TestAttachWorkspaceSources_RejectsSameRepositoryDifferentBaseOnLocalRuntime(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-local-base", Name: "Local"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-local-base", WorkspaceID: "ws-local-base", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-local-base", WorkspaceID: "ws-local-base", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-local-base", WorkflowID: "wf-local-base", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-local-base", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-local-base", TaskID: task.ID, ExecutorType: "local_pc"}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RepositoryID: "repo-local-base", BaseBranch: "release"}}})
	if !errors.Is(err, ErrWorkspaceSourceConflict) {
		t.Fatalf("error = %v, want local runtime-name conflict", err)
	}
}

func TestAttachWorkspaceSourcesCollapsesDuplicateRepositoryWithinBatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-dup", Name: "Dup"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-dup", WorkspaceID: "ws-dup", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-dup", WorkspaceID: "ws-dup", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-dup", WorkflowID: "wf-dup", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-dup", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RepositoryID: "repo-dup", BaseBranch: "release", CheckoutBranch: "feature/x"}, {Kind: WorkspaceSourceRepository, RepositoryID: "repo-dup", BaseBranch: "release", CheckoutBranch: "feature/x"}}})
	if err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("task repositories = %d, want primary plus one attachment", len(rows))
	}
}

func TestAttachWorkspaceSourcesExactRetriesSkipRuntimeSideEffects(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-idempotent-retry", Name: "Retry"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-idempotent-retry", WorkspaceID: "ws-idempotent-retry", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-idempotent-retry", WorkspaceID: "ws-idempotent-retry", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-idempotent-retry", WorkflowID: "wf-idempotent-retry", Title: "Task",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-idempotent-retry", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	task := taskResult.Task
	source := WorkspaceSourceInput{Kind: WorkspaceSourceRepository, RepositoryID: "repo-idempotent-retry", BaseBranch: "release", CheckoutBranch: "feature/retry"}
	if _, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}}); err != nil {
		t.Fatalf("initial attachment: %v", err)
	}
	eventBus.ClearEvents()
	materializer := &recordingWorkspaceSourceMaterializer{}
	refresher := &recordingWorkspaceSourceProviderRefresher{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)

	result, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
	if err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if result.Changed || materializer.called || refresher.calls != 0 || len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("exact retry changed=%t materialized=%t refreshes=%d events=%d, want no side effects", result.Changed, materializer.called, refresher.calls, len(eventBus.GetPublishedEvents()))
	}
	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("task repositories after exact retry = %d, want 2", len(rows))
	}
}

func TestAttachWorkspaceSourcesExactFolderRetryCanonicalizesPathWithoutSideEffects(t *testing.T) {
	svc, eventBus, repo, task := newWorkspaceSourceRetryFixture(t)
	ctx := context.Background()
	folder := t.TempDir()
	source := WorkspaceSourceInput{Kind: WorkspaceSourceFolder, LocalPath: folder, DisplayName: "docs"}
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
	require.NoError(t, err)

	eventBus.ClearEvents()
	materializer := &recordingWorkspaceSourceMaterializer{}
	refresher := &recordingWorkspaceSourceProviderRefresher{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	retry, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{
		Kind: WorkspaceSourceFolder, LocalPath: workspaceSourceTestSymlink(t, folder), DisplayName: "docs",
	}}})
	require.NoError(t, err)
	require.False(t, retry.Changed)
	require.False(t, materializer.called)
	require.Zero(t, refresher.calls)
	require.Empty(t, eventBus.GetPublishedEvents())
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, folders, 1)
}

func TestAttachWorkspaceSourcesExactFolderRetryIgnoresExecutorChange(t *testing.T) {
	svc, _, repo, task := newWorkspaceSourceRetryFixture(t)
	ctx := context.Background()
	folder := t.TempDir()
	source := WorkspaceSourceInput{Kind: WorkspaceSourceFolder, LocalPath: folder, DisplayName: "docs"}
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
	require.NoError(t, err)

	// A task can switch executor profiles between launches. An exact retry must
	// remain an idempotent no-op even when the new executor cannot accept folders.
	require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-retry-remote", TaskID: task.ID, ExecutorType: "remote_docker",
	}))
	retry, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
	require.NoError(t, err)
	require.False(t, retry.Changed)
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, folders, 1)
}

func TestAttachWorkspaceSourcesMixedDuplicateAndNewFoldersCommitsOnlyNewSource(t *testing.T) {
	svc, eventBus, repo, task := newWorkspaceSourceRetryFixture(t)
	ctx := context.Background()
	existing := t.TempDir()
	newFolder := t.TempDir()
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{
		Kind: WorkspaceSourceFolder, LocalPath: existing, DisplayName: "existing",
	}}})
	require.NoError(t, err)

	eventBus.ClearEvents()
	materializer := &recordingWorkspaceSourceMaterializer{}
	refresher := &recordingWorkspaceSourceProviderRefresher{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	result, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{
		{Kind: WorkspaceSourceFolder, LocalPath: workspaceSourceTestSymlink(t, existing), DisplayName: "existing"},
		{Kind: WorkspaceSourceFolder, LocalPath: newFolder, DisplayName: "new"},
	}})
	require.NoError(t, err)
	require.True(t, result.Changed)
	require.True(t, materializer.called)
	require.Equal(t, 1, refresher.calls)
	require.NotEmpty(t, eventBus.GetPublishedEvents())
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, folders, 2)
	require.Equal(t, "existing", folders[0].DisplayName)
	require.Equal(t, "new", folders[1].DisplayName)
}

func TestAttachWorkspaceSourcesRejectsContradictoryFolderDuplicates(t *testing.T) {
	svc, eventBus, repo, task := newWorkspaceSourceRetryFixture(t)
	ctx := context.Background()
	folder := t.TempDir()
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{
		Kind: WorkspaceSourceFolder, LocalPath: folder, DisplayName: "docs",
	}}})
	require.NoError(t, err)

	for _, testCase := range []struct {
		name   string
		source WorkspaceSourceInput
	}{
		{name: "same path different name", source: WorkspaceSourceInput{Kind: WorkspaceSourceFolder, LocalPath: workspaceSourceTestSymlink(t, folder), DisplayName: "renamed"}},
		{name: "same name different path", source: WorkspaceSourceInput{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			eventBus.ClearEvents()
			result, attachErr := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{testCase.source}})
			require.ErrorIs(t, attachErr, ErrWorkspaceSourceConflict)
			require.Nil(t, result)
			require.Empty(t, eventBus.GetPublishedEvents())
			folders, listErr := repo.ListTaskWorkspaceFolders(ctx, task.ID)
			require.NoError(t, listErr)
			require.Len(t, folders, 1)
		})
	}
}

func workspaceSourceTestSymlink(t *testing.T, target string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "folder-link")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("create workspace-source test symlink: %v", err)
	}
	return path
}

func TestAttachWorkspaceSourcesExactRepositoryRetryNormalizesDefaultBaseBranch(t *testing.T) {
	svc, eventBus, repo, task := newWorkspaceSourceRetryFixture(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repo-retry-added", WorkspaceID: "ws-retry", Name: "added", DefaultBranch: "main"}))
	source := WorkspaceSourceInput{Kind: WorkspaceSourceRepository, RepositoryID: "repo-retry-added", CheckoutBranch: "feature/retry"}
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
	require.NoError(t, err)

	eventBus.ClearEvents()
	materializer := &recordingWorkspaceSourceMaterializer{}
	refresher := &recordingWorkspaceSourceProviderRefresher{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	svc.SetWorkspaceSourceProviderRefresher(refresher)
	retry, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{
		Kind: WorkspaceSourceRepository, RepositoryID: "repo-retry-added", BaseBranch: "main", CheckoutBranch: "feature/retry",
	}}})
	require.NoError(t, err)
	require.False(t, retry.Changed)
	require.False(t, materializer.called)
	require.Zero(t, refresher.calls)
	require.Empty(t, eventBus.GetPublishedEvents())
	repositories, err := repo.ListTaskRepositories(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, repositories, 2)
}

func TestAttachWorkspaceSourcesConcurrentExactFolderRetriesRemainNoOps(t *testing.T) {
	svc, eventBus, repo, task := newWorkspaceSourceRetryFixture(t)
	ctx := context.Background()
	folder := t.TempDir()
	source := WorkspaceSourceInput{Kind: WorkspaceSourceFolder, LocalPath: folder, DisplayName: "docs"}
	_, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
	require.NoError(t, err)
	eventBus.ClearEvents()

	results := make(chan *AttachWorkspaceSourcesResult, 2)
	errors := make(chan error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, attachErr := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{source}})
			results <- result
			errors <- attachErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errors)
	for attachErr := range errors {
		require.NoError(t, attachErr)
	}
	for result := range results {
		require.NotNil(t, result)
		require.False(t, result.Changed)
	}
	require.Empty(t, eventBus.GetPublishedEvents())
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	require.NoError(t, err)
	require.Len(t, folders, 1)
}

func newWorkspaceSourceRetryFixture(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, *models.Task) {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-retry", Name: "Retry"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-retry", WorkspaceID: "ws-retry", Name: "Workflow"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repo-retry-primary", WorkspaceID: "ws-retry", Name: "primary", DefaultBranch: "main"}))
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-retry", WorkflowID: "wf-retry", WorkflowStepID: "step", Title: "Retry task",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-retry-primary", BaseBranch: "main"}},
	})
	require.NoError(t, err)
	return svc, eventBus, repo, created.Task
}

func TestAttachWorkspaceSourcesRejectsSanitizedBranchCollisionWithinBatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-slug", Name: "Slug"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-slug", WorkspaceID: "ws-slug", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-slug", WorkspaceID: "ws-slug", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-slug", WorkflowID: "wf-slug", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-slug", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{
		{Kind: WorkspaceSourceRepository, RepositoryID: "repo-slug", BaseBranch: "main", CheckoutBranch: "feature/a"},
		{Kind: WorkspaceSourceRepository, RepositoryID: "repo-slug", BaseBranch: "release", CheckoutBranch: "feature-a"},
	}})
	if err == nil {
		t.Fatal("AttachWorkspaceSources succeeded, want sanitized worktree-path conflict")
	}
}

func TestAttachWorkspaceSourcesClassifiesCrossWorkspaceRepositoryAsNotFound(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{{ID: "ws-owner", Name: "Owner"}, {ID: "ws-other", Name: "Other"}} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-owner", WorkspaceID: "ws-owner", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-owner", WorkspaceID: "ws-owner", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-other", WorkspaceID: "ws-other", Name: "other", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-owner", WorkflowID: "wf-owner", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-owner", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RepositoryID: "repo-other", BaseBranch: "main"}}})
	if !errors.Is(err, taskrepository.ErrRepositoryNotFound) {
		t.Fatalf("error = %v, want errors.Is(ErrRepositoryNotFound)", err)
	}
}

func TestAttachWorkspaceSourcesRejectsFolderNameCollidingWithRepositoryRuntimeDirectory(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-runtime", Name: "Runtime"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-runtime", WorkspaceID: "ws-runtime", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-runtime", WorkspaceID: "ws-runtime", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-runtime", WorkflowID: "wf-runtime", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-runtime", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "app-main"}}})
	if !errors.Is(err, ErrWorkspaceSourceConflict) {
		t.Fatalf("error = %v, want runtime-name conflict", err)
	}
}

func TestAttachWorkspaceSourcesRejectsNewRepositoriesWithSameRuntimeName(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-new-runtime", Name: "Runtime"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-new-runtime", WorkspaceID: "ws-new-runtime", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-primary-runtime", WorkspaceID: "ws-new-runtime", Name: "primary", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-new-runtime", WorkflowID: "wf-new-runtime", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-primary-runtime", BaseBranch: "main"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{
		{Kind: WorkspaceSourceRepository, GitHubURL: "https://github.com/a-b/repo", BaseBranch: "main"},
		{Kind: WorkspaceSourceRepository, GitHubURL: "https://github.com/a/b-repo", BaseBranch: "main"},
	}})
	if !errors.Is(err, ErrWorkspaceSourceConflict) {
		t.Fatalf("error = %v, want runtime-name conflict", err)
	}
	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("task repositories = %d, want no durable attachment mutation", len(rows))
	}
}

func TestAttachWorkspaceSourcesExplicitPlacementBackfillsLegacyBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-legacy-primary", Name: "Legacy"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-legacy-primary", WorkspaceID: "ws-legacy-primary", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	primaryPath := t.TempDir()
	seedBareGitDir(t, primaryPath, "ref: refs/heads/main\n")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-legacy-primary", WorkspaceID: "ws-legacy-primary", Name: "primary", LocalPath: canonicalRepoTestPath(t, primaryPath)}); err != nil {
		t.Fatal(err)
	}
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-legacy-primary", WorkflowID: "wf-legacy-primary", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-legacy-primary"}}})
	task := taskResult.Task
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-legacy-primary", TaskID: task.ID, ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: t.TempDir(), TaskDirName: "task-legacy"}); err != nil {
		t.Fatal(err)
	}

	addedPath := filepath.Join(t.TempDir(), "added")
	seedBareGitDir(t, addedPath, "ref: refs/heads/main\n")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-added", WorkspaceID: task.WorkspaceID, Name: "added", LocalPath: addedPath, DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	sources := []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RepositoryID: "repo-added", BaseBranch: "main"}}
	preview, err := svc.PreviewWorkspaceRepositoryPlacement(ctx, task.ID, sources, WorkspacePlacementCurrentRoot)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: sources, RepositoryPlacement: WorkspacePlacementCurrentRoot, PreviewRevision: preview.Revision})

	if err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	links, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if links[0].BaseBranch != "main" {
		t.Fatalf("legacy primary base branch = %q, want main", links[0].BaseBranch)
	}
}
