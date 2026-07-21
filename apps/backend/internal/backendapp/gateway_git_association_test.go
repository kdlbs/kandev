package backendapp

import (
	"context"
	"errors"
	"testing"
)

func TestCreatedChangeAssociationRouterRoutesGitLabSingleAndMultiRepo(t *testing.T) {
	var got []string
	router := createdChangeAssociationRouter{
		resolveRepositoryID: func(_ context.Context, _ string, repo string) string {
			if repo == "" {
				return "repo-primary"
			}
			return "repo-" + repo
		},
		resolveWorkspaceID: func(context.Context, string) (string, error) { return "workspace-1", nil },
		associateGitLab: func(_ context.Context, workspaceID, taskID, repositoryID, mrURL string) error {
			got = append(got, workspaceID+"|"+taskID+"|"+repositoryID+"|"+mrURL)
			return nil
		},
	}
	for _, repo := range []string{"", "secondary"} {
		if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "https://gitlab.example/g/r/-/merge_requests/4", "feature", repo); err != nil {
			t.Fatalf("associate repo %q: %v", repo, err)
		}
	}
	want := []string{
		"workspace-1|task-1|repo-primary|https://gitlab.example/g/r/-/merge_requests/4",
		"workspace-1|task-1|repo-secondary|https://gitlab.example/g/r/-/merge_requests/4",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("associations = %#v", got)
	}
}

func TestCreatedChangeAssociationRouterPreservesGitHubAndMissingServices(t *testing.T) {
	var got string
	router := createdChangeAssociationRouter{
		resolveRepositoryID: func(context.Context, string, string) string { return "repo-1" },
		associateGitHub: func(_ context.Context, sessionID, taskID, repositoryID, prURL, branch string) {
			got = sessionID + "|" + taskID + "|" + repositoryID + "|" + prURL + "|" + branch
		},
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "github", "https://github.com/g/r/pull/2", "feature", ""); err != nil {
		t.Fatal(err)
	}
	if got != "session-1|task-1|repo-1|https://github.com/g/r/pull/2|feature" {
		t.Fatalf("GitHub association = %q", got)
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "url", "feature", ""); err != nil {
		t.Fatalf("missing GitLab service should be a no-op: %v", err)
	}
}

func TestCreatedChangeAssociationRouterGitLabFailureCanRetry(t *testing.T) {
	attempts := 0
	router := createdChangeAssociationRouter{
		resolveRepositoryID: func(context.Context, string, string) string { return "repo-1" },
		resolveWorkspaceID:  func(context.Context, string) (string, error) { return "workspace-1", nil },
		associateGitLab: func(context.Context, string, string, string, string) error {
			attempts++
			if attempts == 1 {
				return errors.New("temporary failure containing sensitive provider output")
			}
			return nil
		},
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "url", "feature", ""); err == nil {
		t.Fatal("first association unexpectedly succeeded")
	}
	if err := router.associate(context.Background(), "session-1", "task-1", "gitlab", "url", "feature", ""); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d", attempts)
	}
}
