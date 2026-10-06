package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-WORKSPACES-BRANCH-POLICIES-004.7, AC-WORKSPACES-BRANCH-POLICIES-001.5
func TestBranchPolicyPatchTaskSnapshots(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	workflow := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-policy-service")
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-policy-service", WorkspaceID: "ws-policy-service", Name: "Policy repo", DefaultBranch: "main",
	}))
	original := createPatchPolicy(t, svc)
	createTask := func(title string) *models.Task {
		task, err := svc.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-policy-service", WorkflowID: workflow, Title: title,
			Repositories: []TaskRepositoryInput{{RepositoryID: original.RepositoryID, BranchPolicyID: original.ID}},
		})
		require.NoError(t, err)
		return task.Task
	}
	oldTask := createTask("Original workflow")
	peer := branchPolicyPatchPeer(t, svc, repo)
	edited := overlapPolicyPatches(t, svc, peer, original.ID,
		&UpdateRepositoryBranchPolicyRequest{Name: stringPointer("Hotfix"), Description: stringPointer("changed")},
		&UpdateRepositoryBranchPolicyRequest{BaseBranch: stringPointer("main"), BranchTemplate: stringPointer("hotfix/{title}-{suffix}"), PullRequestTarget: stringPointer("release")})
	eventBus.ClearEvents()
	newTask := createTask("Current workflow")
	assertPersistedPolicySnapshot(t, repo, oldTask.ID, original)
	assertPersistedPolicySnapshot(t, repo, newTask.ID, edited)
	assertPolicyTaskEvent(t, eventBus, newTask.ID, edited)
	require.NoError(t, svc.DeleteRepositoryBranchPolicy(ctx, original.ID))
	assertPersistedPolicySnapshot(t, repo, oldTask.ID, original)
	assertPersistedPolicySnapshot(t, repo, newTask.ID, edited)
}

func assertPersistedPolicySnapshot(t *testing.T, repo interface {
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
}, taskID string, policy *models.RepositoryBranchPolicy) {
	t.Helper()
	rows, err := repo.ListTaskRepositories(context.Background(), taskID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, policy.BaseBranch, row.BaseBranch)
	require.Equal(t, policy.ID, row.BranchPolicyID)
	require.Equal(t, policy.Name, row.BranchPolicyName)
	require.Equal(t, policy.BaseBranch, row.BranchPolicyBaseBranch)
	require.Equal(t, policy.BranchTemplate, row.BranchPolicyBranchTemplate)
	require.Equal(t, policy.PullRequestTarget, row.BranchPolicyPullRequestTarget)
}

func assertPolicyTaskEvent(t *testing.T, eventBus *MockEventBus, taskID string, policy *models.RepositoryBranchPolicy) {
	t.Helper()
	found := false
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskCreated {
			continue
		}
		data := event.Data.(map[string]interface{})
		if data["task_id"] != taskID {
			continue
		}
		rows := data["repositories"].([]map[string]interface{})
		require.Len(t, rows, 1)
		require.Equal(t, policy.ID, rows[0]["branch_policy_id"])
		require.Equal(t, policy.Name, rows[0]["branch_policy_name"])
		require.Equal(t, policy.BaseBranch, rows[0]["branch_policy_base_branch"])
		require.Equal(t, policy.BranchTemplate, rows[0]["branch_policy_branch_template"])
		require.Equal(t, policy.PullRequestTarget, rows[0]["branch_policy_pull_request_target"])
		found = true
	}
	require.True(t, found, "actual task.created event missing")
}
