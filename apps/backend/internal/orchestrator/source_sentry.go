package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/task/models"
)

// maxSentryTitleRunes bounds the issue-title portion of a generated task
// title. Sentry titles are derived from error messages / culprits and can be
// arbitrarily long (unlike Linear/Jira issue titles, which are short upstream),
// so the producer truncates here rather than deferring to the UI.
const maxSentryTitleRunes = 80

func truncateSentryTitle(s string) string {
	if utf8.RuneCountInString(s) <= maxSentryTitleRunes {
		return s
	}
	return string([]rune(s)[:maxSentryTitleRunes-1]) + "…"
}

// SentryWatcherSource adapts the Sentry integration onto the WatcherSource
// pipeline. Mirrors LinearWatcherSource so a third integration is a clean
// instance of the abstraction.
type SentryWatcherSource struct {
	service SentryService
	logger  *logger.Logger
}

// NewSentryWatcherSource constructs a source bound to the orchestrator's
// Sentry service handle. logger may be nil — methods that log will no-op.
func NewSentryWatcherSource(svc SentryService, log *logger.Logger) *SentryWatcherSource {
	return &SentryWatcherSource{service: svc, logger: log}
}

func (s *SentryWatcherSource) Name() string { return "sentry" }

func (s *SentryWatcherSource) Reserve(ctx context.Context, evt any) (bool, error) {
	e, ok := evt.(*sentry.NewSentryIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return false, errors.New("sentry source: event payload missing or wrong type")
	}
	// Matches Linear: nil service is "fail open" so a boot-order corner case
	// doesn't drop the event.
	if s.service == nil {
		return true, nil
	}
	return s.service.ReserveIssueWatchTask(ctx, e.IssueWatchID, e.Issue.ShortID, e.Issue.Permalink)
}

func (s *SentryWatcherSource) Release(ctx context.Context, evt any) {
	e, ok := evt.(*sentry.NewSentryIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return
	}
	if err := s.service.ReleaseIssueWatchTask(ctx, e.IssueWatchID, e.Issue.ShortID); err != nil && s.logger != nil {
		s.logger.Warn("sentry source: release failed",
			zap.String("short_id", e.Issue.ShortID), zap.Error(err))
	}
}

func (s *SentryWatcherSource) BuildTaskRequest(evt any) (*IssueTaskRequest, error) {
	e, ok := evt.(*sentry.NewSentryIssueEvent)
	if !ok || e == nil || e.Issue == nil {
		return nil, errors.New("sentry source: event payload missing or wrong type")
	}
	return &IssueTaskRequest{
		WorkspaceID:    e.WorkspaceID,
		WorkflowID:     e.WorkflowID,
		WorkflowStepID: e.WorkflowStepID,
		Title:          fmt.Sprintf("[%s] %s — %s", strings.ToUpper(e.Issue.Level), e.Issue.ShortID, truncateSentryTitle(e.Issue.Title)),
		Description:    interpolateSentryPrompt(e.Prompt, e.Issue),
		Metadata: map[string]interface{}{
			"sentry_issue_watch_id":         e.IssueWatchID,
			"sentry_issue_short_id":         e.Issue.ShortID,
			"sentry_issue_url":              e.Issue.Permalink,
			"sentry_issue_level":            e.Issue.Level,
			"sentry_issue_status":           e.Issue.Status,
			"sentry_issue_project":          e.Issue.ProjectSlug,
			models.MetaKeyAgentProfileID:    e.AgentProfileID,
			models.MetaKeyExecutorProfileID: e.ExecutorProfileID,
		},
	}, nil
}

func (s *SentryWatcherSource) AttachTaskID(ctx context.Context, evt any, taskID string) error {
	e, ok := evt.(*sentry.NewSentryIssueEvent)
	if !ok || e == nil || e.Issue == nil || s.service == nil {
		return nil
	}
	return s.service.AssignIssueWatchTaskID(ctx, e.IssueWatchID, e.Issue.ShortID, taskID)
}

func (s *SentryWatcherSource) AutoStartParams(evt any) AutoStartParams {
	e, ok := evt.(*sentry.NewSentryIssueEvent)
	if !ok || e == nil {
		return AutoStartParams{}
	}
	return AutoStartParams{
		AgentProfileID:    e.AgentProfileID,
		ExecutorProfileID: e.ExecutorProfileID,
		WorkflowStepID:    e.WorkflowStepID,
	}
}

func (s *SentryWatcherSource) WatchID(evt any) string {
	e, ok := evt.(*sentry.NewSentryIssueEvent)
	if !ok || e == nil {
		return ""
	}
	return e.IssueWatchID
}

// MaxInflightTasks reports the per-watch cap on open watcher-created tasks.
// Sentry issue-watch events carry no per-watch override, so this always
// returns nil and the coordinator applies its default cap (mirrors the
// Linear/Jira sources' nil path).
func (s *SentryWatcherSource) MaxInflightTasks(evt any) *int {
	return nil
}
