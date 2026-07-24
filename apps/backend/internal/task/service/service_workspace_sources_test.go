package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
)

func TestAttachWorkspaceSourcesDerivesLegacyPrimaryBranchForWorktreeProjection(t *testing.T) {
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
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-legacy-primary", WorkspaceID: "ws-legacy-primary", Name: "primary", LocalPath: primaryPath}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-legacy-primary", WorkflowID: "wf-legacy-primary", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-legacy-primary"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-legacy-primary", TaskID: task.ID, ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	addedPath := filepath.Join(t.TempDir(), "added")
	seedBareGitDir(t, addedPath, "ref: refs/heads/main\n")
	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{
		{Kind: WorkspaceSourceRepository, LocalPath: addedPath, BaseBranch: "main"},
		{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "notes"},
	}})
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

func TestAttachWorkspaceSourcesRejectsDetachedLegacyPrimaryAtomically(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-detached-primary", Name: "Detached"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-detached-primary", WorkspaceID: "ws-detached-primary", Name: "WF"}); err != nil {
		t.Fatal(err)
	}
	primaryPath := t.TempDir()
	seedBareGitDir(t, primaryPath, "c0ffee\n")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-detached-primary", WorkspaceID: "ws-detached-primary", Name: "primary", LocalPath: primaryPath}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-detached-primary", WorkflowID: "wf-detached-primary", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-detached-primary"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-detached-primary", TaskID: task.ID, ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: t.TempDir()}); err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "notes"}}})
	if !errors.Is(err, ErrInvalidWorkspaceSource) {
		t.Fatalf("error = %v, want invalid workspace source", err)
	}
	links, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if links[0].BaseBranch != "" {
		t.Fatalf("legacy primary base branch = %q, want unchanged empty branch", links[0].BaseBranch)
	}
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Fatalf("folders = %#v, want no attachment mutation", folders)
	}
}

type failingWorkspaceSourceMaterializer struct{}

func (failingWorkspaceSourceMaterializer) MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	return nil, errors.New("materialize failed")
}

type cancelingWorkspaceSourceMaterializer struct{ cancel context.CancelFunc }

func (m cancelingWorkspaceSourceMaterializer) MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	m.cancel()
	return &WorkspaceSourceMaterializationResult{}, nil
}

type resultWorkspaceSourceMaterializer struct {
	result *WorkspaceSourceMaterializationResult
}

func (m resultWorkspaceSourceMaterializer) MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	return m.result, nil
}

type recordingWorkspaceSourceMaterializer struct{ called bool }

func (m *recordingWorkspaceSourceMaterializer) MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	m.called = true
	return &WorkspaceSourceMaterializationResult{}, nil
}

type blockingWorkspaceSourceMaterializer struct {
	started chan struct{}
	release chan struct{}
}

func (m blockingWorkspaceSourceMaterializer) MaterializeWorkspaceSources(context.Context, string, *models.WorkspaceSourceBatch) (*WorkspaceSourceMaterializationResult, error) {
	close(m.started)
	<-m.release
	return &WorkspaceSourceMaterializationResult{}, nil
}

func TestAttachWorkspaceSources_CancellationBeforeMaterializationPreventsMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-cancel-before-materialize", Name: "Cancel"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-cancel-before-materialize", WorkspaceID: "ws-cancel-before-materialize", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-cancel-before-materialize", WorkflowID: "wf-cancel-before-materialize", WorkflowStepID: "step", Title: "Task"})
	if err != nil {
		t.Fatal(err)
	}
	materializer := &recordingWorkspaceSourceMaterializer{}
	svc.SetWorkspaceSourceMaterializer(materializer)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.AttachWorkspaceSources(canceled, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}}); err == nil {
		t.Fatal("AttachWorkspaceSources succeeded with canceled context")
	}
	if materializer.called {
		t.Fatal("materializer ran after cancellation")
	}
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Fatalf("folders after canceled request = %#v", folders)
	}
}

func TestAttachWorkspaceSources_RejectsRepositorylessTaskBeforePersistingFolders(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-repositoryless", Name: "Repositoryless"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-repositoryless", WorkspaceID: "ws-repositoryless", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-repositoryless", WorkflowID: "wf-repositoryless", WorkflowStepID: "step", Title: "Task"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}})
	if !errors.Is(err, ErrInvalidWorkspaceSource) {
		t.Fatalf("error = %v, want invalid workspace source", err)
	}
	folders, err := repo.ListTaskWorkspaceFolders(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Fatalf("folders after rejected attachment = %#v", folders)
	}
}

