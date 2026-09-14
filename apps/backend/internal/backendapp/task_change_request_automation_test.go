package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskChangeRequestAutomationTaskLookup struct {
	task *models.Task
}

func (l taskChangeRequestAutomationTaskLookup) GetTask(context.Context, string) (*models.Task, error) {
	return l.task, nil
}

func (l taskChangeRequestAutomationTaskLookup) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	provider := "github"
	if id == "repo-gl" {
		provider = "gitlab"
	}
	return &models.Repository{ID: id, WorkspaceID: l.task.WorkspaceID, Provider: provider}, nil
}

type taskChangeRequestAutomationGitHubFake struct {
	prs       []*github.TaskPR
	options   *github.TaskCIOptionsResponse
	updateErr error
	updates   []github.TaskCIOptionsPatch
	callOrder *[]string
}

func (f *taskChangeRequestAutomationGitHubFake) ListTaskPRs(context.Context, []string) (map[string][]*github.TaskPR, error) {
	return map[string][]*github.TaskPR{"task-1": f.prs}, nil
}

func (f *taskChangeRequestAutomationGitHubFake) GetTaskCIOptionsResponse(context.Context, string) (*github.TaskCIOptionsResponse, error) {
	if f.options == nil {
		return &github.TaskCIOptionsResponse{}, nil
	}
	return f.options, nil
}

func (f *taskChangeRequestAutomationGitHubFake) GetWorkspaceAuthStatus(context.Context, string, string) (*github.WorkspaceAuthStatus, error) {
	return &github.WorkspaceAuthStatus{
		Automation:    &github.WorkspaceAutomationStatus{},
		Authenticated: true,
	}, nil
}

func (f *taskChangeRequestAutomationGitHubFake) UpdateTaskCIOptions(
	_ context.Context, _ string, patch github.TaskCIOptionsPatch,
) (*github.TaskCIOptionsResponse, error) {
	f.updates = append(f.updates, patch)
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, mcphandlers.TaskChangeRequestProviderGitHub)
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.options, nil
}

type taskChangeRequestAutomationGitLabFake struct {
	mrs       []*gitlab.TaskMR
	options   *gitlab.TaskMRAutomationResponse
	status    *gitlab.Status
	updateErr error
	updates   []gitlab.TaskMRAutomationPatch
	callOrder *[]string
}

func (f *taskChangeRequestAutomationGitLabFake) ListTaskMRsByTask(context.Context, string) ([]*gitlab.TaskMR, error) {
	return f.mrs, nil
}

func (f *taskChangeRequestAutomationGitLabFake) GetTaskMRAutomationResponse(context.Context, string) (*gitlab.TaskMRAutomationResponse, error) {
	if f.options == nil {
		return &gitlab.TaskMRAutomationResponse{}, nil
	}
	return f.options, nil
}

func (f *taskChangeRequestAutomationGitLabFake) GetStatusForWorkspace(context.Context, string) (*gitlab.Status, error) {
	if f.status != nil {
		return f.status, nil
	}
	return &gitlab.Status{Authenticated: true, AuthMethod: gitlab.AuthMethodPAT, TokenConfigured: true}, nil
}

func (f *taskChangeRequestAutomationGitLabFake) UpdateTaskMRAutomationOptions(
	_ context.Context, _ string, patch gitlab.TaskMRAutomationPatch,
) (*gitlab.TaskMRAutomationResponse, error) {
	f.updates = append(f.updates, patch)
	if f.callOrder != nil {
		*f.callOrder = append(*f.callOrder, mcphandlers.TaskChangeRequestProviderGitLab)
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.options, nil
}

func automationTaskForTest() *models.Task {
	return &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1",
		Repositories: []*models.TaskRepository{
			{RepositoryID: "repo-gh"},
			{RepositoryID: "repo-gl"},
		},
	}
}

func automationGitHubChangeForTest() *github.TaskPR {
	return &github.TaskPR{RepositoryID: "repo-gh", PRNumber: 8, State: "open"}
}

func automationGitLabChangeForTest() *gitlab.TaskMR {
	return &gitlab.TaskMR{
		RepositoryID: "repo-gl", ProjectPath: "group/project", MRIID: 42,
		Host: "https://gitlab.example.test", State: "opened",
	}
}

func newAutomationCoordinatorForTest(
	githubFake *taskChangeRequestAutomationGitHubFake,
	gitlabFake *taskChangeRequestAutomationGitLabFake,
) *taskChangeRequestAutomationCoordinator {
	return newTaskChangeRequestAutomationCoordinator(
		taskChangeRequestAutomationTaskLookup{task: automationTaskForTest()},
		githubFake, gitlabFake, nil, nil,
	)
}

