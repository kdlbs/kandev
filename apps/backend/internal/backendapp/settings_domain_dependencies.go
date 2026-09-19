package backendapp

import (
	"context"
	"encoding/json"

	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/azuredevops"
	editormodels "github.com/kandev/kandev/internal/editors/models"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/notifications/dto"
	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/sentry"
	storage "github.com/kandev/kandev/internal/system/storage"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	workflowcontroller "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

type settingsTaskService interface {
	GetWorkflow(context.Context, string) (*taskmodels.Workflow, error)
	UpdateWorkflow(context.Context, string, *taskservice.UpdateWorkflowRequest) (*taskmodels.Workflow, error)
	ListWorkflows(context.Context, string, bool) ([]*taskmodels.Workflow, error)
	GetWorkspace(context.Context, string) (*taskmodels.Workspace, error)
	UpdateWorkspace(context.Context, string, *taskservice.UpdateWorkspaceRequest) (*taskmodels.Workspace, error)
	ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error)
	GetRepository(context.Context, string) (*taskmodels.Repository, error)
	UpdateRepository(context.Context, string, *taskservice.UpdateRepositoryRequest) (*taskmodels.Repository, error)
	ListRepositories(context.Context, string) ([]*taskmodels.Repository, error)
	GetRepositoryScript(context.Context, string) (*taskmodels.RepositoryScript, error)
	UpdateRepositoryScript(context.Context, string, *taskservice.UpdateRepositoryScriptRequest) (*taskmodels.RepositoryScript, error)
	ListRepositoryScripts(context.Context, string) ([]*taskmodels.RepositoryScript, error)
	GetRepositorySet(context.Context, string) (*taskmodels.RepositorySet, error)
	UpdateRepositorySet(context.Context, string, *taskservice.UpdateRepositorySetRequest) (*taskmodels.RepositorySet, error)
	ListRepositorySets(context.Context, string) ([]*taskmodels.RepositorySet, error)
	GetExecutor(context.Context, string) (*taskmodels.Executor, error)
	UpdateExecutor(context.Context, string, *taskservice.UpdateExecutorRequest) (*taskmodels.Executor, error)
	ListExecutors(context.Context) ([]*taskmodels.Executor, error)
	GetExecutorProfile(context.Context, string) (*taskmodels.ExecutorProfile, error)
	UpdateExecutorProfile(context.Context, string, *taskservice.UpdateExecutorProfileRequest) (*taskmodels.ExecutorProfile, error)
	ListAllExecutorProfiles(context.Context) ([]*taskmodels.ExecutorProfile, error)
	GetEnvironment(context.Context, string) (*taskmodels.Environment, error)
	UpdateEnvironment(context.Context, string, *taskservice.UpdateEnvironmentRequest) (*taskmodels.Environment, error)
	ListEnvironments(context.Context) ([]*taskmodels.Environment, error)
	GetTask(context.Context, string) (*taskmodels.Task, error)
	UpdateTask(context.Context, string, *taskservice.UpdateTaskRequest) (*taskmodels.Task, error)
	ListTasks(context.Context, string) ([]*taskmodels.Task, error)
	AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error
}

type settingsWorkflowService interface {
	ListStepsByWorkflow(context.Context, string) ([]*workflowmodels.WorkflowStep, error)
	ListStepsByWorkspaceID(context.Context, string) ([]*workflowmodels.WorkflowStep, error)
}

type settingsWorkflowController interface {
	GetStep(context.Context, string) (*workflowcontroller.GetStepResponse, error)
	UpdateStep(context.Context, workflowcontroller.UpdateStepRequest) (*workflowcontroller.GetStepResponse, error)
}

type settingsPromptService interface {
	ListPrompts(context.Context) ([]*promptmodels.Prompt, error)
	UpdatePrompt(context.Context, string, *string, *string) (*promptmodels.Prompt, error)
}

