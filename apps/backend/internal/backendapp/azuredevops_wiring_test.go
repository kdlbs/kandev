package backendapp

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/azuredevops"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestServicesExposeAzureDevOps(t *testing.T) {
	field, ok := reflect.TypeOf(Services{}).FieldByName("AzureDevOps")
	if !ok || field.Type != reflect.TypeOf((*azuredevops.Service)(nil)) {
		t.Fatal("Services does not expose *azuredevops.Service")
	}
}

func TestRepositoryLookupAdapterResolvesOnlyTaskLinkedAzureRepository(t *testing.T) {
	harness := newBootStateTestHarness(t)
	workspaces, err := harness.taskSvc.ListWorkspaces(t.Context())
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("list workspaces: %v", err)
	}
	repository, err := harness.taskSvc.CreateRepository(t.Context(), &taskservice.CreateRepositoryRequest{
		WorkspaceID: workspaces[0].ID, Name: "Azure repository", SourceType: "provider",
		Provider: azuredevops.RepositoryProvider, ProviderRepoID: "azure-repo-1",
		ProviderOwner: "project-1", ProviderName: "platform", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	task, err := harness.taskSvc.CreateTask(t.Context(), &taskservice.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID, Title: "Azure task", IsEphemeral: true,
		Repositories: []taskservice.TaskRepositoryInput{{RepositoryID: repository.ID, BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	adapter, ok := any(&repositoryLookupAdapter{svc: harness.taskSvc}).(azuredevops.RepositoryLookup)
	if !ok {
		t.Fatal("repositoryLookupAdapter does not implement azuredevops.RepositoryLookup")
	}
	binding, err := adapter.LookupTaskRepository(t.Context(), task.ID, repository.ID)
	if err != nil {
		t.Fatalf("lookup linked repository: %v", err)
	}
	if binding == nil || binding.WorkspaceID != workspaces[0].ID ||
		binding.Provider != azuredevops.RepositoryProvider || binding.ProviderOwner != "project-1" ||
		binding.ProviderRepoID != "azure-repo-1" {
		t.Fatalf("binding = %+v", binding)
	}
	unlinked, err := adapter.LookupTaskRepository(t.Context(), task.ID, "not-linked")
	if err != nil || unlinked != nil {
		t.Fatalf("unlinked lookup: binding=%+v err=%v", unlinked, err)
	}
}