func TestChangeRequestAutomationTargetMatrix(t *testing.T) {
	falseValue := false
	trueValue := true
	prompt := ""

	t.Run("association keeps exact GitHub identity", func(t *testing.T) {
		githubFake := &taskChangeRequestAutomationGitHubFake{
			prs: []*github.TaskPR{automationGitHubChangeForTest()},
			options: &github.TaskCIOptionsResponse{PROptions: []*github.TaskPRAutomationOptions{{
				RepositoryID: "repo-gh", PRNumber: 8,
			}}},
		}
		coordinator := newAutomationCoordinatorForTest(githubFake, &taskChangeRequestAutomationGitLabFake{})
		result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
			Target: mcphandlers.TaskChangeRequestAutomationTarget{Scope: mcphandlers.TaskChangeRequestAutomationScopeAssociation, Provider: "github", RepositoryID: "repo-gh", Number: 8},
			Patch:  mcphandlers.TaskChangeRequestAutomationPatch{AutoFixEnabled: &falseValue},
		})
		require.NoError(t, err)
		require.Len(t, githubFake.updates, 1)
		assert.Equal(t, "repo-gh", *githubFake.updates[0].RepositoryID)
		assert.Equal(t, 8, *githubFake.updates[0].PRNumber)
		assert.False(t, *githubFake.updates[0].AutoFixEnabled)
		assert.Equal(t, mcphandlers.TaskChangeRequestAutomationStatusApplied, result.Status)
		require.Len(t, result.Providers, 1)
		assert.Equal(t, "repo-gh", result.Providers[0].Affected[0].RepositoryID)
	})

	t.Run("prompt-only task update allows an empty task", func(t *testing.T) {
		githubFake := &taskChangeRequestAutomationGitHubFake{options: &github.TaskCIOptionsResponse{}}
		coordinator := newAutomationCoordinatorForTest(githubFake, &taskChangeRequestAutomationGitLabFake{})
		result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
			Target: mcphandlers.TaskChangeRequestAutomationTarget{Scope: mcphandlers.TaskChangeRequestAutomationScopeTask, Providers: []string{"github"}},
			Patch:  mcphandlers.TaskChangeRequestAutomationPatch{AutoFixPromptOverride: &prompt},
		})
		require.NoError(t, err)
		require.Len(t, githubFake.updates, 1)
		assert.Nil(t, githubFake.updates[0].RepositoryID)
		assert.Equal(t, "", *githubFake.updates[0].AutoFixPromptOverride)
		assert.Equal(t, mcphandlers.TaskChangeRequestAutomationStatusApplied, result.Status)
	})

	t.Run("mixed task update applies in provider order", func(t *testing.T) {
		order := []string{}
		githubFake := &taskChangeRequestAutomationGitHubFake{
			prs:       []*github.TaskPR{automationGitHubChangeForTest()},
			options:   &github.TaskCIOptionsResponse{PROptions: []*github.TaskPRAutomationOptions{{RepositoryID: "repo-gh", PRNumber: 8}}},
			callOrder: &order,
		}
		gitlabFake := &taskChangeRequestAutomationGitLabFake{
			mrs:       []*gitlab.TaskMR{automationGitLabChangeForTest()},
			options:   &gitlab.TaskMRAutomationResponse{MROptions: []*gitlab.TaskMRAutomationOptionsForMR{{RepositoryID: "repo-gl", ProjectPath: "group/project", MRIID: 42}}},
			callOrder: &order,
		}
		coordinator := newAutomationCoordinatorForTest(githubFake, gitlabFake)
		result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
			Target: mcphandlers.TaskChangeRequestAutomationTarget{Scope: mcphandlers.TaskChangeRequestAutomationScopeTask, Providers: []string{"gitlab", "github"}},
			Patch:  mcphandlers.TaskChangeRequestAutomationPatch{PromptOnMerged: &trueValue},
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"github", "gitlab"}, order)
		assert.Equal(t, mcphandlers.TaskChangeRequestAutomationStatusApplied, result.Status)
		assert.Len(t, githubFake.updates, 1)
		assert.Len(t, gitlabFake.updates, 1)
	})
}

func TestChangeRequestAutomationPreflightRejectsUnresolvedWithoutWrites(t *testing.T) {
	gitlabFake := &taskChangeRequestAutomationGitLabFake{
		mrs:     []*gitlab.TaskMR{{ProjectPath: "group/project", MRIID: 42, Host: "https://gitlab.example.test"}},
		options: &gitlab.TaskMRAutomationResponse{},
	}
	coordinator := newAutomationCoordinatorForTest(&taskChangeRequestAutomationGitHubFake{}, gitlabFake)
	value := true
	result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
		Target: mcphandlers.TaskChangeRequestAutomationTarget{Scope: mcphandlers.TaskChangeRequestAutomationScopeTask, Providers: []string{"gitlab"}},
		Patch:  mcphandlers.TaskChangeRequestAutomationPatch{AutoFixEnabled: &value},
	})
	require.Error(t, err)
	assert.Empty(t, gitlabFake.updates)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationStatusFailed, result.Status)
	require.Len(t, result.Providers, 1)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationProviderFailed, result.Providers[0].Status)
	assert.Equal(t, "preflight_failed", result.Providers[0].ErrorCode)
}

