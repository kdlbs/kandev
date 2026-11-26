package backendapp

import (
	"context"
	"errors"
	"go/ast"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/plugins"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kdlbs/kandev/pluginsdk"
)

type emptyOfficeProjectSourceReader struct{}

func (emptyOfficeProjectSourceReader) ReadProjectRepositorySources(
	context.Context,
	string,
) (taskservice.ProjectRepositorySources, error) {
	return taskservice.ProjectRepositorySources{WorkspaceID: "ws-1", Sources: []string{}}, nil
}

type fakeOfficeProjectRepositoryReader struct {
	project *officemodels.Project
	err     error
	gotID   string
}

func (r *fakeOfficeProjectRepositoryReader) GetProject(_ context.Context, id string) (*officemodels.Project, error) {
	r.gotID = id
	return r.project, r.err
}

func (r *fakeOfficeProjectRepositoryReader) GetProjectWorkspaceID(_ context.Context, id string) (string, error) {
	r.gotID = id
	if r.err != nil {
		return "", r.err
	}
	if r.project == nil || r.project.ID != id {
		return "", nil
	}
	return r.project.WorkspaceID, nil
}

func TestOfficeProjectRepositorySourceAdapterReadsExactProject(t *testing.T) {
	encoded, err := officemodels.EncodeRepositories([]string{"/repos/first", "https://github.com/acme/second.git"})
	if err != nil {
		t.Fatalf("encode sources: %v", err)
	}
	reader := &fakeOfficeProjectRepositoryReader{project: &officemodels.Project{
		ID: "project-exact", WorkspaceID: "workspace-exact", Repositories: encoded,
	}}
	result, err := (&officeProjectRepositorySourceAdapter{reader: reader}).ReadProjectRepositorySources(context.Background(), "project-exact")
	if err != nil {
		t.Fatalf("ReadProjectRepositorySources: %v", err)
	}
	if reader.gotID != "project-exact" {
		t.Fatalf("project ID = %q, want exact ID", reader.gotID)
	}
	workspaceID, err := (&officeProjectRepositorySourceAdapter{reader: reader}).GetProjectWorkspaceID(context.Background(), "project-exact")
	if err != nil || workspaceID != "workspace-exact" {
		t.Fatalf("GetProjectWorkspaceID = %q, err = %v; want exact workspace", workspaceID, err)
	}
	if result.WorkspaceID != "workspace-exact" || len(result.Sources) != 2 || result.Sources[0] != "/repos/first" || result.Sources[1] != "https://github.com/acme/second.git" {
		t.Fatalf("result = %#v, want workspace and ordered sources", result)
	}
}

