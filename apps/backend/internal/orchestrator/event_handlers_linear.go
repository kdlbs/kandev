package orchestrator

import (
	"context"
	"strings"

	"go.uber.org/zap"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/linear"
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

// handleNewLinearIssue translates a LinearNewIssue bus event into a
// WatcherDispatchCoordinator.Dispatch call. The integration-specific work
// (reserve, build, attach, release, auto-start params) lives in
// LinearWatcherSource. The pipeline (create, auto-start, error/release
// handling) lives in the coordinator.
func (s *Service) handleNewLinearIssue(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*linear.NewLinearIssueEvent)
	if !ok || evt == nil || evt.Issue == nil {
		return nil
	}
	s.logger.Info("new linear issue detected from watch",
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("identifier", evt.Issue.Identifier))

	if s.issueTaskCreator == nil {
		s.logger.Warn("issue task creator not configured, skipping linear task creation")
		return nil
	}
	if s.watcherCoordinator == nil {
		// Defensive: coordinator is wired by SetIssueTaskCreator. If we got
		// here without the creator we already returned above; this is just
		// a belt-and-braces guard for tests that wire pieces individually.
		s.logger.Warn("watcher coordinator not configured, skipping linear task dispatch",
			zap.String("issue_watch_id", evt.IssueWatchID),
			zap.String("identifier", evt.Issue.Identifier))
		return nil
	}

	src := NewLinearWatcherSource(s.linearService, s.logger)
	// Detach from cancellation but keep request-scoped values (tracing, etc.):
	// the bus delivery context may be cancelled before task creation finishes,
	// but in-memory/non-NATS bus implementations may carry values worth
	// propagating.
	go s.watcherCoordinator.Dispatch(context.WithoutCancel(ctx), src, evt)
	return nil
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
