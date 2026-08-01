package orchestrator

import (
	"context"
	"fmt"

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

// SetGitLabMRLinkService wires the auto-link surface into the orchestrator so
// push detection and the on-demand check can find and associate merge
// requests opened outside Kandev's own Create-PR action. Mirrors
// SetGitHubService.
func (s *Service) SetGitLabMRLinkService(svc GitLabMRLinkService) {
	s.gitlabMRLinkService = svc
}

// SetGitLabCredentialResolver binds execution auth to the task workspace.
func (s *Service) SetGitLabCredentialResolver(resolver executor.GitLabCredentialResolver) {
	s.executor.SetGitLabCredentialResolver(resolver)
}

// SetGitLabMRAutomationService wires the GitLab MR lifecycle notification
// surface into the orchestrator. Mirrors SetGitHubService's role for the
// narrower taskPRAgentAutomationService interface — GitLab has no
// auto-fix/auto-merge automation, so there is no larger GitLabService
// interface for this to be carved out of.
func (s *Service) SetGitLabMRAutomationService(svc taskMRAgentAutomationService) {
	s.gitlabMRAutomation = svc
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
	if _, err := s.eventBus.Subscribe(events.GitLabTaskMRUpdated, s.handleGitLabTaskMRUpdated); err != nil {
		s.logger.Error("subscribe gitlab.task_mr.updated", zap.Error(err))
	}
}

// handleGitLabTaskMRUpdated reacts to every observed MR change — from the
// poller's lifecycle sync pass or a manual link/refresh — by starting a
// single-flight lifecycle evaluation. event.Data is *gitlab.TaskMRUpdatedEvent
// (published by both producers); a differently-shaped payload is ignored.
func (s *Service) handleGitLabTaskMRUpdated(ctx context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*gitlab.TaskMRUpdatedEvent)
	if !ok || payload == nil || payload.TaskMR == nil {
		return nil
	}
	s.startTaskMRLifecycleAutomation(ctx, payload.TaskMR)
	return nil
}

// startTaskMRLifecycleAutomation runs the lifecycle evaluation pass in a
// detached, single-flight goroutine keyed by (task, repository, iid) —
// mirrors startTaskPRCIAutomationWithRefresh (AC23).
func (s *Service) startTaskMRLifecycleAutomation(ctx context.Context, mr *gitlab.TaskMR) {
	if mr == nil {
		return
	}
	key := fmt.Sprintf("%s|%s|%d", mr.TaskID, mr.RepositoryID, mr.MRIID)
	if _, loaded := s.mrAutomationInFlight.LoadOrStore(key, struct{}{}); loaded {
		s.logger.Debug("MR lifecycle automation already in flight",
			zap.String("task_id", mr.TaskID),
			zap.String("repository_id", mr.RepositoryID),
			zap.Int("mr_iid", mr.MRIID))
		return
	}
	go func() {
		defer s.mrAutomationInFlight.Delete(key)
		automationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ciAutomationDetachedTimeout)
		defer cancel()
		if err := s.handleTaskMRLifecycleAutomation(automationCtx, mr); err != nil {
			s.logger.Debug("MR lifecycle automation handling failed", zap.String("task_id", mr.TaskID), zap.Error(err))
		}
	}()
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
