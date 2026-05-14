package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/task/models"
)

// LinearService is the subset of linear.Service the orchestrator needs to
// deduplicate issue→task mappings. Mirrors the JIRA equivalent.
type LinearService interface {
	ReserveIssueWatchTask(ctx context.Context, watchID, identifier, issueURL string) (bool, error)
	AssignIssueWatchTaskID(ctx context.Context, watchID, identifier, taskID string) error
	ReleaseIssueWatchTask(ctx context.Context, watchID, identifier string) error
}

// SetLinearService wires the Linear dedup helpers into the orchestrator so
// linear-watch handlers can claim issue slots before creating tasks.
func (s *Service) SetLinearService(l LinearService) {
	s.linearService = l
}

// subscribeLinearEvents wires the Linear event handlers onto the bus.
func (s *Service) subscribeLinearEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.LinearNewIssue, s.handleNewLinearIssue); err != nil {
		s.logger.Error("failed to subscribe to linear.new_issue events", zap.Error(err))
	}
}

// handleNewLinearIssue creates a Kandev task for a freshly-observed Linear issue.
func (s *Service) handleNewLinearIssue(_ context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*linear.NewLinearIssueEvent)
	if !ok {
		return nil
	}
	if evt.Issue == nil {
		return nil
	}
	s.logger.Info("new linear issue detected from watch",
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("identifier", evt.Issue.Identifier))

	if s.issueTaskCreator == nil {
		s.logger.Warn("issue task creator not configured, skipping linear task creation")
		return nil
	}

	// Background context: the bus delivery context may be cancelled before
	// task creation finishes. The work is independent of the publisher.
	go s.createLinearIssueTask(context.Background(), evt)
	return nil
}

// createLinearIssueTask mirrors createJiraIssueTask intentionally: the dedup-
// reserve → create → assign-or-release lifecycle is the same shape across
// both integrations. Refactoring out the symmetry would couple two
// integration packages we'd rather keep independent (their event payloads,
// metadata keys, and prompt placeholders all differ).
//
//nolint:dupl // intentional symmetry with createJiraIssueTask; see comment above
func (s *Service) createLinearIssueTask(ctx context.Context, evt *linear.NewLinearIssueEvent) {
	issue := evt.Issue
	if !s.reserveLinearIssue(ctx, evt) {
		return
	}

	req := &IssueTaskRequest{
		WorkspaceID:    evt.WorkspaceID,
		WorkflowID:     evt.WorkflowID,
		WorkflowStepID: evt.WorkflowStepID,
		Title:          fmt.Sprintf("[%s] %s", issue.Identifier, issue.Title),
		Description:    interpolateLinearPrompt(evt.Prompt, issue),
		Metadata: map[string]interface{}{
			"linear_issue_watch_id":         evt.IssueWatchID,
			"linear_issue_identifier":       issue.Identifier,
			"linear_issue_url":              issue.URL,
			"linear_state":                  issue.StateName,
			"linear_assignee":               issue.AssigneeName,
			models.MetaKeyAgentProfileID:    evt.AgentProfileID,
			models.MetaKeyExecutorProfileID: evt.ExecutorProfileID,
		},
	}

	task, err := s.issueTaskCreator.CreateIssueTask(ctx, req)
	if err != nil {
		s.logger.Error("failed to create linear issue task",
			zap.String("issue_watch_id", evt.IssueWatchID),
			zap.String("identifier", issue.Identifier),
			zap.Error(err))
		s.releaseLinearIssue(ctx, evt)
		return
	}

	s.attachLinearIssueTaskID(ctx, evt, task.ID)
	s.logger.Info("created linear issue task",
		zap.String("task_id", task.ID),
		zap.String("identifier", issue.Identifier))

	if !s.shouldAutoStartStep(ctx, evt.WorkflowStepID) {
		return
	}
	if _, err := s.StartTask(
		ctx, task.ID, evt.AgentProfileID, "", evt.ExecutorProfileID,
		0, task.Description, evt.WorkflowStepID, false, nil,
	); err != nil {
		s.logger.Error("failed to auto-start linear issue task",
			zap.String("task_id", task.ID), zap.Error(err))
		return
	}
	s.logger.Info("auto-started linear issue task",
		zap.String("task_id", task.ID),
		zap.String("identifier", issue.Identifier))
}

// reserveLinearIssue claims the dedup slot before task creation. When the
// linear service isn't wired (boot order corner case), proceed anyway —
// better to risk a duplicate task than silently drop the event.
func (s *Service) reserveLinearIssue(ctx context.Context, evt *linear.NewLinearIssueEvent) bool {
	if s.linearService == nil {
		return true
	}
	reserved, err := s.linearService.ReserveIssueWatchTask(ctx, evt.IssueWatchID, evt.Issue.Identifier, evt.Issue.URL)
	if err != nil {
		s.logger.Error("failed to reserve linear issue slot",
			zap.String("identifier", evt.Issue.Identifier), zap.Error(err))
		return false
	}
	if !reserved {
		s.logger.Debug("linear issue already reserved by concurrent handler, skipping",
			zap.String("identifier", evt.Issue.Identifier))
	}
	return reserved
}

func (s *Service) releaseLinearIssue(ctx context.Context, evt *linear.NewLinearIssueEvent) {
	if s.linearService == nil {
		return
	}
	if err := s.linearService.ReleaseIssueWatchTask(ctx, evt.IssueWatchID, evt.Issue.Identifier); err != nil {
		s.logger.Warn("failed to release linear issue reservation after task-create failure",
			zap.String("identifier", evt.Issue.Identifier), zap.Error(err))
	}
}

func (s *Service) attachLinearIssueTaskID(ctx context.Context, evt *linear.NewLinearIssueEvent, taskID string) {
	if s.linearService == nil {
		return
	}
	if err := s.linearService.AssignIssueWatchTaskID(ctx, evt.IssueWatchID, evt.Issue.Identifier, taskID); err != nil {
		s.logger.Error("failed to assign task ID to linear issue reservation",
			zap.String("task_id", taskID),
			zap.String("identifier", evt.Issue.Identifier), zap.Error(err))
	}
}

// interpolateLinearPrompt replaces {{issue.*}} placeholders with issue fields.
// When the template is empty (user didn't configure a custom prompt), it falls
// back to the embedded default at config/prompts/linear-issue-watch-default.md
// — same pattern as the github + jira watchers, so prompt content is editable
// without redeploying.
func interpolateLinearPrompt(template string, i *linear.LinearIssue) string {
	if strings.TrimSpace(template) == "" {
		template = promptcfg.Get("linear-issue-watch-default")
	}
	r := strings.NewReplacer(
		"{{issue.identifier}}", i.Identifier,
		"{{issue.title}}", i.Title,
		"{{issue.url}}", i.URL,
		"{{issue.state}}", i.StateName,
		"{{issue.priority}}", i.PriorityLabel,
		"{{issue.team}}", i.TeamKey,
		"{{issue.assignee}}", i.AssigneeName,
		"{{issue.creator}}", i.CreatorName,
		"{{issue.description}}", i.Description,
	)
	return r.Replace(template)
}