func TestOfficeProjectRepositorySourceAdapterRejectsReadAndDecodeFailures(t *testing.T) {
	readErr := errors.New("office database unavailable")
	for _, tc := range []struct {
		name    string
		reader  *fakeOfficeProjectRepositoryReader
		wantErr error
	}{
		{name: "read failure", reader: &fakeOfficeProjectRepositoryReader{err: readErr}, wantErr: readErr},
		{name: "missing project", reader: &fakeOfficeProjectRepositoryReader{}},
		{name: "invalid stored sources", reader: &fakeOfficeProjectRepositoryReader{project: &officemodels.Project{ID: "project-bad", WorkspaceID: "ws-1", Repositories: "{invalid"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (&officeProjectRepositorySourceAdapter{reader: tc.reader}).ReadProjectRepositorySources(context.Background(), "project-bad")
			if err == nil {
				t.Fatal("ReadProjectRepositorySources succeeded, want error")
			}
			if tc.name == "missing project" && !errors.Is(err, taskservice.ErrProjectNotFound) {
				t.Fatalf("missing project error = %v, want ErrProjectNotFound", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want wrapped %v", err, tc.wantErr)
			}
		})
	}
}

func TestOfficeProjectRepositoryReaderWiring(t *testing.T) {
	_, taskSvc, officeRepo := newOfficeTaskAdapterHarness(t)
	wireOfficeProjectRepositorySources(taskSvc, officeRepo)
	encoded, err := officemodels.EncodeRepositories([]string{"https://github.com/acme/project-repo"})
	if err != nil {
		t.Fatalf("encode project sources: %v", err)
	}
	if err := officeRepo.CreateProject(context.Background(), &officemodels.Project{
		ID: "project-wiring", WorkspaceID: "ws-1", Name: "Wired project",
		Status: officemodels.ProjectStatusActive, Repositories: encoded,
	}); err != nil {
		t.Fatalf("create Office project: %v", err)
	}
	workflows, err := taskSvc.ListWorkflows(context.Background(), "ws-1", true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: %v (len=%d)", err, len(workflows))
	}
	created, err := taskSvc.CreateTask(context.Background(), &taskservice.CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workflows[0].ID, Title: "Project source wiring",
		ProjectID: "project-wiring",
	})
	if err != nil {
		t.Fatalf("CreateTask through wired task service: %v", err)
	}
	if len(created.Task.Repositories) != 1 {
		t.Fatalf("created task repositories = %#v, want one project source", created.Task.Repositories)
	}
	repositories, err := taskSvc.ListRepositories(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if len(repositories) != 1 || repositories[0].RemoteURL != "https://github.com/acme/project-repo.git" {
		t.Fatalf("workspace repositories = %#v, want resolved project remote", repositories)
	}
}

// @covers AC-PLUGINS-PLUGINS-001.17, AC-PLUGINS-PLUGINS-001.19
func TestPluginsTaskWriter_CreateTaskInProject(t *testing.T) {
	ctx := context.Background()
	_, taskSvc, officeRepo, taskRepo, testDB := newOfficeTaskAdapterHarnessWithTestDB(t)
	wireOfficeProjectRepositorySources(taskSvc, officeRepo)
	project := func(id, workspaceID string) {
		t.Helper()
		repositories, err := officemodels.EncodeRepositories(nil)
		require.NoError(t, err)
		require.NoError(t, officeRepo.CreateProject(ctx, &officemodels.Project{
			ID: id, WorkspaceID: workspaceID, Name: id,
			Status: officemodels.ProjectStatusActive, Repositories: repositories,
		}))
	}
	project("project-plugin-create", "ws-1")

	created, err := (pluginsTaskWriterAdapter{svc: taskSvc}).CreateTask(ctx, plugins.TaskCreateInput{
		WorkspaceID: "ws-1", Title: "Create in project",
		ProjectID: "project-plugin-create",
	})
	require.NoError(t, err)
	require.Equal(t, "project-plugin-create", created.ProjectID)
	require.NotEmpty(t, created.WorkflowID, "project creates resolve the Office workflow")
	require.Empty(t, created.ParentID)

	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-other", Name: "Other workspace"}))
	project("project-plugin-foreign", "ws-other")
	for _, tc := range []struct {
		name      string
		projectID string
	}{
		{name: "missing project", projectID: "missing-project"},
		{name: "foreign workspace", projectID: "project-plugin-foreign"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (pluginsTaskWriterAdapter{svc: taskSvc}).CreateTask(ctx, plugins.TaskCreateInput{
				WorkspaceID: "ws-1", Title: "Reject project",
				ProjectID:    tc.projectID,
				Repositories: []pluginsdk.PluginTaskRepository{{RepositoryID: "must-not-resolve"}},
			})
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}

	project("project-plugin-delete-before-sources", "ws-1")
	deleteBeforeSources := projectDeleteAfterWorkspaceRead{Repository: officeRepo}
	wireOfficeProjectRepositorySources(taskSvc, deleteBeforeSources)
	_, err = (pluginsTaskWriterAdapter{svc: taskSvc}).CreateTask(ctx, plugins.TaskCreateInput{
		WorkspaceID: "ws-1", Title: "Project deleted before source read",
		ProjectID: "project-plugin-delete-before-sources",
	})
	require.Error(t, err)
	require.Equal(t, codes.Aborted, status.Code(err))
	assertNoProjectTask(t, ctx, testDB, "Project deleted before source read")

	project("project-plugin-race", "ws-1")
	racingReader := projectDeleteAfterRead{Repository: officeRepo}
	wireOfficeProjectRepositorySources(taskSvc, racingReader)
	_, err = (pluginsTaskWriterAdapter{svc: taskSvc}).CreateTask(ctx, plugins.TaskCreateInput{
		WorkspaceID: "ws-1", Title: "Project membership race",
		ProjectID: "project-plugin-race",
	})
	require.Error(t, err)
	require.Equal(t, codes.Aborted, status.Code(err))
	assertNoProjectTask(t, ctx, testDB, "Project membership race")
}

func assertNoProjectTask(t *testing.T, ctx context.Context, database *sqlx.DB, title string) {
	t.Helper()
	var taskCount int
	require.NoError(t, database.GetContext(ctx, &taskCount,
		database.Rebind(`SELECT COUNT(*) FROM tasks WHERE title = ?`), title))
	require.Zero(t, taskCount, "a project deleted during creation must not leave a task row")
}

type projectDeleteAfterRead struct {
	*officesqlite.Repository
}

func (r projectDeleteAfterRead) GetProject(ctx context.Context, id string) (*officemodels.Project, error) {
	project, err := r.Repository.GetProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := r.DeleteProject(ctx, id); err != nil {
		return nil, err
	}
	return project, nil
}

type projectDeleteAfterWorkspaceRead struct {
	*officesqlite.Repository
}

func (r projectDeleteAfterWorkspaceRead) GetProjectWorkspaceID(ctx context.Context, id string) (string, error) {
	workspaceID, err := r.Repository.GetProjectWorkspaceID(ctx, id)
	if err != nil {
		return "", err
	}
	if err := r.DeleteProject(ctx, id); err != nil {
		return "", err
	}
	return workspaceID, nil
}

func TestRegisterRoutesWiresOfficeProjectRepositorySources(t *testing.T) {
	fn := findFuncDecl(t, "helpers.go", "registerRoutes")
	found := false
	ast.Inspect(fn, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || dottedExprString(call.Fun) != "wireOfficeProjectRepositorySources" || len(call.Args) != 2 {
			return true
		}
		found = dottedExprString(call.Args[0]) == "p.taskSvc" && dottedExprString(call.Args[1]) == "p.officeRepo"
		return true
	})
	if !found {
		t.Fatal("registerRoutes does not wire Office project sources into the shared task service")
	}
}
