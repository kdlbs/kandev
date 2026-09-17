package orchestrator

import (
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestDynamicRepositorySummaryPreservesMixedWorkspaceSourceOrder(t *testing.T) {
	got := dynamicRepositorySummary(&v1.Task{
		Repositories: []v1.TaskRepository{
			{RepositoryID: "repo-after", BaseBranch: "main", Position: 2},
			{RepositoryID: "repo-before", BaseBranch: "develop", Position: 0},
		},
		WorkspaceFolders: []v1.TaskWorkspaceFolder{
			{DisplayName: "assets", LocalPath: "/work/assets", Position: 1},
		},
	})

	want := "repo-before @ develop\nfolder: assets @ /work/assets\nrepo-after @ main"
	if got != want {
		t.Fatalf("dynamic repository summary = %q, want %q", got, want)
	}
}

func TestDynamicRepositorySummaryKeepsLegacyArrayOrderWhenPositionsTie(t *testing.T) {
	got := dynamicRepositorySummary(&v1.Task{
		Repositories: []v1.TaskRepository{
			{RepositoryID: "repo-a", BaseBranch: "main"},
			{RepositoryID: "repo-b", BaseBranch: "develop"},
		},
		WorkspaceFolders: []v1.TaskWorkspaceFolder{
			{DisplayName: "assets", LocalPath: "/work/assets"},
		},
	})

	want := "repo-a @ main\nrepo-b @ develop\nfolder: assets @ /work/assets"
	if got != want {
		t.Fatalf("legacy dynamic repository summary = %q, want %q", got, want)
	}
}