type settingsUtilityService interface {
	ListAgents(context.Context) ([]*utilitymodels.UtilityAgent, error)
	GetAgentByID(context.Context, string) (*utilitymodels.UtilityAgent, error)
	UpdateAgent(context.Context, string, *string, *string, *string, *string, *string, *string, *string, *bool) (*utilitymodels.UtilityAgent, error)
}

type settingsEditorService interface {
	ListEditors(context.Context) ([]*editormodels.Editor, error)
	UpdateEditor(context.Context, editorservice.UpdateEditorInput) (*editormodels.Editor, error)
}

type settingsNotificationController interface {
	ListProviders(context.Context) (dto.NotificationProvidersResponse, error)
	UpdateProvider(context.Context, string, dto.UpdateProviderRequest) (dto.NotificationProviderDTO, error)
}

type settingsRuntimeFlagsService interface {
	ListStates(context.Context) ([]runtimeflags.RuntimeFlagState, error)
	SetOverride(context.Context, string, *bool) ([]runtimeflags.RuntimeFlagState, error)
}

type settingsStorageService interface {
	GetSettings(context.Context) (storage.StorageMaintenanceSettings, error)
	SaveSettingsWithConfirmations(context.Context, storage.StorageMaintenanceSettings, storage.SaveConfirmations) (storage.StorageMaintenanceSettings, error)
	PatchSettingsWithConfirmations(context.Context, map[string]json.RawMessage, storage.SaveConfirmations) (storage.StorageMaintenanceSettings, error)
}

type settingsAutomationService interface {
	GetAutomation(context.Context, string) (*automation.Automation, error)
	ListAutomations(context.Context, string) ([]*automation.Automation, error)
	UpdateAutomation(context.Context, string, *automation.UpdateAutomationRequest) (*automation.Automation, error)
	GetTrigger(context.Context, string) (*automation.AutomationTrigger, error)
	UpdateTrigger(context.Context, string, *automation.UpdateTriggerRequest) error
}

type settingsAgentService interface {
	GetAgent(context.Context, string) (*agentsettingsdto.AgentDTO, error)
	ListAgents(context.Context) (*agentsettingsdto.ListAgentsResponse, error)
	UpdateAgent(context.Context, agentsettingscontroller.UpdateAgentRequest) (*agentsettingsdto.AgentDTO, error)
}

type settingsJiraService interface {
	GetConfigForWorkspace(context.Context, string) (*jira.JiraConfig, error)
	SetConfigForWorkspace(context.Context, string, *jira.SetConfigRequest) (*jira.JiraConfig, error)
	GetIssueWatch(context.Context, string) (*jira.IssueWatch, error)
	ListIssueWatches(context.Context, string) ([]*jira.IssueWatch, error)
	UpdateIssueWatch(context.Context, string, *jira.UpdateIssueWatchRequest) (*jira.IssueWatch, error)
}

type settingsLinearService interface {
	GetConfigForWorkspace(context.Context, string) (*linear.LinearConfig, error)
	SetConfigForWorkspace(context.Context, string, *linear.SetConfigRequest) (*linear.LinearConfig, error)
	GetIssueWatch(context.Context, string) (*linear.IssueWatch, error)
	ListIssueWatches(context.Context, string) ([]*linear.IssueWatch, error)
	UpdateIssueWatch(context.Context, string, *linear.UpdateIssueWatchRequest) (*linear.IssueWatch, error)
}

type settingsSentryService interface {
	GetInstance(context.Context, string, string) (*sentry.SentryConfig, error)
	ListInstances(context.Context, string) ([]*sentry.SentryConfig, error)
	UpdateInstance(context.Context, string, string, *sentry.UpdateConfigRequest) (*sentry.SentryConfig, error)
	GetIssueWatch(context.Context, string) (*sentry.IssueWatch, error)
	ListIssueWatches(context.Context, string) ([]*sentry.IssueWatch, error)
	UpdateIssueWatch(context.Context, string, *sentry.UpdateIssueWatchRequest) (*sentry.IssueWatch, error)
}

