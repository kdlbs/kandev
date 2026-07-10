package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/jira"
)

// jiraWatchMetadataKey is the task-metadata key a Jira watcher-created task
// records its originating watch id under. Shared by BuildTaskRequest (writer)
// and WatchMetadataKey (read by the throttle gate's task counter).
const jiraWatchMetadataKey = "jira_issue_watch_id"

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
	req := &IssueTaskRequest{
		WorkspaceID:    e.WorkspaceID,
		WorkflowID:     e.WorkflowID,
		WorkflowStepID: e.WorkflowStepID,
		Title:          fmt.Sprintf("[%s] %s", e.Issue.Key, e.Issue.Summary),
		Description:    interpolateJiraPrompt(e.Prompt, e.Issue),
		// Preserve today's literal metadata keys (do NOT normalise to
		// models.MetaKey* in this refactor — separate cleanup).
		Metadata: map[string]interface{}{
			jiraWatchMetadataKey:  e.IssueWatchID,
			"jira_issue_key":      e.Issue.Key,
			"jira_issue_url":      e.Issue.URL,
			"jira_status":         e.Issue.StatusName,
			"jira_assignee":       e.Issue.AssigneeName,
			"agent_profile_id":    e.AgentProfileID,
			"executor_profile_id": e.ExecutorProfileID,
		},
	}
	// Only a bound watch carries Repositories. An unbound watch leaves the slice
	// nil so the launch path falls to the historical blank-scratch behaviour.
	if e.RepositoryID != "" {
		req.Repositories = []IssueTaskRepository{{
			RepositoryID: e.RepositoryID,
			BaseBranch:   e.BaseBranch,
		}}
	}
	return req, nil
}

func (s *JiraWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return nil
	}
	return s.service.AssignIssueWatchTaskID(ctx, e.IssueWatchID, e.Issue.Key, taskID)
}

func (s *JiraWatcherSource) WatchID(evt any) string {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil {
		return ""
	}
	return e.IssueWatchID
}

func (s *JiraWatcherSource) MaxInflightTasks(evt any) *int {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil {
		return nil
	}
	return e.MaxInflightTasks
}

// WatchMetadataKey returns the task-metadata key this source writes in
// BuildTaskRequest, used by the throttle gate to count open Jira tasks.
func (s *JiraWatcherSource) WatchMetadataKey() string { return jiraWatchMetadataKey }

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

// AgentProfileID returns the watcher's bound profile id, or "" when the
// event payload is malformed. Pre-flight uses "" as the skip-check signal.
func (s *JiraWatcherSource) AgentProfileID(evt any) string {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil {
		return ""
	}
	return e.AgentProfileID
}

// SelfHeal disables the jira_issue_watches row that produced this event.
// Symmetric with LinearWatcherSource.SelfHeal; nil-safe.
func (s *JiraWatcherSource) SelfHeal(ctx context.Context, evt any, cause string) error {
	e, ok := evt.(*jira.NewJiraIssueEvent)
	if !ok || e == nil || s.service == nil {
		return nil
	}
	return s.service.DisableIssueWatchWithError(ctx, e.IssueWatchID, cause)
}
