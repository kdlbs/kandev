package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	azureDevOpsWorkItemWatchMetadataKey    = "azure_devops_work_item_watch_id"
	azureDevOpsPullRequestWatchMetadataKey = "azure_devops_pr_watch_id"
)

type AzureDevOpsWatchService interface {
	ReserveWorkItemWatchTask(context.Context, string, int64, string, int, string) (bool, error)
	AssignWorkItemWatchTaskID(context.Context, string, int64, string, int, string) error
	ReleaseWorkItemWatchTask(context.Context, string, int64, string, int) error
	DisableWorkItemWatchWithError(context.Context, string, string) error
	ReservePullRequestWatchTask(context.Context, string, int64, string, string, int, string) (bool, error)
	AssignPullRequestWatchTaskID(context.Context, string, int64, string, string, int, string) error
	ReleasePullRequestWatchTask(context.Context, string, int64, string, string, int) error
	DisablePullRequestWatchWithError(context.Context, string, string) error
}

type AzureDevOpsWorkItemWatcherSource struct {
	service AzureDevOpsWatchService
	logger  *logger.Logger
}

func NewAzureDevOpsWorkItemWatcherSource(svc AzureDevOpsWatchService, log *logger.Logger) *AzureDevOpsWorkItemWatcherSource {
	return &AzureDevOpsWorkItemWatcherSource{service: svc, logger: log}
}

func (s *AzureDevOpsWorkItemWatcherSource) Name() string { return "azure_devops_work_item" }

func (s *AzureDevOpsWorkItemWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, err := azureWorkItemWatchEvent(evt)
	if err != nil {
		return false, err
	}
	if s.service == nil {
		return true, nil
	}
	if e.ReservationClaimed {
		return true, nil
	}
	return s.service.ReserveWorkItemWatchTask(ctx, e.WatchID, e.WatchGeneration, e.ProjectID, e.WorkItem.ID, e.WorkItem.WebURL)
}

func (s *AzureDevOpsWorkItemWatcherSource) Release(ctx context.Context, evt any) {
	e, err := azureWorkItemWatchEvent(evt)
	if err != nil || s.service == nil {
		return
	}
	if releaseErr := s.service.ReleaseWorkItemWatchTask(ctx, e.WatchID, e.WatchGeneration, e.ProjectID, e.WorkItem.ID); releaseErr != nil && s.logger != nil {
		s.logger.Warn("azure devops work-item source: release failed", zap.Int("work_item_id", e.WorkItem.ID), zap.Error(releaseErr))
	}
}

func (s *AzureDevOpsWorkItemWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, err := azureWorkItemWatchEvent(evt)
	if err != nil {
		return nil, err
	}
	req := &IssueTaskRequest{
		WorkspaceID: e.WorkspaceID, WorkflowID: e.WorkflowID, WorkflowStepID: e.WorkflowStepID,
		Title:       fmt.Sprintf("[%s#%d] %s", e.ProjectID, e.WorkItem.ID, e.WorkItem.Title),
		Description: interpolateAzureWorkItemPrompt(e.Prompt, &e.WorkItem),
		Metadata: map[string]interface{}{
			azureDevOpsWorkItemWatchMetadataKey: e.WatchID,
			"azure_devops_project_id":           e.ProjectID,
			"azure_devops_work_item_id":         e.WorkItem.ID,
			"azure_devops_work_item_url":        e.WorkItem.WebURL,
			"azure_devops_work_item_type":       e.WorkItem.Type,
			"azure_devops_work_item_state":      e.WorkItem.State,
			models.MetaKeyAgentProfileID:        e.AgentProfileID,
			models.MetaKeyExecutorProfileID:     e.ExecutorProfileID,
		},
	}
	if e.RepositoryID != "" {
		req.Repositories = []IssueTaskRepository{{RepositoryID: e.RepositoryID, BaseBranch: e.BaseBranch}}
	}
	return req, nil
}

func (s *AzureDevOpsWorkItemWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, err := azureWorkItemWatchEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.AssignWorkItemWatchTaskID(ctx, e.WatchID, e.WatchGeneration, e.ProjectID, e.WorkItem.ID, taskID)
}

