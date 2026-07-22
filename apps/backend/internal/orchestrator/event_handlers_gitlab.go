package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// SetGitLabService is the entry point for wiring the GitLab service into the
// orchestrator so the event handlers can reserve dedup rows. Mirrors
// SetGitHubService / SetJiraService.
func (s *Service) SetGitLabService(svc GitLabWatchService) {
	s.gitlabService = svc
	s.gitlabReviewSource = NewGitLabReviewWatcherSource(svc, s.logger)
	s.gitlabIssueSource = NewGitLabIssueWatcherSource(svc, s.logger)
}

// SetGitLabCredentialResolver binds execution auth to the task workspace.
func (s *Service) SetGitLabCredentialResolver(resolver executor.GitLabCredentialResolver) {
	s.executor.SetGitLabCredentialResolver(resolver)
}

// subscribeGitLabEvents wires bus subscriptions for the GitLab integration
// events. Idempotent — safe to call once per orchestrator boot.
func (s *Service) subscribeGitLabEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.GitLabNewReviewMR, s.handleGitLabNewReviewMR); err != nil {
		s.logger.Error("subscribe gitlab.new_mr_to_review", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.GitLabNewIssue, s.handleGitLabNewIssue); err != nil {
		s.logger.Error("subscribe gitlab.new_issue", zap.Error(err))
	}
}

// handleGitLabNewReviewMR turns a new-MR-to-review event into a Kandev task.
// When no review-task creator is configured the event is logged and dropped
// (matches the GitHub flow when a workspace has no task creator wired).
func (s *Service) handleGitLabNewReviewMR(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*gitlab.NewReviewMREvent)
	if !ok || evt == nil || evt.MR == nil {
		return nil
	}
	s.logger.Info("new gitlab MR detected from review watch",
		zap.String("review_watch_id", evt.ReviewWatchID),
		zap.String("project", evt.MR.ProjectPath),
		zap.Int("iid", evt.MR.IID))
	src := s.gitlabReviewSource
	if src == nil {
		src = NewGitLabReviewWatcherSource(nil, s.logger)
	}
	s.dispatchWatcherEvent(ctx, src, evt,
		zap.String("review_watch_id", evt.ReviewWatchID),
		zap.String("project", evt.MR.ProjectPath),
		zap.Int("iid", evt.MR.IID))
	return nil
}

// handleGitLabNewIssue mirrors handleGitLabNewReviewMR for issue events.
func (s *Service) handleGitLabNewIssue(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*gitlab.NewIssueEvent)
	if !ok || evt == nil || evt.Issue == nil {
		return nil
	}
	s.logger.Info("new gitlab issue detected from issue watch",
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("project", evt.Issue.ProjectPath),
		zap.Int("iid", evt.Issue.IID))
	src := s.gitlabIssueSource
	if src == nil {
		src = NewGitLabIssueWatcherSource(nil, s.logger)
	}
	s.dispatchWatcherEvent(ctx, src, evt,
		zap.String("issue_watch_id", evt.IssueWatchID),
		zap.String("project", evt.Issue.ProjectPath),
		zap.Int("iid", evt.Issue.IID))
	return nil
}