func TestChangeRequestAutomationPreflightRequiresSelectedProviderOnTask(t *testing.T) {
	prompt := "use this prompt"
	gitlabFake := &taskChangeRequestAutomationGitLabFake{}
	lookup := taskChangeRequestAutomationTaskLookup{
		task: &models.Task{
			ID: "task-1", WorkspaceID: "workspace-1",
			Repositories: []*models.TaskRepository{{RepositoryID: "repo-gh"}},
		},
	}
	coordinator := newTaskChangeRequestAutomationCoordinator(
		lookup, &taskChangeRequestAutomationGitHubFake{}, gitlabFake, nil, nil,
	)

	result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
		Target: mcphandlers.TaskChangeRequestAutomationTarget{
			Scope: mcphandlers.TaskChangeRequestAutomationScopeTask, Providers: []string{"gitlab"},
		},
		Patch: mcphandlers.TaskChangeRequestAutomationPatch{
			AutoFixPromptOverride: &prompt,
		},
	})

	require.Error(t, err)
	assert.Empty(t, gitlabFake.updates)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationStatusFailed, result.Status)
	require.Len(t, result.Providers, 1)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationProviderFailed, result.Providers[0].Status)
	assert.Equal(t, "preflight_failed", result.Providers[0].ErrorCode)
	assert.Contains(t, result.Providers[0].ErrorMessage, "not attached")
}

func TestChangeRequestAutomationAssociationReportsOnlyTargetedGitLabMR(t *testing.T) {
	value := false
	gitlabFake := &taskChangeRequestAutomationGitLabFake{
		mrs: []*gitlab.TaskMR{
			{RepositoryID: "repo-gl-a", ProjectPath: "group/project-a", MRIID: 42, Host: "https://gitlab.example.test", State: "opened"},
			{RepositoryID: "repo-gl-b", ProjectPath: "group/project-b", MRIID: 7, Host: "https://gitlab.example.test", State: "opened"},
		},
		// The production service returns the complete task-wide MROptions list
		// after a single-MR patch. The MCP result must still identify only the
		// association requested by the caller.
		options: &gitlab.TaskMRAutomationResponse{MROptions: []*gitlab.TaskMRAutomationOptionsForMR{
			{RepositoryID: "repo-gl-a", ProjectPath: "group/project-a", MRIID: 42},
			{RepositoryID: "repo-gl-b", ProjectPath: "group/project-b", MRIID: 7},
		}},
	}
	coordinator := newAutomationCoordinatorForTest(&taskChangeRequestAutomationGitHubFake{}, gitlabFake)

	result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
		Target: mcphandlers.TaskChangeRequestAutomationTarget{
			Scope: mcphandlers.TaskChangeRequestAutomationScopeAssociation, Provider: "gitlab", RepositoryID: "repo-gl-a", Number: 42,
		},
		Patch: mcphandlers.TaskChangeRequestAutomationPatch{AutoFixEnabled: &value},
	})

	require.NoError(t, err)
	require.Len(t, result.Providers, 1)
	require.Len(t, result.Providers[0].Affected, 1)
	assert.Equal(t, mcphandlers.TaskChangeRequestProviderGitLab, result.Providers[0].Affected[0].Provider)
	assert.Equal(t, "repo-gl-a", result.Providers[0].Affected[0].RepositoryID)
	assert.Equal(t, 42, result.Providers[0].Affected[0].Number)
}

func TestChangeRequestAutomationPartialFailurePreservesAppliedProvider(t *testing.T) {
	order := []string{}
	githubFake := &taskChangeRequestAutomationGitHubFake{
		prs:       []*github.TaskPR{automationGitHubChangeForTest()},
		options:   &github.TaskCIOptionsResponse{PROptions: []*github.TaskPRAutomationOptions{{RepositoryID: "repo-gh", PRNumber: 8}}},
		callOrder: &order,
	}
	gitlabFake := &taskChangeRequestAutomationGitLabFake{
		mrs:       []*gitlab.TaskMR{automationGitLabChangeForTest()},
		options:   &gitlab.TaskMRAutomationResponse{MROptions: []*gitlab.TaskMRAutomationOptionsForMR{{RepositoryID: "repo-gl", ProjectPath: "group/project", MRIID: 42}}},
		updateErr: errors.New("second provider write failed"),
		callOrder: &order,
	}
	coordinator := newAutomationCoordinatorForTest(githubFake, gitlabFake)
	value := true
	result, err := coordinator.UpdateTaskChangeRequestAutomation(context.Background(), "task-1", mcphandlers.TaskChangeRequestAutomationRequest{
		Target: mcphandlers.TaskChangeRequestAutomationTarget{Scope: mcphandlers.TaskChangeRequestAutomationScopeTask, Providers: []string{"github", "gitlab"}},
		Patch:  mcphandlers.TaskChangeRequestAutomationPatch{AutoFixEnabled: &value},
	})
	require.Error(t, err)
	assert.Equal(t, []string{"github", "gitlab"}, order)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationStatusPartial, result.Status)
	require.Len(t, result.Providers, 2)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationProviderApplied, result.Providers[0].Status)
	assert.Equal(t, mcphandlers.TaskChangeRequestAutomationProviderFailed, result.Providers[1].Status)
	assert.True(t, result.Providers[0].StateKnown)
	assert.False(t, result.Providers[1].StateKnown)
	assert.Equal(t, "provider_update_failed", result.Providers[1].ErrorCode)
}