type settingsGitHubService interface {
	GetWorkspaceSettings(context.Context, string) (*github.WorkspaceSettings, error)
	UpdateWorkspaceSettings(context.Context, *github.UpdateWorkspaceSettingsRequest) (*github.WorkspaceSettings, error)
	GetReviewWatch(context.Context, string) (*github.ReviewWatch, error)
	ListReviewWatches(context.Context, string) ([]*github.ReviewWatch, error)
	UpdateReviewWatch(context.Context, string, *github.UpdateReviewWatchRequest) error
	GetIssueWatch(context.Context, string) (*github.IssueWatch, error)
	ListIssueWatches(context.Context, string) ([]*github.IssueWatch, error)
	UpdateIssueWatch(context.Context, string, *github.UpdateIssueWatchRequest) error
}

type settingsGitLabService interface {
	GetConfigForWorkspace(context.Context, string) (*gitlab.GitLabConfig, error)
	SetConfigForWorkspace(context.Context, string, *gitlab.SetConfigRequest) (*gitlab.GitLabConfig, error)
	GetReviewWatch(context.Context, string) (*gitlab.ReviewWatch, error)
	ListReviewWatches(context.Context, string) ([]*gitlab.ReviewWatch, error)
	UpdateReviewWatch(context.Context, string, *gitlab.UpdateReviewWatchRequest) error
	GetIssueWatch(context.Context, string) (*gitlab.IssueWatch, error)
	ListIssueWatches(context.Context, string) ([]*gitlab.IssueWatch, error)
	UpdateIssueWatch(context.Context, string, *gitlab.UpdateIssueWatchRequest) error
}

type settingsAzureDevOpsService interface {
	GetConfigForWorkspace(context.Context, string) (*azuredevops.Config, error)
	SetConfigForWorkspace(context.Context, string, *azuredevops.SetConfigRequest) (*azuredevops.Config, error)
	GetWorkspaceSettings(context.Context, string) (*azuredevops.WorkspaceSettings, error)
	UpdateWorkspaceSettings(context.Context, *azuredevops.UpdateWorkspaceSettingsRequest) (*azuredevops.WorkspaceSettings, error)
	ListWorkItemWatches(context.Context, string) ([]*azuredevops.WorkItemWatch, error)
	UpdateWorkItemWatch(context.Context, string, string, *azuredevops.UpdateWorkItemWatchRequest) (*azuredevops.WorkItemWatch, error)
	ListPullRequestWatches(context.Context, string) ([]*azuredevops.PullRequestWatch, error)
	UpdatePullRequestWatch(context.Context, string, string, *azuredevops.UpdatePullRequestWatchRequest) (*azuredevops.PullRequestWatch, error)
}

func automationServiceFromComponents(components *automation.Components) settingsAutomationService {
	if components == nil || components.Service == nil {
		return nil
	}
	return components.Service
}

func settingsJiraServiceFromPointer(service *jira.Service) settingsJiraService {
	if service == nil {
		return nil
	}
	return service
}

func settingsLinearServiceFromPointer(service *linear.Service) settingsLinearService {
	if service == nil {
		return nil
	}
	return service
}

func settingsSentryServiceFromPointer(service *sentry.Service) settingsSentryService {
	if service == nil {
		return nil
	}
	return service
}

func settingsGitHubServiceFromPointer(service *github.Service) settingsGitHubService {
	if service == nil {
		return nil
	}
	return service
}

func settingsGitLabServiceFromPointer(service *gitlab.Service) settingsGitLabService {
	if service == nil {
		return nil
	}
	return service
}

func settingsAzureDevOpsServiceFromPointer(service *azuredevops.Service) settingsAzureDevOpsService {
	if service == nil {
		return nil
	}
	return service
}

var _ settingsTaskService = (*taskservice.Service)(nil)
var _ settingsPromptService = (*promptservice.Service)(nil)
var _ settingsUtilityService = (*utilityservice.Service)(nil)
var _ settingsEditorService = (*editorservice.Service)(nil)
var _ settingsRuntimeFlagsService = (*runtimeflags.Service)(nil)
var _ settingsAgentService = (*agentsettingscontroller.Controller)(nil)
