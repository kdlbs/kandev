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
	// DisableIssueWatchWithError is invoked by the dispatch coordinator's
	// self-heal flow when the watcher's bound agent profile has been
	// soft-deleted.
	DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error
}

// SetLinearService wires the Linear dedup helpers into the orchestrator so
// linear-watch handlers can claim issue slots before creating tasks. Also
// (re)builds the cached LinearWatcherSource so handleNewLinearIssue can
// dispatch without allocating per event.
func (s *Service) SetLinearService(l LinearService) {
	s.linearService = l
	s.linearSource = NewLinearWatcherSource(l, s.logger)
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
// dispatchWatcherEvent call. Integration-specific work
// (reserve, build, attach, release, auto-start params) lives in
// LinearWatcherSource; the pipeline (create, auto-start, error/release
// handling) lives in the coordinator; the shared wiring guards
// (creator/coordinator nil-checks, cancellation detachment) live in
// the dispatchWatcherEvent helper.
func (s *Service) handleNewLinearIssue(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*linear.NewLinearIssueEvent)
	if !ok || evt == nil || evt.Issue == nil {
		return nil
	}
	src := s.linearSource
	if src == nil {
		// SetLinearService was never called; fall back to a fail-open
		// source so behaviour matches the pre-cache code path.
		src = NewLinearWatcherSource(nil, s.logger)
	}
	s.dispatchWatcherEvent(ctx, src, evt,
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("identifier", evt.Issue.Identifier))
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
