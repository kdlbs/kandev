package lifecycle

import (
	"reflect"
	"testing"
)

func TestTaskHostWorkspaceProjectionUsesPersistedPhysicalPositions(t *testing.T) {
	primary, secondary := 0, 1
	info := &WorkspaceInfo{WorkspaceRepositories: []WorkspaceRepositorySpec{
		{RepositoryID: "repo-beta", RepoName: "beta", BaseBranch: "main", Position: 0, TaskHostPosition: &secondary},
		{RepositoryID: "repo-alpha", RepoName: "alpha", BaseBranch: "main", Position: 1, TaskHostPosition: &primary},
	}}

	workspacePath, roots, err := taskHostWorkspaceProjection("docker", info)
	if err != nil {
		t.Fatal(err)
	}
	if workspacePath != "/workspace" {
		t.Fatalf("workspace path = %q, want /workspace", workspacePath)
	}
	want := []string{"/workspace", "/workspace/beta-main"}
	if !reflect.DeepEqual(roots, want) {
		t.Fatalf("physical roots = %v, want %v", roots, want)
	}
}

func TestTaskHostWorkspaceProjectionKeepsBorrowerSecondaryRepoNested(t *testing.T) {
	secondary := 1
	info := &WorkspaceInfo{WorkspaceRepositories: []WorkspaceRepositorySpec{{
		RepositoryID: "repo-beta", RepoName: "beta", BaseBranch: "main", Position: 0, TaskHostPosition: &secondary,
	}}}

	_, roots, err := taskHostWorkspaceProjection("docker", info)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roots, []string{"/workspace/beta-main"}) {
		t.Fatalf("secondary-only borrower roots = %v", roots)
	}
}

func TestTaskHostWorkspaceProjectionRejectsPartialPhysicalMapping(t *testing.T) {
	primary := 0
	_, _, err := taskHostWorkspaceProjection("docker", &WorkspaceInfo{
		WorkspaceRepositories: []WorkspaceRepositorySpec{
			{RepositoryID: "repo-alpha", RepoName: "alpha", BaseBranch: "main", TaskHostPosition: &primary},
			{RepositoryID: "repo-beta", RepoName: "beta", BaseBranch: "main"},
		},
	})
	if err == nil {
		t.Fatal("partial physical task-host mapping was accepted")
	}
}
