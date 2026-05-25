package orchestrator

import (
	"context"
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

// handleNewJiraIssue translates a JiraNewIssue bus event into a
// dispatchWatcherEvent call. Symmetric with handleNewLinearIssue.
func (s *Service) handleNewJiraIssue(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*jira.NewJiraIssueEvent)
	if !ok || evt == nil || evt.Issue == nil {
		return nil
	}
	s.dispatchWatcherEvent(ctx, "jira",
		NewJiraWatcherSource(s.jiraService, s.logger), evt,
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("issue_key", evt.Issue.Key))
	return nil
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
