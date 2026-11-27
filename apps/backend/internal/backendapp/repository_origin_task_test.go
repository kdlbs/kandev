package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/plugins"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type gitLabConnectionStub struct {
	config      *gitlab.GitLabConfig
	err         error
	workspaceID string
}

func (s *gitLabConnectionStub) GetConfigForWorkspace(_ context.Context, workspaceID string) (*gitlab.GitLabConfig, error) {
	s.workspaceID = workspaceID
	return s.config, s.err
}

type projectSourceReaderStub struct {
	workspaceID string
	sources     []string
}

func (s projectSourceReaderStub) ReadProjectRepositorySources(context.Context, string) (taskservice.ProjectRepositorySources, error) {
	return taskservice.ProjectRepositorySources{WorkspaceID: s.workspaceID, Sources: s.sources}, nil
}

// TestCreateTaskProjectRemoteProviderAdapters wires the production GitLab
// origin verifier and plugin selection adapter into a real task service. It
// proves the selected workspace reaches the connection reader and that the
// adapter descriptors are persisted without rebuilding their clone URLs.
//
// @covers AC-TASKS-PROJECT-REPOSITORIES-001.10, AC-TASKS-PROJECT-REPOSITORIES-001.12
func TestCreateTaskProjectRemoteProviderAdapters(t *testing.T) {
	t.Run("self-managed GitLab through the real origin verifier", func(t *testing.T) {
		harness := newBootStateTestHarness(t)
		ctx := context.Background()
		adapterWorkspaceID := createAdapterWorkspace(t, harness, "adapters")
		connections := &gitLabConnectionStub{config: &gitlab.GitLabConfig{
			Host: "https://gitlab.example.test:8443", AuthMethod: gitlab.AuthMethodPAT,
		}}
		harness.taskSvc.SetGitLabRepositoryOriginVerifier(gitLabRepositoryOriginVerifier{connections: connections})
		harness.taskSvc.SetProjectRepositorySourceReader(projectSourceReaderStub{
			workspaceID: adapterWorkspaceID,
			sources:     []string{"https://gitlab.example.test:8443/group/subgroup/project"},
		})

		result, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
			WorkspaceID: adapterWorkspaceID, WorkflowID: "wf-adapters", WorkflowStepID: "step-adapters",
			Title: "GitLab adapter task", ProjectID: "project-adapters-gitlab",
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if connections.workspaceID != adapterWorkspaceID {
			t.Fatalf("connection reader workspace = %q, want the selected task workspace", connections.workspaceID)
		}
		stored, err := harness.taskRepo.GetRepository(ctx, result.Task.Repositories[0].RepositoryID)
		if err != nil {
			t.Fatalf("GetRepository: %v", err)
		}
		if stored.Provider != "gitlab" || stored.ProviderHost != "https://gitlab.example.test:8443" ||
			stored.ProviderOwner != "group/subgroup" || stored.ProviderName != "project" {
			t.Fatalf("stored repository = %+v", stored)
		}
	})

	t.Run("plugin descriptor through the real selection adapter", func(t *testing.T) {
		harness := newBootStateTestHarness(t)
		ctx := context.Background()
		adapterWorkspaceID := createAdapterWorkspace(t, harness, "adapters")
		inspector := &repositoryProviderInspectorStub{discovery: &plugins.RepositoryProviderDiscoveryResult{
			Candidates: []plugins.RepositoryProviderInspection{{
				ProviderID: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
				ProviderScope: "workspace-a", ProviderRepositoryID: "repo-42", OwnerOrProject: "TEAM",
				Name: "fixture", CloneURL: "https://bitbucket.example.test/scm/TEAM/fixture.git", DefaultBranch: "main",
			}},
		}}
		harness.taskSvc.SetRepositorySelectionResolver(pluginRepositorySelectionResolver{inspector: inspector})
		harness.taskSvc.SetProjectRepositorySourceReader(projectSourceReaderStub{
			workspaceID: adapterWorkspaceID,
			sources:     []string{"https://bitbucket.example.test/projects/TEAM/fixture"},
		})

		result, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
			WorkspaceID: adapterWorkspaceID, WorkflowID: "wf-adapters", WorkflowStepID: "step-adapters",
			Title: "Plugin adapter task", ProjectID: "project-adapters-plugin",
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if inspector.discoveryRequest.URL != "https://bitbucket.example.test/projects/TEAM/fixture" {
			t.Fatalf("discovery request = %+v", inspector.discoveryRequest)
		}
		stored, err := harness.taskRepo.GetRepository(ctx, result.Task.Repositories[0].RepositoryID)
		if err != nil {
			t.Fatalf("GetRepository: %v", err)
		}
		if stored.Provider != "fixture-source-control" || stored.ProviderHost != "https://bitbucket.example.test" ||
			stored.ProviderScope != "workspace-a" || stored.ProviderRepoID != "repo-42" ||
			stored.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" ||
			stored.DefaultBranch != "main" {
			t.Fatalf("stored repository = %+v", stored)
		}
	})

	t.Run("unconfigured connection refuses before writes", func(t *testing.T) {
		harness := newBootStateTestHarness(t)
		ctx := context.Background()
		adapterWorkspaceID := createAdapterWorkspace(t, harness, "adapters")
		connections := &gitLabConnectionStub{err: errors.New("connection unavailable")}
		harness.taskSvc.SetGitLabRepositoryOriginVerifier(gitLabRepositoryOriginVerifier{connections: connections})
		harness.taskSvc.SetProjectRepositorySourceReader(projectSourceReaderStub{
			workspaceID: adapterWorkspaceID,
			sources:     []string{"https://gitlab.example.test:8443/group/subgroup/project"},
		})
		_, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
			WorkspaceID: adapterWorkspaceID, WorkflowID: "wf-adapters", WorkflowStepID: "step-adapters",
			Title: "Unavailable GitLab adapter task", ProjectID: "project-adapters-unavailable",
		})
		var selectionErr *taskservice.RepositorySelectionError
		if !errors.As(err, &selectionErr) || selectionErr.Code != taskservice.RepositorySelectionErrorUnavailable {
			t.Fatalf("error = %v, want unavailable", err)
		}
		repositories, listErr := harness.taskRepo.ListRepositories(ctx, adapterWorkspaceID)
		if listErr != nil || len(repositories) != 0 {
			t.Fatalf("repositories = %+v, error = %v; want no writes", repositories, listErr)
		}
	})
}

func createAdapterWorkspace(t *testing.T, harness bootStateTestHarness, name string) string {
	t.Helper()
	ctx := context.Background()
	workspace, err := harness.taskSvc.CreateWorkspace(ctx, &taskservice.CreateWorkspaceRequest{Name: name})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if _, err := harness.taskSvc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		ID: "wf-adapters", WorkspaceID: workspace.ID, Name: "Adapters",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := harness.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: "step-adapters", WorkflowID: "wf-adapters", Name: "Adapters", Position: 0,
	}); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	return workspace.ID
}