func TestAttachWorkspaceSources_RejectsUnsafeBranchBeforeCreatingRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-unsafe-source", Name: "Unsafe"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-unsafe-source", WorkspaceID: "ws-unsafe-source", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-primary", WorkspaceID: "ws-unsafe-source", Name: "primary", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-unsafe-source", WorkflowID: "wf-unsafe-source", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-primary", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, LocalPath: t.TempDir(), BaseBranch: "--upload-pack=evil"}}})
	if !errors.Is(err, ErrInvalidWorkspaceSource) {
		t.Fatalf("error = %v, want invalid workspace source", err)
	}
	repositories, listErr := repo.ListRepositories(ctx, "ws-unsafe-source")
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(repositories) != 1 {
		t.Fatalf("repositories = %#v, want only primary repository", repositories)
	}
	links, listErr := repo.ListTaskRepositories(ctx, task.ID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(links) != 1 {
		t.Fatalf("task repository links = %#v, want only primary link", links)
	}
}

func TestResolveRepositoryRef_RejectsUntrustedGitLabOrigins(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-gitlab-origin", Name: "Origin"}); err != nil {
		t.Fatal(err)
	}
	for _, remoteURL := range []string{
		"http://gitlab.com/acme/app.git",
		"https://localhost/acme/app.git",
		"https://127.0.0.1/acme/app.git",
		"https://10.0.0.1/acme/app.git",
	} {
		t.Run(remoteURL, func(t *testing.T) {
			_, _, _, err := svc.ResolveRepositoryRef(ctx, "ws-gitlab-origin", TaskRepositoryInput{RemoteURL: remoteURL, Provider: "gitlab", BaseBranch: "main"})
			if err == nil {
				t.Fatalf("ResolveRepositoryRef accepted untrusted GitLab origin %q", remoteURL)
			}
		})
	}
}

func TestAttachWorkspaceSourcesReturnsAuthoritativeMaterializationResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-authoritative-result", Name: "Result"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-authoritative-result", WorkspaceID: "ws-authoritative-result", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-authoritative-result", WorkflowID: "wf-authoritative-result", WorkflowStepID: "step", Title: "Task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-authoritative-result", TaskID: task.ID, WorkspacePath: "/stale/workspace"}); err != nil {
		t.Fatal(err)
	}
	initialRepository := t.TempDir()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-authoritative-result", WorkspaceID: "ws-authoritative-result", Name: "initial", LocalPath: initialRepository}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repo-authoritative-result", TaskID: task.ID, RepositoryID: "repo-authoritative-result", BaseBranch: "main", Position: 0, Metadata: map[string]interface{}{}}); err != nil {
		t.Fatal(err)
	}
	svc.SetWorkspaceSourceMaterializer(resultWorkspaceSourceMaterializer{result: &WorkspaceSourceMaterializationResult{WorkspacePath: "/adopted/workspace", SessionIDs: []string{"adopted-session"}}})
	result, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkspacePath != "/adopted/workspace" || len(result.SessionIDs) != 1 || result.SessionIDs[0] != "adopted-session" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAttachWorkspaceSources_PublishesTaskBeforeSessionAdoption(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	svc.workspaceFolders = repo
	svc.SetWorkspaceSourceMaterializer(resultWorkspaceSourceMaterializer{result: &WorkspaceSourceMaterializationResult{WorkspacePath: "/workspace/task", SessionIDs: []string{"session-1"}}})
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-event-order", Name: "Events"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-event-order", WorkspaceID: "ws-event-order", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-event-order", WorkspaceID: "ws-event-order", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-event-order", WorkflowID: "wf-event-order", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-event-order", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()

	if _, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}}); err != nil {
		t.Fatal(err)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 2 {
		t.Fatalf("published events = %#v, want task.updated then workspace adoption", published)
	}
	if published[0].Type != events.TaskUpdated || published[1].Type != events.SessionWorkspaceSourcesUpdated {
		t.Fatalf("published event types = %q, %q", published[0].Type, published[1].Type)
	}
}

