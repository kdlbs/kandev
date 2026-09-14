package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskChangeRequestReadTaskLookup struct {
	task *models.Task
}

func (l taskChangeRequestReadTaskLookup) GetTask(context.Context, string) (*models.Task, error) {
	return l.task, nil
}

func (l taskChangeRequestReadTaskLookup) GetRepository(context.Context, string) (*models.Repository, error) {
	return nil, nil
}

type taskChangeRequestReadProviderFake struct {
	name   string
	result taskChangeRequestProviderResult
}

func (p taskChangeRequestReadProviderFake) Name() string { return p.name }

func (p taskChangeRequestReadProviderFake) ReadTaskChangeRequests(context.Context, *models.Task) taskChangeRequestProviderResult {
	return p.result
}

func TestTaskChangeRequestReaderPreservesProviderAvailabilityAndFailures(t *testing.T) {
	now := time.Now().UTC()
	githubRepositoryID := "repo-gh"
	github := taskChangeRequestReadProviderFake{
		name: mcphandlers.TaskChangeRequestProviderGitHub,
		result: taskChangeRequestProviderResult{
			Configured: true,
			Available:  true,
			Status:     mcphandlers.TaskChangeRequestProviderStatusAvailable,
			Settings: &mcphandlers.TaskChangeRequestProviderSettings{
				Provider:         mcphandlers.TaskChangeRequestProviderGitHub,
				AutoFixMaxRounds: 10,
				PromptScope:      mcphandlers.TaskChangeRequestPromptScopeTaskProvider,
			},
			ChangeRequests: []mcphandlers.TaskChangeRequest{{
				Provider:       mcphandlers.TaskChangeRequestProviderGitHub,
				RepositoryID:   &githubRepositoryID,
				Number:         3506,
				State:          "open",
				IdentityStatus: mcphandlers.TaskChangeRequestIdentityResolved,
				UpdatedAt:      &now,
			}},
		},
	}
	gitlab := taskChangeRequestReadProviderFake{
		name: mcphandlers.TaskChangeRequestProviderGitLab,
		result: taskChangeRequestProviderResult{
			Configured: false,
			Available:  false,
			Status:     mcphandlers.TaskChangeRequestProviderStatusUnavailable,
			ReasonCode: "provider_not_configured",
			Errors:     []error{errors.New("gitlab read must be surfaced")},
		},
	}
	reader := taskChangeRequestReader{
		tasks:     taskChangeRequestReadTaskLookup{task: &models.Task{ID: "task-1", WorkspaceID: "workspace-1"}},
		providers: []taskChangeRequestProviderAdapter{github, gitlab},
	}

	got, err := reader.GetTaskChangeRequests(context.Background(), "task-1")
	require.NoError(t, err)
	assert.Equal(t, "task-1", got.TaskID)
	assert.False(t, got.Complete)
	assert.Len(t, got.ChangeRequests, 1)
	assert.Equal(t, mcphandlers.TaskChangeRequestProviderGitHub, got.ChangeRequests[0].Provider)
	assert.Len(t, got.ProviderCapabilities, 2)
	assert.Equal(t, "provider_read_failed", got.ProviderCapabilities[1].ReasonCode)
	assert.False(t, got.ProviderCapabilities[1].Available)
	assert.Len(t, got.Errors, 1)
	assert.Equal(t, mcphandlers.TaskChangeRequestProviderGitLab, got.Errors[0].Provider)
}

