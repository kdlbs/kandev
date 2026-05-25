//nolint:dupl // Mirrors event_handlers_linear.go / event_handlers_jira.go by design — each watcher source has a small 3-line forwarder onto the shared WatcherDispatchCoordinator. See ADR on WatcherSource pattern.
package orchestrator

import (
	"context"
	"strconv"
	"strings"

	"go.uber.org/zap"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/sentry"
)

// SentryService is the subset of sentry.Service the orchestrator needs to
// deduplicate issue→task mappings. Mirrors the Linear / Jira equivalents.
type SentryService interface {
	ReserveIssueWatchTask(ctx context.Context, watchID, shortID, issueURL string) (bool, error)
	AssignIssueWatchTaskID(ctx context.Context, watchID, shortID, taskID string) error
	ReleaseIssueWatchTask(ctx context.Context, watchID, shortID string) error
}

// SetSentryService wires the Sentry dedup helpers into the orchestrator so
// sentry-watch handlers can claim issue slots before creating tasks. Also
// (re)builds the cached SentryWatcherSource so handleNewSentryIssue can
// dispatch without allocating per event.
func (s *Service) SetSentryService(sv SentryService) {
	s.sentryService = sv
	s.sentrySource = NewSentryWatcherSource(sv, s.logger)
}

// subscribeSentryEvents wires the Sentry event handlers onto the bus.
func (s *Service) subscribeSentryEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.SentryNewIssue, s.handleNewSentryIssue); err != nil {
		s.logger.Error("failed to subscribe to sentry.new_issue events", zap.Error(err))
	}
}

// handleNewSentryIssue translates a SentryNewIssue bus event into a
// dispatchWatcherEvent call. Integration-specific work (reserve, build,
// attach, release, auto-start params) lives in SentryWatcherSource; the
// pipeline (create, auto-start, error/release handling) lives in the
// coordinator; the shared wiring guards (creator/coordinator nil-checks,
// cancellation detachment) live in the dispatchWatcherEvent helper.
func (s *Service) handleNewSentryIssue(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*sentry.NewSentryIssueEvent)
	if !ok || evt == nil || evt.Issue == nil {
		return nil
	}
	src := s.sentrySource
	if src == nil {
		// SetSentryService was never called; fall back to a fail-open
		// source so behaviour matches the pre-cache code path.
		src = NewSentryWatcherSource(nil, s.logger)
	}
	s.dispatchWatcherEvent(ctx, "sentry", src, evt,
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("short_id", evt.Issue.ShortID))
	return nil
}

// interpolateSentryPrompt replaces {{issue.*}} placeholders with issue fields.
// When the template is empty (user didn't configure a custom prompt), it falls
// back to the embedded default at config/prompts/sentry-issue-watch-default.md
// — same pattern as the linear / jira watchers, so prompt content is editable
// without redeploying.
func interpolateSentryPrompt(template string, i *sentry.SentryIssue) string {
	if strings.TrimSpace(template) == "" {
		template = promptcfg.Get("sentry-issue-watch-default")
	}
	project := i.ProjectSlug
	if i.ProjectName != "" {
		project = i.ProjectName
	}
	r := strings.NewReplacer(
		"{{issue.short_id}}", i.ShortID,
		"{{issue.title}}", i.Title,
		"{{issue.url}}", i.Permalink,
		"{{issue.project}}", project,
		"{{issue.level}}", i.Level,
		"{{issue.status}}", i.Status,
		"{{issue.culprit}}", i.Culprit,
		"{{issue.assignee}}", i.AssigneeName,
		"{{issue.count}}", i.Count,
		"{{issue.user_count}}", strconv.Itoa(i.UserCount),
		"{{issue.first_seen}}", i.FirstSeen,
		"{{issue.last_seen}}", i.LastSeen,
	)
	return r.Replace(template)
}
