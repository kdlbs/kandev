package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
)

type projectRepositorySourceReaderFunc func(context.Context, string) (ProjectRepositorySources, error)

func (f projectRepositorySourceReaderFunc) ReadProjectRepositorySources(
	ctx context.Context,
	projectID string,
) (ProjectRepositorySources, error) {
	return f(ctx, projectID)
}

type officeProjectLookup interface {
	GetProject(context.Context, string) (*officemodels.Project, error)
}

type officeProjectRepositorySourceReaderForTest struct {
	reader officeProjectLookup
}

func (r officeProjectRepositorySourceReaderForTest) ReadProjectRepositorySources(
	ctx context.Context,
	projectID string,
) (ProjectRepositorySources, error) {
	project, err := r.reader.GetProject(ctx, projectID)
	if err != nil {
		return ProjectRepositorySources{}, err
	}
	sources, err := officemodels.DecodeRepositories(project.Repositories)
	if err != nil {
		return ProjectRepositorySources{}, err
	}
	return ProjectRepositorySources{WorkspaceID: project.WorkspaceID, Sources: sources}, nil
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.1, AC-TASKS-PROJECT-REPOSITORIES-001.2
func TestCreateTaskRootProjectRepositories(t *testing.T) {
	ctx := context.Background()
	svc, db := createOfficeIntegrationServiceWithDB(t)
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("open Office repository: %v", err)
	}
	svc.SetProjectRepositorySourceReader(officeProjectRepositorySourceReaderForTest{reader: officeRepo})
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-project-repositories", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := svc.workflows.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-project-repositories", WorkspaceID: "ws-project-repositories", Name: "Office",
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	sourcePath := t.TempDir()
	initRealGitRepo(t, sourcePath)
	canonicalPath := canonicalRepoTestPath(t, sourcePath)
	if err := svc.repoEntities.CreateRepository(ctx, &models.Repository{
		ID: "repo-project-repositories", WorkspaceID: "ws-project-repositories", Name: "source",
		SourceType: sourceTypeLocal, LocalPath: canonicalPath, DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("register repository: %v", err)
	}
	encodedSources, err := officemodels.EncodeRepositories([]string{sourcePath})
	if err != nil {
		t.Fatalf("encode project repositories: %v", err)
	}
	if err := officeRepo.CreateProject(ctx, &officemodels.Project{
		ID: "project-repositories", WorkspaceID: "ws-project-repositories", Name: "Project",
		Status: officemodels.ProjectStatusActive, Repositories: encodedSources,
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-project-repositories", WorkflowID: "wf-project-repositories",
		Title: "Project task", ProjectID: "project-repositories",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	attached, err := svc.taskRepos.ListTaskRepositories(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("list task repositories: %v", err)
	}
	if len(attached) != 1 || attached[0].RepositoryID != "repo-project-repositories" {
		t.Fatalf("task repositories = %#v, want registered project repository attached", attached)
	}
	var createdEvent map[string]interface{}
	for _, event := range svc.eventBus.(*MockEventBus).GetPublishedEvents() {
		if event.Type != "task.created" {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if ok && data["task_id"] == created.Task.ID {
			createdEvent = data
			break
		}
	}
	if createdEvent == nil {
		t.Fatal("task.created event was not published")
	}
	eventRepositories, ok := createdEvent["repositories"].([]map[string]interface{})
	if !ok || len(eventRepositories) != 1 || eventRepositories[0]["repository_id"] != "repo-project-repositories" {
		t.Fatalf("task.created repositories = %#v, want project repository persisted before publication", createdEvent["repositories"])
	}
}

func TestCreateTaskProjectRepositorySourceSelectionPrecedence(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	sourcePath := t.TempDir()
	initRealGitRepo(t, sourcePath)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-explicit-project", WorkspaceID: "ws-1", Name: "explicit",
		SourceType: sourceTypeLocal, LocalPath: canonicalRepoTestPath(t, sourcePath), DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("register explicit repository: %v", err)
	}
	readerCalls := 0
	svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		readerCalls++
		return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{sourcePath}}, nil
	}))
	workflowID := "wf-project-source-precedence"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: "ws-1", Name: "Board"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	cases := []struct {
		name      string
		request   CreateTaskRequest
		wantRepos int
	}{
		{
			name:      "explicit subset",
			request:   CreateTaskRequest{Repositories: []TaskRepositoryInput{{RepositoryID: "repo-explicit-project"}}},
			wantRepos: 1,
		},
		{
			name:    "explicit empty selection",
			request: CreateTaskRequest{Repositories: []TaskRepositoryInput{}},
		},
		{
			name:    "explicit workspace path",
			request: CreateTaskRequest{WorkspacePath: t.TempDir()},
		},
		{
			name:    "shared workspace policy",
			request: CreateTaskRequest{WorkspacePolicy: &WorkspacePolicy{Mode: workspaceModeSharedGroup, GroupID: "existing-group"}},
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := tc.request
			request.WorkspaceID = "ws-1"
			request.WorkflowID = workflowID
			request.Title = fmt.Sprintf("Project selection %d", i)
			request.ProjectID = "project-defaults"
			created, err := svc.CreateTask(ctx, &request)
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			attached, err := repo.ListTaskRepositories(ctx, created.Task.ID)
			if err != nil {
				t.Fatalf("ListTaskRepositories: %v", err)
			}
			if len(attached) != tc.wantRepos {
				t.Fatalf("attached repositories = %d, want %d: %#v", len(attached), tc.wantRepos, attached)
			}
		})
	}
	if readerCalls != 0 {
		t.Fatalf("project reader calls = %d, want 0 for explicit selections", readerCalls)
	}
}

