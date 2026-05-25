package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/jira"
)

// JiraWatcherSource adapts the Jira integration onto the WatcherSource
// pipeline. Symmetric with LinearWatcherSource; the only differences are
// the metadata keys and the interpolation function.
type JiraWatcherSource struct {
	service JiraService
	logger  *logger.Logger
}

// NewJiraWatcherSource constructs a source bound to the orchestrator's
// Jira service handle.
func NewJiraWatcherSource(svc JiraService, log *logger.Logger) *JiraWatcherSource {
	return &JiraWatcherSource{service: svc, logger: log}
}

func (s *JiraWatcherSource) Name() string { return "jira" }

func (s *JiraWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return false, errors.New("jira source: event payload missing or wrong type")
	}
	if s.service == nil {
		return true, nil
	}
	return s.service.ReserveIssueWatchTask(ctx, e.IssueWatchID, e.Issue.Key, e.Issue.URL)
}

func (s *JiraWatcherSource) Release(ctx context.Context, evt any) {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return
	}
	if err := s.service.ReleaseIssueWatchTask(ctx, e.IssueWatchID, e.Issue.Key); err != nil && s.logger != nil {
		s.logger.Warn("jira source: release failed",
			zap.String("issue_key", e.Issue.Key), zap.Error(err))
	}
}

func (s *JiraWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return nil, errors.New("jira source: event payload missing or wrong type")
	}
	return &IssueTaskRequest{
		WorkspaceID:    e.WorkspaceID,
		WorkflowID:     e.WorkflowID,
		WorkflowStepID: e.WorkflowStepID,
		Title:          fmt.Sprintf("[%s] %s", e.Issue.Key, e.Issue.Summary),
		Description:    interpolateJiraPrompt(e.Prompt, e.Issue),
		// Preserve today's literal metadata keys (do NOT normalise to
		// models.MetaKey* in this refactor — separate cleanup).
		Metadata: map[string]interface{}{
			"jira_issue_watch_id": e.IssueWatchID,
			"jira_issue_key":      e.Issue.Key,
			"jira_issue_url":      e.Issue.URL,
			"jira_status":         e.Issue.StatusName,
			"jira_assignee":       e.Issue.AssigneeName,
			"agent_profile_id":    e.AgentProfileID,
			"executor_profile_id": e.ExecutorProfileID,
		},
	}, nil
}

func (s *JiraWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return nil
	}
	return s.service.AssignIssueWatchTaskID(ctx, e.IssueWatchID, e.Issue.Key, taskID)
}

func (s *JiraWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil {
		return AutoStartParams{}
	}
	return AutoStartParams{
		AgentProfileID:    e.AgentProfileID,
		ExecutorProfileID: e.ExecutorProfileID,
		WorkflowStepID:    e.WorkflowStepID,
	}
}