func TestGitHubTaskChangeRequestReadMapsSwitchesAndAutomationStatus(t *testing.T) {
	repositoryID := "repo-gh"
	change := githubTaskChangeRequest(&github.TaskPR{
		RepositoryID: repositoryID,
		PRNumber:     3506,
		State:        "open",
		IsDraft:      boolPtrForReadTest(true),
	})
	errorMessage := "check delivery failed"
	options := &github.TaskCIOptionsResponse{
		PROptions: []*github.TaskPRAutomationOptions{{
			RepositoryID: repositoryID, PRNumber: 3506,
			AutoFixEnabled: true, AutoMergeEnabled: false,
			PromptOnReviewRequested: true, PromptOnMerged: false, PromptOnClosed: true,
		}},
		PRStates: []*github.TaskCIPRAutomationState{{
			RepositoryID: repositoryID, PRNumber: 3506,
			AutoFixRoundCount: 2, LastError: &errorMessage,
		}},
	}

	changes := []mcphandlers.TaskChangeRequest{change}
	applyGitHubTaskChangeRequestAutomation(changes, options)
	assert.NotNil(t, changes[0].Automation)
	assert.True(t, *changes[0].Automation.AutoFixEnabled)
	assert.False(t, *changes[0].Automation.AutoMergeEnabled)
	assert.True(t, *changes[0].Automation.PromptOnReviewRequested)
	assert.False(t, *changes[0].Automation.PromptOnMerged)
	assert.True(t, *changes[0].Automation.PromptOnClosed)
	assert.NotNil(t, changes[0].AutomationStatus)
	assert.Equal(t, 2, *changes[0].AutomationStatus.AutoFixRoundCount)
	assert.Equal(t, errorMessage, *changes[0].AutomationStatus.LastError)
}

func TestGitLabTaskChangeRequestReadResolvesOnlyVerifiedIdentity(t *testing.T) {
	lookup := taskChangeRequestReadRepositoryLookup{repositories: map[string]*models.Repository{
		"repo-gl": {
			ID: "repo-gl", WorkspaceID: "workspace-1", Provider: "gitlab",
			ProviderHost: "https://gitlab.example.test", ProviderOwner: "group/subgroup", ProviderName: "project",
		},
	}}
	adapter := gitlabTaskChangeRequestAdapter{tasks: lookup}
	task := &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1",
		Repositories: []*models.TaskRepository{{RepositoryID: "repo-gl"}},
	}
	resolved := adapter.gitLabTaskChangeRequest(context.Background(), task, &gitlab.TaskMR{
		Host: "https://gitlab.example.test/", ProjectPath: "group/subgroup/project", MRIID: 12,
		State: "locked",
	})
	require.NotNil(t, resolved.RepositoryID)
	assert.Equal(t, "repo-gl", *resolved.RepositoryID)
	assert.Equal(t, mcphandlers.TaskChangeRequestIdentityResolved, resolved.IdentityStatus)
	assert.Equal(t, "unknown", resolved.State)
	assert.Equal(t, "locked", resolved.ProviderState)

	unresolved := adapter.gitLabTaskChangeRequest(context.Background(), task, &gitlab.TaskMR{
		Host: "https://gitlab.other.test", ProjectPath: "group/subgroup/project", MRIID: 12,
	})
	assert.Nil(t, unresolved.RepositoryID)
	assert.Equal(t, mcphandlers.TaskChangeRequestIdentityUnresolved, unresolved.IdentityStatus)
}

func TestGitLabTaskChangeRequestReadMarksDuplicateCanonicalIdentityAmbiguous(t *testing.T) {
	adapter := gitlabTaskChangeRequestAdapter{}
	changes := adapter.gitLabTaskChangeRequests(context.Background(), &models.Task{WorkspaceID: "workspace-1"}, []*gitlab.TaskMR{
		{RepositoryID: "repo-gl", ProjectPath: "group/one", MRIID: 12, MRURL: "url-one"},
		{RepositoryID: "repo-gl", ProjectPath: "group/two", MRIID: 12, MRURL: "url-two"},
	})
	require.Len(t, changes, 2)
	assert.Equal(t, mcphandlers.TaskChangeRequestIdentityAmbiguous, changes[0].IdentityStatus)
	assert.Equal(t, mcphandlers.TaskChangeRequestIdentityAmbiguous, changes[1].IdentityStatus)
}

type taskChangeRequestReadRepositoryLookup struct {
	repositories map[string]*models.Repository
}

func (l taskChangeRequestReadRepositoryLookup) GetTask(context.Context, string) (*models.Task, error) {
	return nil, nil
}

func (l taskChangeRequestReadRepositoryLookup) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	return l.repositories[id], nil
}

func boolPtrForReadTest(value bool) *bool {
	return &value
}