func TestCreateTaskWithoutProjectDoesNotRequireProjectSourceReader(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	svc.SetProjectRepositorySourceReader(nil)
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-no-project-sources", WorkspaceID: "ws-1", Name: "Board"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-no-project-sources", Title: "No project selected",
	})
	if err != nil {
		t.Fatalf("CreateTask without project source reader: %v", err)
	}
	if len(created.Task.Repositories) != 0 {
		t.Fatalf("task repositories = %#v, want no automatic sources", created.Task.Repositories)
	}
}

func TestCreateTaskProjectRepositorySourceFailuresPrecedeTaskCreation(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name             string
		reader           ProjectRepositorySourceReader
		errorNotContains string
	}{
		{name: "missing reader"},
		{name: "read failure", reader: projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{}, errors.New("project store unavailable")
		})},
		{name: "foreign workspace", errorNotContains: "ws-foreign", reader: projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{WorkspaceID: "ws-foreign"}, nil
		})},
		{name: "unsupported source", reader: projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{"https://forge.invalid/acme/repo.git"}}, nil
		})},
		{name: "mixed valid and invalid sources", reader: projectRepositorySourceReaderFunc(func(_ context.Context, _ string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{"https://github.com/acme/valid.git", "https://forge.invalid/acme/invalid.git"}}, nil
		})},
		{name: "GitLab trusted origin before untrusted HTTP origin", reader: projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{"https://gitlab.com/acme/api.git", "http://gitlab.com/acme/api.git"}}, nil
		})},
		{name: "GitLab untrusted HTTP origin before trusted origin", reader: projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{"http://gitlab.com/acme/api.git", "https://gitlab.com/acme/api.git"}}, nil
		})},
		{name: "invalid local source", reader: projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
			return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{"/missing/project/source"}}, nil
		})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"}); err != nil {
				t.Fatalf("create workflow: %v", err)
			}
			svc.SetProjectRepositorySourceReader(tc.reader)
			_, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: "wf-1", Title: "Invalid project task", ProjectID: "project-1",
			})
			if err == nil {
				t.Fatal("CreateTask succeeded with invalid or unavailable project sources")
			}
			if tc.errorNotContains != "" && strings.Contains(err.Error(), tc.errorNotContains) {
				t.Fatalf("CreateTask error %q exposes forbidden value %q", err, tc.errorNotContains)
			}
			var taskCount int
			if err := repo.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE workspace_id = ?`, "ws-1").Scan(&taskCount); err != nil {
				t.Fatalf("count tasks: %v", err)
			}
			if taskCount != 0 {
				t.Fatalf("task rows = %d, want none after source resolution failure", taskCount)
			}
			for _, event := range svc.eventBus.(*MockEventBus).GetPublishedEvents() {
				if event.Type == "task.created" {
					t.Fatalf("published task.created event = %#v, want none", event)
				}
			}
		})
	}
}

func TestCreateTaskProjectSourcesDeduplicateLocalRemoteRepositoryAliases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources func(localPath string) []string
	}{
		{
			name: "local path before remote URL",
			sources: func(localPath string) []string {
				return []string{localPath, "https://github.com/acme/api.git"}
			},
		},
		{
			name: "remote URL before local path",
			sources: func(localPath string) []string {
				return []string{"https://github.com/acme/api.git", localPath}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := setupOfficeTest(t)
			ctx := context.Background()
			const repoID = "repo-project-local-remote-alias"
			workflowID := "wf-project-local-remote-alias"
			if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: "ws-1", Name: "Board"}); err != nil {
				t.Fatalf("create workflow: %v", err)
			}
			localPath := t.TempDir()
			initRealGitRepo(t, localPath)
			addRemote := exec.Command("git", "remote", "add", "origin", "https://github.com/acme/api.git")
			addRemote.Dir = localPath
			if output, err := addRemote.CombinedOutput(); err != nil {
				t.Fatalf("add matching repository origin: %v: %s", err, output)
			}
			canonicalPath := canonicalRepoTestPath(t, localPath)
			if err := repo.CreateRepository(ctx, &models.Repository{
				ID: repoID, WorkspaceID: "ws-1", Name: "acme/api", SourceType: sourceTypeLocal,
				LocalPath: canonicalPath, Provider: "github", ProviderHost: "https://github.com",
				ProviderOwner: "acme", ProviderName: "api", RemoteURL: "https://github.com/acme/api.git",
				DefaultBranch: "main",
			}); err != nil {
				t.Fatalf("register local checkout: %v", err)
			}
			svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
				return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: tc.sources(localPath)}, nil
			}))

			created, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Attach local and remote aliases", ProjectID: "project-local-remote-alias",
			})
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			attached, err := repo.ListTaskRepositories(ctx, created.Task.ID)
			if err != nil {
				t.Fatalf("ListTaskRepositories: %v", err)
			}
			if len(attached) != 1 || attached[0].RepositoryID != repoID {
				t.Fatalf("task repositories = %#v, want one attachment reusing %q", attached, repoID)
			}
		})
	}
}

func TestCreateTaskProjectSourcesReuseDeduplicateAndResolve(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	firstPath, secondPath := t.TempDir(), t.TempDir()
	initRealGitRepo(t, firstPath)
	initRealGitRepo(t, secondPath)
	canonicalFirst, canonicalSecond := canonicalRepoTestPath(t, firstPath), canonicalRepoTestPath(t, secondPath)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-project-existing", WorkspaceID: "ws-1", Name: "existing", SourceType: sourceTypeLocal,
		LocalPath: canonicalFirst, DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("register existing repository: %v", err)
	}
	workflowID := "wf-project-source-resolution"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: "ws-1", Name: "Board"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	projectSources := []string{
		firstPath,
		"github.com/acme/api",
		"https://github.com/acme/api",
		secondPath,
		firstPath + string('/'),
		"git@github.com:acme/api.git",
	}
	svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: projectSources}, nil
	}))
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Resolve project sources", ProjectID: "project-sources",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	attached, err := repo.ListTaskRepositories(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(attached) != 3 {
		t.Fatalf("attached repositories = %#v, want three unique project repositories", attached)
	}
	if attached[0].RepositoryID != "repo-project-existing" {
		t.Fatalf("first repository = %q, want existing repository reused", attached[0].RepositoryID)
	}
	remote, err := repo.GetRepository(ctx, attached[1].RepositoryID)
	if err != nil {
		t.Fatalf("get remote repository: %v", err)
	}
	if remote.RemoteURL != "https://github.com/acme/api.git" {
		t.Fatalf("remote URL = %q, want normal GitHub resolver result", remote.RemoteURL)
	}
	local, err := repo.GetRepository(ctx, attached[2].RepositoryID)
	if err != nil {
		t.Fatalf("get second local repository: %v", err)
	}
	if local.LocalPath != canonicalSecond {
		t.Fatalf("second local path = %q, want %q", local.LocalPath, canonicalSecond)
	}
	if attached[0].BaseBranch != "main" || attached[2].BaseBranch != "main" {
		t.Fatalf("local base branches = %q, %q; want normal main defaults", attached[0].BaseBranch, attached[2].BaseBranch)
	}
}

func TestCreateTaskProjectSourcesDoNotOverrideSubtaskRepositories(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	sourcePath := t.TempDir()
	initRealGitRepo(t, sourcePath)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-child-parent", WorkspaceID: "ws-1", Name: "parent", SourceType: sourceTypeLocal,
		LocalPath: canonicalRepoTestPath(t, sourcePath), DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("register parent repository: %v", err)
	}
	workflowID := "wf-project-child-context"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: "ws-1", Name: "Board"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	readerCalls := 0
	svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		readerCalls++
		return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{sourcePath}}, nil
	}))
	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Parent", ProjectID: "project-with-source",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, ParentID: parent.Task.ID, Title: "Inheriting child",
	})
	if err != nil {
		t.Fatalf("create inheriting child: %v", err)
	}
	childRepos, err := repo.ListTaskRepositories(ctx, child.Task.ID)
	if err != nil {
		t.Fatalf("list inheriting child repositories: %v", err)
	}
	if len(childRepos) != 1 || childRepos[0].RepositoryID != "repo-child-parent" {
		t.Fatalf("child repositories = %#v, want inherited parent repository", childRepos)
	}
	if readerCalls != 1 {
		t.Fatalf("project reader calls after inheriting child = %d, want 1", readerCalls)
	}

	parentWithoutRepos, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Repositoryless parent",
		ProjectID: "empty-project", Repositories: []TaskRepositoryInput{},
	})
	if err != nil {
		t.Fatalf("create repositoryless parent: %v", err)
	}
	repositorylessChild, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, ParentID: parentWithoutRepos.Task.ID, Title: "Repositoryless child",
	})
	if err != nil {
		t.Fatalf("create repositoryless child: %v", err)
	}
	childRepos, err = repo.ListTaskRepositories(ctx, repositorylessChild.Task.ID)
	if err != nil {
		t.Fatalf("list repositoryless child repositories: %v", err)
	}
	if len(childRepos) != 0 {
		t.Fatalf("repositoryless child repositories = %#v, want no project-source fallback", childRepos)
	}
	if readerCalls != 1 {
		t.Fatalf("project reader calls after repositoryless child = %d, want 1", readerCalls)
	}
}

func TestCreateTaskProjectSourcesEmptyAndExternalIDRetry(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupOfficeTest(t)
	workflowID := "wf-project-empty-retry"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: "ws-1", Name: "Board"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	readerCalls := 0
	svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		readerCalls++
		return ProjectRepositorySources{WorkspaceID: "ws-1"}, nil
	}))
	empty, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Empty project", ProjectID: "empty-project",
	})
	if err != nil {
		t.Fatalf("create empty project task: %v", err)
	}
	attached, err := repo.ListTaskRepositories(ctx, empty.Task.ID)
	if err != nil || len(attached) != 0 {
		t.Fatalf("empty project repositories = %#v, error = %v; want none", attached, err)
	}

	path := t.TempDir()
	initRealGitRepo(t, path)
	svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		readerCalls++
		return ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{path}}, nil
	}))
	first, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Idempotent project", ProjectID: "project-retry", ExternalID: "project-retry-id",
	})
	if err != nil {
		t.Fatalf("create idempotent project task: %v", err)
	}
	svc.SetProjectRepositorySourceReader(projectRepositorySourceReaderFunc(func(context.Context, string) (ProjectRepositorySources, error) {
		return ProjectRepositorySources{}, errors.New("retry must not read project sources")
	}))
	retry, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflowID, Title: "Retry", ProjectID: "project-retry", ExternalID: "project-retry-id",
	})
	if err != nil || retry.Task.ID != first.Task.ID {
		t.Fatalf("idempotent retry = %#v, error = %v; want original task", retry, err)
	}
	if readerCalls != 2 {
		t.Fatalf("reader calls after retry = %d, want no additional read", readerCalls)
	}
}

func TestProjectRepositoryInputsPreserveOrderForResolvedDeduplication(t *testing.T) {
	firstPath := t.TempDir()
	secondPath := t.TempDir()
	initRealGitRepo(t, firstPath)
	initRealGitRepo(t, secondPath)
	inputs, err := projectRepositoryInputs([]string{
		firstPath,
		"github.com/acme/api",
		"https://github.com/acme/api",
		secondPath,
		firstPath + string('/'),
		"git@github.com:acme/api.git",
	})
	if err != nil {
		t.Fatalf("projectRepositoryInputs: %v", err)
	}
	if len(inputs) != 6 {
		t.Fatalf("input count = %d, want all six entries retained for authoritative resolution: %#v", len(inputs), inputs)
	}
	if inputs[0].LocalPath != canonicalRepoTestPath(t, firstPath) ||
		inputs[1].RemoteURL != "github.com/acme/api" ||
		inputs[2].RemoteURL != "https://github.com/acme/api" ||
		inputs[3].LocalPath != canonicalRepoTestPath(t, secondPath) ||
		inputs[4].LocalPath != canonicalRepoTestPath(t, firstPath) ||
		inputs[5].RemoteURL != "git@github.com:acme/api.git" {
		t.Fatalf("inputs = %#v, want every source in first-occurrence order", inputs)
	}
}

func TestProjectRepositoryInputsTreatWindowsDrivePathAsLocal(t *testing.T) {
	if isRemoteProjectRepositorySource(`C:\work\repo`) {
		t.Fatal("Windows drive path was classified as a remote URL")
	}
	if !isRemoteProjectRepositorySource("github.com/acme/repo") {
		t.Fatal("scheme-less GitHub URL was not classified as a remote")
	}
	if !isRemoteProjectRepositorySource("git@github.com:acme/repo.git") {
		t.Fatal("SSH Git URL was not classified as remote")
	}
}
