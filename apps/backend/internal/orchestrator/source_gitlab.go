package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	gitlabReviewWatchMetadataKey = "gitlab_review_watch_id"
	gitlabIssueWatchMetadataKey  = "gitlab_issue_watch_id"
)

// GitLabWatchService is the dedup and self-heal surface used by both GitLab
// watcher sources. Keeping it narrow lets the orchestrator own dispatch
// without depending on GitLab's polling implementation.
type GitLabWatchService interface {
	ReserveReviewMRTask(context.Context, string, int64, string, int, string) (bool, error)
	AssignReviewMRTaskID(context.Context, string, int64, string, int, string) error
	ReleaseReviewMRTask(context.Context, string, int64, string, int) error
	DisableReviewWatchWithError(context.Context, string, string) error
	ReserveIssueWatchTask(context.Context, string, int64, string, int, string) (bool, error)
	AssignIssueWatchTaskID(context.Context, string, int64, string, int, string) error
	ReleaseIssueWatchTask(context.Context, string, int64, string, int) error
	DisableIssueWatchWithError(context.Context, string, string) error
}

type GitLabReviewWatcherSource struct {
	service GitLabWatchService
	logger  *logger.Logger
}

func NewGitLabReviewWatcherSource(svc GitLabWatchService, log *logger.Logger) *GitLabReviewWatcherSource {
	return &GitLabReviewWatcherSource{service: svc, logger: log}
}

func (s *GitLabReviewWatcherSource) Name() string { return "gitlab_review" }

func (s *GitLabReviewWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, err := gitlabReviewEvent(evt)
	if err != nil {
		return false, err
	}
	if s.service == nil {
		return true, nil
	}
	return s.service.ReserveReviewMRTask(ctx, e.ReviewWatchID, e.WatchGeneration, e.MR.ProjectPath, e.MR.IID, e.MR.WebURL)
}

func (s *GitLabReviewWatcherSource) Release(ctx context.Context, evt any) {
	e, err := gitlabReviewEvent(evt)
	if err != nil || s.service == nil {
		return
	}
	if err := s.service.ReleaseReviewMRTask(ctx, e.ReviewWatchID, e.WatchGeneration, e.MR.ProjectPath, e.MR.IID); err != nil && s.logger != nil {
		s.logger.Warn("gitlab review source: release failed", zap.Error(err))
	}
}

func (s *GitLabReviewWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, err := gitlabReviewEvent(evt)
	if err != nil {
		return nil, err
	}
	req := gitlabTaskRequest(e.WorkspaceID, e.WorkflowID, e.WorkflowStepID,
		fmt.Sprintf("[%s!%d] %s", e.MR.ProjectPath, e.MR.IID, e.MR.Title),
		interpolateGitLabMRPrompt(e.Prompt, e.MR), e.RepositoryID, e.BaseBranch,
		map[string]interface{}{
			gitlabReviewWatchMetadataKey:    e.ReviewWatchID,
			"gitlab_project_path":           e.MR.ProjectPath,
			"gitlab_mr_iid":                 e.MR.IID,
			"gitlab_mr_url":                 e.MR.WebURL,
			models.MetaKeyAgentProfileID:    e.AgentProfileID,
			models.MetaKeyExecutorProfileID: e.ExecutorProfileID,
		})
	return req, nil
}

func (s *GitLabReviewWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, err := gitlabReviewEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.AssignReviewMRTaskID(ctx, e.ReviewWatchID, e.WatchGeneration, e.MR.ProjectPath, e.MR.IID, taskID)
}

func (s *GitLabReviewWatcherSource) IsTerminalAttachError(err error) bool {
	return errors.Is(err, gitlab.ErrWatchOwnershipLost)
}

func (s *GitLabReviewWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, err := gitlabReviewEvent(evt)
	if err != nil {
		return AutoStartParams{}
	}
	return gitlabAutoStartParams(e.AgentProfileID, e.ExecutorProfileID, e.WorkflowStepID)
}

func (s *GitLabReviewWatcherSource) AgentProfileID(evt any) string {
	e, _ := gitlabReviewEvent(evt)
	if e == nil {
		return ""
	}
	return e.AgentProfileID
}

func (s *GitLabReviewWatcherSource) WatchID(evt any) string {
	e, _ := gitlabReviewEvent(evt)
	if e == nil {
		return ""
	}
	return e.ReviewWatchID
}

func (s *GitLabReviewWatcherSource) MaxInflightTasks(evt any) *int {
	e, _ := gitlabReviewEvent(evt)
	if e == nil {
		return nil
	}
	return e.MaxInflightTasks
}

func (s *GitLabReviewWatcherSource) WatchMetadataKey() string { return gitlabReviewWatchMetadataKey }

func (s *GitLabReviewWatcherSource) SelfHeal(ctx context.Context, evt any, cause string) error {
	e, err := gitlabReviewEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.DisableReviewWatchWithError(ctx, e.ReviewWatchID, cause)
}

type GitLabIssueWatcherSource struct {
	service GitLabWatchService
	logger  *logger.Logger
}

func NewGitLabIssueWatcherSource(svc GitLabWatchService, log *logger.Logger) *GitLabIssueWatcherSource {
	return &GitLabIssueWatcherSource{service: svc, logger: log}
}

func (s *GitLabIssueWatcherSource) Name() string { return "gitlab_issue" }