func TestAttachWorkspaceSources_HoldsTaskMutationGateThroughMaterialization(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	materializer := blockingWorkspaceSourceMaterializer{started: make(chan struct{}), release: make(chan struct{})}
	svc.SetWorkspaceSourceMaterializer(materializer)
	released := false
	defer func() {
		if !released {
			close(materializer.release)
		}
	}()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-mutation-gate", Name: "Gate"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-mutation-gate", WorkspaceID: "ws-mutation-gate", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-mutation-gate", WorkspaceID: "ws-mutation-gate", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-mutation-gate", WorkflowID: "wf-mutation-gate", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-mutation-gate", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-mutation-gate", TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	attachDone := make(chan error, 1)
	go func() {
		_, attachErr := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: folder, DisplayName: "docs"}}})
		attachDone <- attachErr
	}()
	select {
	case <-materializer.started:
	case attachErr := <-attachDone:
		t.Fatalf("AttachWorkspaceSources before materialization: %v", attachErr)
	case <-time.After(time.Second):
		t.Fatal("AttachWorkspaceSources did not reach materialization")
	}
	turnDone := make(chan error, 1)
	go func() {
		_, turnErr := svc.StartTurn(ctx, "session-mutation-gate")
		turnDone <- turnErr
	}()
	select {
	case err := <-turnDone:
		t.Fatalf("StartTurn completed during source materialization: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(materializer.release)
	released = true
	if err := <-attachDone; err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	if err := <-turnDone; err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
}

func TestAttachWorkspaceSources_CommitsWhenCancellationArrivesAfterMaterialization(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx, cancel := context.WithCancel(context.Background())
	svc.SetWorkspaceSourceMaterializer(cancelingWorkspaceSourceMaterializer{cancel: cancel})
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-cancel-after-materialize", Name: "Cancel"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-cancel-after-materialize", WorkspaceID: "ws-cancel-after-materialize", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-cancel-after-materialize", WorkspaceID: "ws-cancel-after-materialize", Name: "app", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-cancel-after-materialize", WorkflowID: "wf-cancel-after-materialize", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-cancel-after-materialize", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}}}); err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	folders, err := repo.ListTaskWorkspaceFolders(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 {
		t.Fatalf("folders after committed materialization = %#v", folders)
	}
}

func TestAttachWorkspaceSourcesPersistsMixedSourcesInRequestOrder(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.workspaceFolders = repo
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-sources", Name: "Sources"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-sources", WorkspaceID: "ws-sources", Name: "Sources"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-sources", WorkspaceID: "ws-sources", Name: "app", LocalPath: t.TempDir(), DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-sources", WorkflowID: "wf-sources", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-sources", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-sources", TaskID: task.ID, WorkspacePath: "/workspace/task"}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceFolder, LocalPath: t.TempDir(), DisplayName: "docs"}, {Kind: WorkspaceSourceRepository, RepositoryID: "repo-sources", BaseBranch: "release", CheckoutBranch: "feature/x"}}})
	if err != nil {
		t.Fatalf("AttachWorkspaceSources: %v", err)
	}
	if len(got.Task.WorkspaceFolders) != 1 || len(got.Task.Repositories) != 2 {
		t.Fatalf("sources = folders %d repos %d", len(got.Task.WorkspaceFolders), len(got.Task.Repositories))
	}
	if got.Task.WorkspaceFolders[0].Position != 1 || got.Task.Repositories[1].Position != 2 {
		t.Fatalf("positions = folder %d repo %d, want 1,2", got.Task.WorkspaceFolders[0].Position, got.Task.Repositories[1].Position)
	}
	if got.WorkspacePath != "/workspace/task" || len(got.SessionIDs) != 0 {
		t.Fatalf("result workspace/session projection = %#v", got)
	}
}

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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-rollback", WorkflowID: "wf-rollback", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-rollback", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-local-pc", WorkflowID: "wf-local-pc", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-local-pc", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-remote-folder", WorkflowID: "wf-remote-folder", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-remote-folder", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-prelaunch", WorkflowID: "wf-prelaunch", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-prelaunch", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-local-checkout", WorkflowID: "wf-local-checkout", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-local-primary", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-local-base", WorkflowID: "wf-local-base", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-local-base", BaseBranch: "main"}}})
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

func TestAttachWorkspaceSourcesRejectsDuplicateRepositoryWithinBatch(t *testing.T) {
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-dup", WorkflowID: "wf-dup", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-dup", BaseBranch: "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AttachWorkspaceSources(ctx, AttachWorkspaceSourcesRequest{TaskID: task.ID, Sources: []WorkspaceSourceInput{{Kind: WorkspaceSourceRepository, RepositoryID: "repo-dup", BaseBranch: "release", CheckoutBranch: "feature/x"}, {Kind: WorkspaceSourceRepository, RepositoryID: "repo-dup", BaseBranch: "release", CheckoutBranch: "feature/x"}}})
	if err == nil {
		t.Fatal("AttachWorkspaceSources succeeded, want duplicate error")
	}
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-slug", WorkflowID: "wf-slug", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-slug", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-owner", WorkflowID: "wf-owner", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-owner", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-runtime", WorkflowID: "wf-runtime", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-runtime", BaseBranch: "main"}}})
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
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "ws-new-runtime", WorkflowID: "wf-new-runtime", WorkflowStepID: "step", Title: "Task", Repositories: []TaskRepositoryInput{{RepositoryID: "repo-primary-runtime", BaseBranch: "main"}}})
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
