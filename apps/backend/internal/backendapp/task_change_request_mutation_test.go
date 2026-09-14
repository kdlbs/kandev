package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskChangeCoordinatorProvidesTypedMutationResult(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: &fakeGitLabChangeLinks{}}

	mutationService, ok := any(coordinator).(mcphandlers.TaskChangeLinkMutationService)
	require.True(t, ok, "task change coordinator must expose the typed neutral mutation seam")
	result, err := mutationService.ManageTaskChangeRequest(context.Background(), mcphandlers.TaskChangeLinkRequest{
		TaskID: "task-1",
		Link:   mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
	})

	require.NoError(t, err)
	assert.Equal(t, "task-1", result.TaskID)
	assert.True(t, result.StateKnown)
	assert.Equal(t, []mcphandlers.TaskChangeLink{{Provider: "gitlab", RepositoryID: "repo-1", Number: 42}}, result.Links)
}

func TestTaskChangeCoordinatorTypedReplacementFailureState(t *testing.T) {
	taskSvc, repos := newTaskChangeCoordinatorHarness(t)
	seedTaskChangeCoordinatorTask(t, repos, "ws-1", "task-1", "repo-1", "https://gitlab.example.test", "group", "project")
	gitlabLinks := &fakeGitLabChangeLinks{
		mrs: []*gitlab.TaskMR{{
			ID: "old", TaskID: "task-1", RepositoryID: "repo-1", MRIID: 7,
			MRURL: "https://gitlab.example.test/group/project/-/merge_requests/7",
		}},
		unlinkErr: map[string]error{
			"old":        errors.New("delete failed"),
			"linked-new": errors.New("rollback failed"),
		},
	}
	coordinator := taskChangeLinkCoordinator{tasks: taskSvc, gitlab: gitlabLinks}
	mutationService, ok := any(coordinator).(mcphandlers.TaskChangeLinkMutationService)
	require.True(t, ok)

	result, err := mutationService.ManageTaskChangeRequest(context.Background(), mcphandlers.TaskChangeLinkRequest{
		Operation: "replace",
		TaskID:    "task-1",
		Link:      mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
		Old:       &mcphandlers.TaskChangeLink{Provider: "gitlab", RepositoryID: "repo-1", Number: 7},
	})

	require.Error(t, err)
	assert.Equal(t, taskChangeMutationOperationError, result.OperationError)
	assert.Equal(t, taskChangeMutationRollbackError, result.RollbackError)
	assert.NotContains(t, result.OperationError, "delete failed")
	assert.NotContains(t, result.RollbackError, "rollback failed")
	assert.True(t, result.StateKnown)
	assert.ElementsMatch(t, []mcphandlers.TaskChangeLink{
		{Provider: "gitlab", RepositoryID: "repo-1", Number: 7},
		{Provider: "gitlab", RepositoryID: "repo-1", Number: 42},
	}, result.Links)
}