func (s *AzureDevOpsWorkItemWatcherSource) IsTerminalAttachError(err error) bool {
	return errors.Is(err, azuredevops.ErrWatchOwnershipLost)
}

func (s *AzureDevOpsWorkItemWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, _ := azureWorkItemWatchEvent(evt)
	if e == nil {
		return AutoStartParams{}
	}
	return AutoStartParams{AgentProfileID: e.AgentProfileID, ExecutorProfileID: e.ExecutorProfileID, WorkflowStepID: e.WorkflowStepID}
}

func (s *AzureDevOpsWorkItemWatcherSource) AgentProfileID(evt any) string {
	e, _ := azureWorkItemWatchEvent(evt)
	if e == nil {
		return ""
	}
	return e.AgentProfileID
}

func (s *AzureDevOpsWorkItemWatcherSource) WatchID(evt any) string {
	e, _ := azureWorkItemWatchEvent(evt)
	if e == nil {
		return ""
	}
	return e.WatchID
}

func (s *AzureDevOpsWorkItemWatcherSource) MaxInflightTasks(evt any) *int {
	e, _ := azureWorkItemWatchEvent(evt)
	if e == nil {
		return nil
	}
	return e.MaxInflightTasks
}

func (s *AzureDevOpsWorkItemWatcherSource) WatchMetadataKey() string {
	return azureDevOpsWorkItemWatchMetadataKey
}

func (s *AzureDevOpsWorkItemWatcherSource) SelfHeal(ctx context.Context, evt any, cause string) error {
	e, err := azureWorkItemWatchEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.DisableWorkItemWatchWithError(ctx, e.WatchID, cause)
}

type AzureDevOpsPullRequestWatcherSource struct {
	service AzureDevOpsWatchService
	logger  *logger.Logger
}

func NewAzureDevOpsPullRequestWatcherSource(svc AzureDevOpsWatchService, log *logger.Logger) *AzureDevOpsPullRequestWatcherSource {
	return &AzureDevOpsPullRequestWatcherSource{service: svc, logger: log}
}

func (s *AzureDevOpsPullRequestWatcherSource) Name() string { return "azure_devops_pull_request" }

func (s *AzureDevOpsPullRequestWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, err := azurePullRequestWatchEvent(evt)
	if err != nil {
		return false, err
	}
	if s.service == nil {
		return true, nil
	}
	if e.ReservationClaimed {
		return true, nil
	}
	return s.service.ReservePullRequestWatchTask(ctx, e.WatchID, e.WatchGeneration, e.ProjectID, e.AzureRepositoryID, e.PullRequest.ID, e.PullRequest.WebURL)
}

func (s *AzureDevOpsPullRequestWatcherSource) Release(ctx context.Context, evt any) {
	e, err := azurePullRequestWatchEvent(evt)
	if err != nil || s.service == nil {
		return
	}
	if releaseErr := s.service.ReleasePullRequestWatchTask(ctx, e.WatchID, e.WatchGeneration, e.ProjectID, e.AzureRepositoryID, e.PullRequest.ID); releaseErr != nil && s.logger != nil {
		s.logger.Warn("azure devops pull-request source: release failed", zap.Int("pull_request_id", e.PullRequest.ID), zap.Error(releaseErr))
	}
}

func (s *AzureDevOpsPullRequestWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, err := azurePullRequestWatchEvent(evt)
	if err != nil {
		return nil, err
	}
	req := &IssueTaskRequest{
		WorkspaceID: e.WorkspaceID, WorkflowID: e.WorkflowID, WorkflowStepID: e.WorkflowStepID,
		Title:       fmt.Sprintf("[%s!%d] %s", e.ProjectID, e.PullRequest.ID, e.PullRequest.Title),
		Description: interpolateAzurePullRequestPrompt(e.Prompt, &e.PullRequest),
		Metadata: map[string]interface{}{
			azureDevOpsPullRequestWatchMetadataKey: e.WatchID,
			"azure_devops_project_id":              e.ProjectID,
			"azure_devops_repository_id":           e.AzureRepositoryID,
			"azure_devops_pull_request_id":         e.PullRequest.ID,
			"azure_devops_pull_request_url":        e.PullRequest.WebURL,
			models.MetaKeyAgentProfileID:           e.AgentProfileID,
			models.MetaKeyExecutorProfileID:        e.ExecutorProfileID,
		},
	}
	if e.RepositoryID != "" {
		req.Repositories = []IssueTaskRepository{{RepositoryID: e.RepositoryID, BaseBranch: e.BaseBranch}}
	}
	return req, nil
}

func (s *AzureDevOpsPullRequestWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, err := azurePullRequestWatchEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.AssignPullRequestWatchTaskID(ctx, e.WatchID, e.WatchGeneration, e.ProjectID, e.AzureRepositoryID, e.PullRequest.ID, taskID)
}

func (s *AzureDevOpsPullRequestWatcherSource) IsTerminalAttachError(err error) bool {
	return errors.Is(err, azuredevops.ErrWatchOwnershipLost)
}

func (s *AzureDevOpsPullRequestWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, _ := azurePullRequestWatchEvent(evt)
	if e == nil {
		return AutoStartParams{}
	}
	return AutoStartParams{AgentProfileID: e.AgentProfileID, ExecutorProfileID: e.ExecutorProfileID, WorkflowStepID: e.WorkflowStepID}
}

func (s *AzureDevOpsPullRequestWatcherSource) AgentProfileID(evt any) string {
	e, _ := azurePullRequestWatchEvent(evt)
	if e == nil {
		return ""
	}
	return e.AgentProfileID
}

func (s *AzureDevOpsPullRequestWatcherSource) WatchID(evt any) string {
	e, _ := azurePullRequestWatchEvent(evt)
	if e == nil {
		return ""
	}
	return e.WatchID
}

func (s *AzureDevOpsPullRequestWatcherSource) MaxInflightTasks(evt any) *int {
	e, _ := azurePullRequestWatchEvent(evt)
	if e == nil {
		return nil
	}
	return e.MaxInflightTasks
}

func (s *AzureDevOpsPullRequestWatcherSource) WatchMetadataKey() string {
	return azureDevOpsPullRequestWatchMetadataKey
}

func (s *AzureDevOpsPullRequestWatcherSource) SelfHeal(ctx context.Context, evt any, cause string) error {
	e, err := azurePullRequestWatchEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.DisablePullRequestWatchWithError(ctx, e.WatchID, cause)
}

func azureWorkItemWatchEvent(evt any) (*azuredevops.WorkItemWatchEvent, error) {
	e, ok := evt.(*azuredevops.WorkItemWatchEvent)
	if !ok || e == nil {
		return nil, errors.New("azure devops work-item source: event payload missing or wrong type")
	}
	return e, nil
}

func azurePullRequestWatchEvent(evt any) (*azuredevops.PullRequestWatchEvent, error) {
	e, ok := evt.(*azuredevops.PullRequestWatchEvent)
	if !ok || e == nil {
		return nil, errors.New("azure devops pull-request source: event payload missing or wrong type")
	}
	return e, nil
}

func interpolateAzureWorkItemPrompt(template string, item *azuredevops.WorkItem) string {
	if item == nil {
		return template
	}
	return strings.NewReplacer(
		"{{work_item.url}}", item.WebURL,
		"{{work_item.title}}", item.Title,
		"{{work_item.project}}", item.Project,
		"{{work_item.id}}", fmt.Sprint(item.ID),
		"{{work_item.description}}", item.Description,
		"{{work_item.state}}", item.State,
		"{{work_item.type}}", item.Type,
	).Replace(template)
}

func interpolateAzurePullRequestPrompt(template string, pullRequest *azuredevops.PullRequest) string {
	if pullRequest == nil {
		return template
	}
	return strings.NewReplacer(
		"{{pull_request.url}}", pullRequest.WebURL,
		"{{pull_request.title}}", pullRequest.Title,
		"{{pull_request.project}}", pullRequest.ProjectName,
		"{{pull_request.id}}", fmt.Sprint(pullRequest.ID),
		"{{pull_request.description}}", pullRequest.Description,
		"{{pull_request.state}}", pullRequest.Status,
		"{{pull_request.source_branch}}", pullRequest.SourceBranch,
		"{{pull_request.target_branch}}", pullRequest.TargetBranch,
	).Replace(template)
}
