package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.1, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationStorageAndRepositoryFailures(t *testing.T) {
	for _, kind := range []string{"storage", "list", "lookup", "no_attached_repository"} {
		t.Run(kind, func(t *testing.T) {
			h := newIssueMutationHarness(t, nil)
			ctx := context.Background()
			gh, _ := issueMutationGitHub(t, h.services[0])
			store := issueMutationAdmissionStore{githubTaskIssueStoreAdapter: githubTaskIssueStoreAdapter{svc: h.services[0]}}
			injected := errors.New("repository read rejected")
			switch kind {
			case "storage":
				_, err := h.repos[0].DB().Exec(`CREATE TRIGGER reject_issue_service BEFORE UPDATE OF metadata ON tasks BEGIN SELECT RAISE(ABORT,'issue rejected'); END`)
				require.NoError(t, err)
			case "list":
				store.listError = injected
			case "lookup":
				store.afterRepository = func(context.Context) error { return injected }
			case "no_attached_repository":
				_, err := h.repos[0].DB().Exec(`DELETE FROM task_repositories WHERE task_id='issue-task'`)
				require.NoError(t, err)
			}
			gh.SetTaskIssueStore(store)
			before, err := h.repos[0].GetTask(ctx, "issue-task")
			require.NoError(t, err)
			_, err = gh.LinkTaskIssue(ctx, "issue-task", github.LinkTaskIssueRequest{Owner: "acme", Repo: "api", Number: 42})
			stored, readErr := h.repos[0].GetTask(ctx, "issue-task")
			require.NoError(t, readErr)
			if kind == "no_attached_repository" {
				require.NoError(t, err)
				assertIssueMutationIdentity(t, stored.Metadata, 42)
				require.Len(t, h.buses[0].snapshot(), 1)
				return
			}
			require.Error(t, err)
			if kind != "storage" {
				require.ErrorIs(t, err, injected)
			}
			require.Equal(t, before, stored)
			require.Empty(t, h.buses[0].snapshot())
			if kind == "storage" {
				require.Error(t, gh.UnlinkTaskIssue(ctx, "issue-task"))
				current, err := h.repos[0].GetTask(ctx, "issue-task")
				require.NoError(t, err)
				require.Equal(t, before, current)
				require.Empty(t, h.buses[0].snapshot())
			}
		})
	}
}
