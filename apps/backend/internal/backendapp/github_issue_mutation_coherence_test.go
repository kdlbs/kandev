package backendapp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.2, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.3
func TestGitHubIssueMutationCoherence(t *testing.T) {
	for _, pair := range [][2]int{{42, 99}, {99, 42}, {42, 42}, {42, 0}, {0, 42}} {
		t.Run(fmt.Sprint(pair), func(t *testing.T) {
			h := newIssueMutationHarness(t, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			task, err := h.repos[0].GetTask(ctx, "issue-task")
			require.NoError(t, err)
			task.Metadata = issueMutationMetadata(7)
			task.Metadata["issue_watch_id"] = "watch"
			task.Metadata["issue_author"] = "author"
			task.Metadata["profile"] = "retained"
			require.NoError(t, h.repos[0].UpdateTask(ctx, task))
			require.NoError(t, h.repos[0].CreateRepository(ctx, &models.Repository{ID: "relink-repo", WorkspaceID: "issue-ws", Name: "tools", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "other", ProviderName: "tools"}))
			require.NoError(t, h.repos[0].CreateTaskRepository(ctx, &models.TaskRepository{ID: "relink-task-repo", TaskID: "issue-task", RepositoryID: "relink-repo"}))
			seedTaskChangeCoordinatorTask(t, h.repos[0], "other-ws", "other-task", "other-repo", "https://github.com", "other", "tools")
			other, err := h.repos[0].GetTask(ctx, "other-task")
			require.NoError(t, err)
			second, gate := issueMutationGitHub(t, h.services[1])
			first, _ := issueMutationGitHub(t, h.services[0])
			gate.armed.Store(true)
			result := make(chan error, 1)
			go func() { result <- runIssueMutation(ctx, second, pair[1]) }()
			joined := false
			defer func() {
				cancel()
				if !joined {
					<-result
				}
			}()
			select {
			case <-gate.arrived:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.NoError(t, runIssueMutation(ctx, first, pair[0]))
			close(gate.release)
			require.NoError(t, <-result)
			joined = true
			stored, err := h.repos[0].GetTask(ctx, "issue-task")
			require.NoError(t, err)
			assertIssueMutationIdentity(t, stored.Metadata, pair[1])
			for _, key := range []string{"issue_watch_id", "issue_author", "profile", "keep"} {
				require.Equal(t, task.Metadata[key], stored.Metadata[key])
			}
			current, err := h.repos[1].GetTask(ctx, "other-task")
			require.NoError(t, err)
			require.Equal(t, other, current)
		})
	}
}

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.3
func TestGitHubIssueMutationDistinctCompleteLinks(t *testing.T) {
	h := newIssueMutationHarness(t, nil)
	ctx := context.Background()
	for _, link := range []*models.TaskGitHubIssueLink{{URL: "https://github.com/acme/api/issues/42", Number: 42, Owner: "acme", Repo: "api"}, {URL: "https://github.com/other/tools/issues/99", Number: 99, Owner: "other", Repo: "tools"}, nil} {
		updated, err := h.services[0].UpdateTaskGitHubIssue(ctx, "issue-task", link)
		require.NoError(t, err)
		if link == nil {
			assertIssueMutationIdentity(t, updated.Metadata, 0)
			continue
		}
		require.Equal(t, link.URL, updated.Metadata["issue_url"])
		require.Equal(t, float64(link.Number), updated.Metadata["issue_number"])
		require.Equal(t, link.Owner, updated.Metadata["issue_owner"])
		require.Equal(t, link.Repo, updated.Metadata["issue_repo"])
		require.Equal(t, true, updated.Metadata["github_issue_linked"])
	}
}
