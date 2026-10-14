package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationPostcommitObservation(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "current_reload", true: "candidate_fallback"}[fallback], func(t *testing.T) {
			var hook *issueMutationTaskHook
			h := newIssueMutationHarness(t, func(i int, r *sqliterepo.Repository) repository.TaskRepository {
				if i == 1 {
					return r
				}
				hook = &issueMutationTaskHook{TaskRepository: r}
				return hook
			})
			ctx := context.Background()
			gh, _ := issueMutationGitHub(t, h.services[0])
			if fallback {
				hook.observationError = errors.New("observation unavailable")
			} else {
				hook.after = func(ctx context.Context) error {
					title := "Later committed title"
					_, err := h.services[1].UpdateTask(ctx, "issue-task", &taskservice.UpdateTaskRequest{Title: &title})
					return err
				}
			}
			response, err := gh.LinkTaskIssue(ctx, "issue-task", issueMutationRequest(42))
			require.NoError(t, err)
			require.Equal(t, "issue-task", response.TaskTitle)
			published := h.buses[0].snapshot()
			require.Len(t, published, 1)
			require.Equal(t, events.TaskUpdated, published[0].Type)
			data := published[0].Data.(map[string]interface{})
			assertIssueMutationIdentity(t, data["metadata"].(map[string]interface{}), 42)
			expected := "Later committed title"
			if fallback {
				expected = "issue-task"
			}
			require.Equal(t, expected, data["title"])
			stored, err := h.repos[0].GetTask(ctx, "issue-task")
			require.NoError(t, err)
			require.Equal(t, expected, stored.Title)
			assertIssueMutationIdentity(t, stored.Metadata, 42)
		})
	}
}

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestGitHubIssueMutationRepositoryObservationFallback(t *testing.T) {
	var hook *issueMutationTaskHook
	h := newIssueMutationHarness(t, func(i int, r *sqliterepo.Repository) repository.TaskRepository {
		if i == 1 {
			return r
		}
		hook = &issueMutationTaskHook{TaskRepository: r}
		return hook
	})
	hook.after = func(context.Context) error { h.repoLists[0].fail.Store(true); return nil }
	gh, _ := issueMutationGitHub(t, h.services[0])
	require.NoError(t, runIssueMutation(context.Background(), gh, 42))
	stored, err := h.repos[1].GetTask(context.Background(), "issue-task")
	require.NoError(t, err)
	assertIssueMutationIdentity(t, stored.Metadata, 42)
	actualRepos, err := h.repos[1].ListTaskRepositories(context.Background(), "issue-task")
	require.NoError(t, err)
	require.Len(t, actualRepos, 1)
	require.Len(t, h.buses[0].snapshot(), 1)
}
