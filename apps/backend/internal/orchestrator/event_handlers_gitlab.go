package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
)

// GitLabIssueTaskCreator creates a task from a GitLab issue + watch context.
// Mirrors github.IssueTaskCreator. Returns the created task id or an error.
type GitLabIssueTaskCreator interface {
	CreateGitLabIssueTask(ctx context.Context, evt *gitlab.NewIssueEvent) (string, error)
}

// GitLabReviewTaskCreator creates a task from a GitLab MR + watch context.
type GitLabReviewTaskCreator interface {
	CreateGitLabReviewTask(ctx context.Context, evt *gitlab.NewReviewMREvent) (string, error)
}

// SetGitLabIssueTaskCreator wires the issue→task creator used by the
// `gitlab.new_issue` handler. When nil, watches still fire but no tasks
// are auto-created (events are logged for observability).
func (s *Service) SetGitLabIssueTaskCreator(c GitLabIssueTaskCreator) {
	s.gitlabIssueTaskCreator = c
}

// SetGitLabReviewTaskCreator wires the review→task creator.
func (s *Service) SetGitLabReviewTaskCreator(c GitLabReviewTaskCreator) {
	s.gitlabReviewTaskCreator = c
}

// SetGitLabService is the entry point for wiring the GitLab service into the
// orchestrator so the event handlers can reserve dedup rows. Mirrors
// SetGitHubService / SetJiraService.
func (s *Service) SetGitLabService(svc *gitlab.Service) {
	s.gitlabService = svc
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
	if s.gitlabReviewTaskCreator == nil {
		s.logger.Debug("gitlab review task creator not configured; skipping task creation")
		return nil
	}
	go s.createGitLabReviewTask(context.Background(), evt)
	return nil
}

// a different domain type (MR vs Issue); merging them would obscure the
// per-event publishing contract.
//
//nolint:dupl // structurally similar to createGitLabIssueTask but operates on
func (s *Service) createGitLabReviewTask(ctx context.Context, evt *gitlab.NewReviewMREvent) {
	if s.gitlabService == nil {
		s.logger.Warn("gitlab service not configured; cannot reserve dedup")
		return
	}
	mr := evt.MR
	claimed, err := s.gitlabService.ReserveReviewMRTask(ctx, evt.ReviewWatchID, mr.ProjectPath, mr.IID, mr.WebURL)
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("reserve gitlab review MR task", zap.Error(err))
		}
		return
	}
	taskID, err := s.gitlabReviewTaskCreator.CreateGitLabReviewTask(ctx, evt)
	if err != nil {
		s.logger.Warn("create gitlab review task", zap.Error(err))
		if relErr := s.gitlabService.ReleaseReviewMRTask(ctx, evt.ReviewWatchID, mr.ProjectPath, mr.IID); relErr != nil {
			s.logger.Warn("release gitlab review MR dedup row after task-create failure",
				zap.String("review_watch_id", evt.ReviewWatchID),
				zap.String("project", mr.ProjectPath),
				zap.Int("iid", mr.IID),
				zap.Error(relErr))
		}
		return
	}
	if err := s.gitlabService.AssignReviewMRTaskID(ctx, evt.ReviewWatchID, mr.ProjectPath, mr.IID, taskID); err != nil {
		s.logger.Warn("assign gitlab review MR task id", zap.Error(err))
	}
	s.logger.Info("gitlab review MR task created",
		zap.String("task_id", taskID),
		zap.String("project", mr.ProjectPath),
		zap.Int("iid", mr.IID))
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
	if s.gitlabIssueTaskCreator == nil {
		s.logger.Debug("gitlab issue task creator not configured; skipping task creation")
		return nil
	}
	go s.createGitLabIssueTask(context.Background(), evt)
	return nil
}

//nolint:dupl // see createGitLabReviewTask — same shape, different domain types.
func (s *Service) createGitLabIssueTask(ctx context.Context, evt *gitlab.NewIssueEvent) {
	if s.gitlabService == nil {
		s.logger.Warn("gitlab service not configured; cannot reserve dedup")
		return
	}
	issue := evt.Issue
	claimed, err := s.gitlabService.ReserveIssueWatchTask(ctx, evt.IssueWatchID, issue.ProjectPath, issue.IID, issue.WebURL)
	if err != nil || !claimed {
		if err != nil {
			s.logger.Warn("reserve gitlab issue task", zap.Error(err))
		}
		return
	}
	taskID, err := s.gitlabIssueTaskCreator.CreateGitLabIssueTask(ctx, evt)
	if err != nil {
		s.logger.Warn("create gitlab issue task", zap.Error(err))
		if relErr := s.gitlabService.ReleaseIssueWatchTask(ctx, evt.IssueWatchID, issue.ProjectPath, issue.IID); relErr != nil {
			s.logger.Warn("release gitlab issue dedup row after task-create failure",
				zap.String("issue_watch_id", evt.IssueWatchID),
				zap.String("project", issue.ProjectPath),
				zap.Int("iid", issue.IID),
				zap.Error(relErr))
		}
		return
	}
	if err := s.gitlabService.AssignIssueWatchTaskID(ctx, evt.IssueWatchID, issue.ProjectPath, issue.IID, taskID); err != nil {
		s.logger.Warn("assign gitlab issue task id", zap.Error(err))
	}
	s.logger.Info("gitlab issue task created",
		zap.String("task_id", taskID),
		zap.String("project", issue.ProjectPath),
		zap.Int("iid", issue.IID))
}
