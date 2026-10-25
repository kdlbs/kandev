package backendapp

import (
	"context"
	"errors"
	"go/ast"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
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
