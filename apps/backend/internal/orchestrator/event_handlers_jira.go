package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/jira"
)

// JiraService is the subset of jira.Service the orchestrator needs to
// deduplicate ticket→task mappings. Mirrors the GitHub equivalent so the
// orchestrator stays decoupled from the concrete jira package types.
type JiraService interface {
	ReserveIssueWatchTask(ctx context.Context, watchID, issueKey, issueURL string) (bool, error)
	AssignIssueWatchTaskID(ctx context.Context, watchID, issueKey, taskID string) error
	ReleaseIssueWatchTask(ctx context.Context, watchID, issueKey string) error
}

// SetJiraService wires the JIRA dedup helpers into the orchestrator so
// jira-watch handlers can claim ticket slots before creating tasks.
func (s *Service) SetJiraService(j JiraService) {
	s.jiraService = j
}

// subscribeJiraEvents wires the JIRA event handlers onto the bus. Called from
// the existing subscribeGitHubEvents-style boot path so all integration
// subscriptions stay grouped together.
func (s *Service) subscribeJiraEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.JiraNewIssue, s.handleNewJiraIssue); err != nil {
		s.logger.Error("failed to subscribe to jira.new_issue events", zap.Error(err))
	}
}

// handleNewJiraIssue creates a Kandev task for a freshly-observed JIRA ticket.
// Reuses the same IssueTaskCreator the GitHub flow uses so task creation logic
// (workflow placement, on-enter actions, etc.) stays in one place.
func (s *Service) handleNewJiraIssue(_ context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*jira.NewJiraIssueEvent)
	if !ok {
		return nil
	}
	if evt.Issue == nil {
		return nil
	}
	s.logger.Info("new jira issue detected from watch",
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("issue_key", evt.Issue.Key))

	if s.issueTaskCreator == nil {
		s.logger.Warn("issue task creator not configured, skipping jira task creation")
		return nil
	}

	// Background context: the bus delivery context may be cancelled before
	// task creation finishes. The work is independent of the publisher.
	go s.createJiraIssueTask(context.Background(), evt)
	return nil
}

func (s *Service) createJiraIssueTask(ctx context.Context, evt *jira.NewJiraIssueEvent) {
	ticket := evt.Issue
	if !s.reserveJiraIssue(ctx, evt) {
		return
	}

	req := &IssueTaskRequest{
		WorkspaceID:    evt.WorkspaceID,
		WorkflowID:     evt.WorkflowID,
		WorkflowStepID: evt.WorkflowStepID,
		Title:          fmt.Sprintf("[%s] %s", ticket.Key, ticket.Summary),
		Description:    interpolateJiraPrompt(evt.Prompt, ticket),
		Metadata: map[string]interface{}{
			"jira_issue_watch_id": evt.IssueWatchID,
			"jira_issue_key":      ticket.Key,
			"jira_issue_url":      ticket.URL,
			"jira_status":         ticket.StatusName,
			"jira_assignee":       ticket.AssigneeName,
			"agent_profile_id":    evt.AgentProfileID,
			"executor_profile_id": evt.ExecutorProfileID,
		},
	}

	task, err := s.issueTaskCreator.CreateIssueTask(ctx, req)
	if err != nil {
		s.logger.Error("failed to create jira issue task",
			zap.String("issue_watch_id", evt.IssueWatchID),
			zap.String("issue_key", ticket.Key),
			zap.Error(err))
		s.releaseJiraIssue(ctx, evt)
		return
	}

	s.attachJiraIssueTaskID(ctx, evt, task.ID)
	s.logger.Info("created jira issue task",
		zap.String("task_id", task.ID),
		zap.String("issue_key", ticket.Key))

	if !s.shouldAutoStartStep(ctx, evt.WorkflowStepID) {
		return
	}
	if _, err := s.StartTask(
		ctx, task.ID, evt.AgentProfileID, "", evt.ExecutorProfileID,
		0, task.Description, evt.WorkflowStepID, false, nil,
	); err != nil {
		s.logger.Error("failed to auto-start jira issue task",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	s.logger.Info("auto-started jira issue task",
		zap.String("task_id", task.ID),
		zap.String("issue_key", ticket.Key))
}

// reserveJiraIssue claims the dedup slot before the (relatively expensive)
// task creation. Returns false if another handler beat us to it. When the
// jira service isn't wired (boot order corner case), proceed anyway — better
// to risk a duplicate task than silently drop the event.
func (s *Service) reserveJiraIssue(ctx context.Context, evt *jira.NewJiraIssueEvent) bool {
	if s.jiraService == nil {
		return true
	}
	reserved, err := s.jiraService.ReserveIssueWatchTask(ctx, evt.IssueWatchID, evt.Issue.Key, evt.Issue.URL)
	if err != nil {
		s.logger.Error("failed to reserve jira issue slot",
			zap.String("issue_key", evt.Issue.Key), zap.Error(err))
		return false
	}
	if !reserved {
		s.logger.Debug("jira issue already reserved by concurrent handler, skipping",
			zap.String("issue_key", evt.Issue.Key))
	}
	return reserved
}

func (s *Service) releaseJiraIssue(ctx context.Context, evt *jira.NewJiraIssueEvent) {
	if s.jiraService == nil {
		return
	}
	if err := s.jiraService.ReleaseIssueWatchTask(ctx, evt.IssueWatchID, evt.Issue.Key); err != nil {
		s.logger.Warn("failed to release jira issue reservation after task-create failure",
			zap.String("issue_key", evt.Issue.Key), zap.Error(err))
	}
}

func (s *Service) attachJiraIssueTaskID(ctx context.Context, evt *jira.NewJiraIssueEvent, taskID string) {
	if s.jiraService == nil {
		return
	}
	if err := s.jiraService.AssignIssueWatchTaskID(ctx, evt.IssueWatchID, evt.Issue.Key, taskID); err != nil {
		s.logger.Error("failed to assign task ID to jira issue reservation",
			zap.String("task_id", taskID),
			zap.String("issue_key", evt.Issue.Key), zap.Error(err))
	}
}

// interpolateJiraPrompt replaces {{issue.*}} placeholders with ticket fields.
// When the template is empty, falls back to the embedded default in
// config/prompts/jira-issue-watch-default.md — same pattern the GitHub
// issue/PR watchers use so the default lives in one editable place.
func interpolateJiraPrompt(template string, t *jira.JiraTicket) string {
	if strings.TrimSpace(template) == "" {
		template = promptcfg.Get("jira-issue-watch-default")
	}
	r := strings.NewReplacer(
		"{{issue.key}}", t.Key,
		"{{issue.summary}}", t.Summary,
		"{{issue.url}}", t.URL,
		"{{issue.status}}", t.StatusName,
		"{{issue.priority}}", t.Priority,
		"{{issue.type}}", t.IssueType,
		"{{issue.assignee}}", t.AssigneeName,
		"{{issue.reporter}}", t.ReporterName,
		"{{issue.project}}", t.ProjectKey,
		"{{issue.description}}", t.Description,
	)
	return r.Replace(template)
}