func (s *GitLabIssueWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, err := gitlabIssueEvent(evt)
	if err != nil {
		return false, err
	}
	if s.service == nil {
		return true, nil
	}
	return s.service.ReserveIssueWatchTask(ctx, e.IssueWatchID, e.WatchGeneration, e.Issue.ProjectPath, e.Issue.IID, e.Issue.WebURL)
}

func (s *GitLabIssueWatcherSource) Release(ctx context.Context, evt any) {
	e, err := gitlabIssueEvent(evt)
	if err != nil || s.service == nil {
		return
	}
	if err := s.service.ReleaseIssueWatchTask(ctx, e.IssueWatchID, e.WatchGeneration, e.Issue.ProjectPath, e.Issue.IID); err != nil && s.logger != nil {
		s.logger.Warn("gitlab issue source: release failed", zap.Error(err))
	}
}

func (s *GitLabIssueWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, err := gitlabIssueEvent(evt)
	if err != nil {
		return nil, err
	}
	return gitlabTaskRequest(e.WorkspaceID, e.WorkflowID, e.WorkflowStepID,
		fmt.Sprintf("[%s#%d] %s", e.Issue.ProjectPath, e.Issue.IID, e.Issue.Title),
		interpolateGitLabIssuePrompt(e.Prompt, e.Issue), e.RepositoryID, e.BaseBranch,
		map[string]interface{}{
			gitlabIssueWatchMetadataKey:     e.IssueWatchID,
			"gitlab_project_path":           e.Issue.ProjectPath,
			"gitlab_issue_iid":              e.Issue.IID,
			"gitlab_issue_url":              e.Issue.WebURL,
			models.MetaKeyAgentProfileID:    e.AgentProfileID,
			models.MetaKeyExecutorProfileID: e.ExecutorProfileID,
		}), nil
}

func (s *GitLabIssueWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, err := gitlabIssueEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.AssignIssueWatchTaskID(ctx, e.IssueWatchID, e.WatchGeneration, e.Issue.ProjectPath, e.Issue.IID, taskID)
}

func (s *GitLabIssueWatcherSource) IsTerminalAttachError(err error) bool {
	return errors.Is(err, gitlab.ErrWatchOwnershipLost)
}

func (s *GitLabIssueWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, err := gitlabIssueEvent(evt)
	if err != nil {
		return AutoStartParams{}
	}
	return gitlabAutoStartParams(e.AgentProfileID, e.ExecutorProfileID, e.WorkflowStepID)
}

func (s *GitLabIssueWatcherSource) AgentProfileID(evt any) string {
	e, _ := gitlabIssueEvent(evt)
	if e == nil {
		return ""
	}
	return e.AgentProfileID
}

func (s *GitLabIssueWatcherSource) WatchID(evt any) string {
	e, _ := gitlabIssueEvent(evt)
	if e == nil {
		return ""
	}
	return e.IssueWatchID
}

func (s *GitLabIssueWatcherSource) MaxInflightTasks(evt any) *int {
	e, _ := gitlabIssueEvent(evt)
	if e == nil {
		return nil
	}
	return e.MaxInflightTasks
}

func (s *GitLabIssueWatcherSource) WatchMetadataKey() string { return gitlabIssueWatchMetadataKey }

func (s *GitLabIssueWatcherSource) SelfHeal(ctx context.Context, evt any, cause string) error {
	e, err := gitlabIssueEvent(evt)
	if err != nil || s.service == nil {
		return err
	}
	return s.service.DisableIssueWatchWithError(ctx, e.IssueWatchID, cause)
}

func gitlabReviewEvent(evt any) (*gitlab.NewReviewMREvent, error) {
	e, ok := evt.(*gitlab.NewReviewMREvent)
	if !ok || e == nil || e.MR == nil {
		return nil, errors.New("gitlab review source: event payload missing or wrong type")
	}
	return e, nil
}

func gitlabIssueEvent(evt any) (*gitlab.NewIssueEvent, error) {
	e, ok := evt.(*gitlab.NewIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return nil, errors.New("gitlab issue source: event payload missing or wrong type")
	}
	return e, nil
}

func gitlabTaskRequest(workspaceID, workflowID, stepID, title, description, repositoryID, baseBranch string, metadata map[string]interface{}) *IssueTaskRequest {
	req := &IssueTaskRequest{WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: stepID, Title: title, Description: description, Metadata: metadata}
	if repositoryID != "" {
		req.Repositories = []IssueTaskRepository{{RepositoryID: repositoryID, BaseBranch: baseBranch}}
	}
	return req
}

func gitlabAutoStartParams(agentID, executorID, stepID string) AutoStartParams {
	return AutoStartParams{AgentProfileID: agentID, ExecutorProfileID: executorID, WorkflowStepID: stepID}
}

func interpolateGitLabMRPrompt(template string, mr *gitlab.MR) string {
	return strings.NewReplacer(
		"{{mr.url}}", mr.WebURL,
		"{{mr.title}}", mr.Title,
		"{{mr.project}}", mr.ProjectPath,
		"{{mr.iid}}", fmt.Sprint(mr.IID),
		"{{mr.description}}", mr.Body,
	).Replace(template)
}

func interpolateGitLabIssuePrompt(template string, issue *gitlab.Issue) string {
	return strings.NewReplacer(
		"{{issue.url}}", issue.WebURL,
		"{{issue.title}}", issue.Title,
		"{{issue.project}}", issue.ProjectPath,
		"{{issue.iid}}", fmt.Sprint(issue.IID),
		"{{issue.description}}", issue.Body,
	).Replace(template)
}
