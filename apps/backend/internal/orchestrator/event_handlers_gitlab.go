package orchestrator

import (
	"context"
	"encoding/json"
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
	if _, err := s.eventBus.Subscribe(events.GitLabTaskMROptionsUpdated, s.handleGitLabTaskMROptionsUpdated); err != nil {
		s.logger.Error("subscribe gitlab.task_mr_options.updated", zap.Error(err))
	}
}

// handleGitLabTaskMRUpdated reacts to every observed MR change — from the
// poller's lifecycle sync pass or a manual link/refresh — by starting a
// single-flight lifecycle evaluation. event.Data is *gitlab.TaskMRUpdatedEvent
// on the in-memory bus, but a map[string]interface{} on the NATS bus (Publish
// JSON-marshals the event; Subscribe unmarshals into the untyped Data field,
// which loses the concrete type) — decodeTaskMRUpdatedEvent normalizes both.
func (s *Service) handleGitLabTaskMRUpdated(ctx context.Context, event *bus.Event) error {
	payload, ok := decodeTaskMRUpdatedEvent(event.Data)
	if !ok || payload == nil || payload.TaskMR == nil {
		return nil
	}
	s.startTaskMRLifecycleAutomationWithObservation(
		ctx, payload.TaskMR, taskMRReviewerObservationFromEvent(payload),
	)
	return nil
}

func taskMRReviewerObservationFromEvent(event *gitlab.TaskMRUpdatedEvent) *taskMRReviewerObservation {
	if event == nil || !event.ReviewersValid {
		return nil
	}
	return &taskMRReviewerObservation{reviewers: append([]gitlab.MRReviewer(nil), event.Reviewers...)}
}

// decodeTaskMRUpdatedEvent accepts either the in-process pointer shape or the
// NATS-decoded map shape and normalizes both to *gitlab.TaskMRUpdatedEvent.
func decodeTaskMRUpdatedEvent(data interface{}) (*gitlab.TaskMRUpdatedEvent, bool) {
	switch v := data.(type) {
	case *gitlab.TaskMRUpdatedEvent:
		return v, true
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var payload gitlab.TaskMRUpdatedEvent
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, false
		}
		return &payload, true
	default:
		return nil, false
	}
}

// handleGitLabTaskMROptionsUpdated reacts to a task's MR lifecycle switches
// changing — via the HTTP PATCH endpoint, an MCP tool call, or the
// orchestrator's own error/recovery state publish — by re-evaluating every
// MR currently linked to that task. Without this, enabling a switch only
// took effect on the next poller sweep (up to a minute later) instead of
// immediately, and MCP or multi-tab changes never reached an already-open
// UI. A no-op evaluation pass doesn't publish state (see
// handleTaskMRLifecycleAutomation), so this cannot cascade indefinitely —
// only a genuine delivery or error re-triggers another round.
func (s *Service) handleGitLabTaskMROptionsUpdated(ctx context.Context, event *bus.Event) error {
	resp, ok := decodeTaskMROptionsUpdatedEvent(event.Data)
	if !ok || resp == nil || resp.TaskID == "" || s.gitlabMRAutomation == nil {
		return nil
	}
	mrs, err := s.gitlabMRAutomation.ListTaskMRsByTask(ctx, resp.TaskID)
	if err != nil {
		s.logger.Debug("list task MRs for options-updated evaluation failed",
			zap.String("task_id", resp.TaskID), zap.Error(err))
		return nil
	}
	for _, mr := range mrs {
		s.startTaskMRLifecycleAutomation(ctx, mr)
	}
	return nil
}

// decodeTaskMROptionsUpdatedEvent mirrors decodeTaskMRUpdatedEvent: the
// in-process bus carries the original *gitlab.TaskMRAutomationResponse
// pointer, NATS round-trips it through JSON into a map.
func decodeTaskMROptionsUpdatedEvent(data interface{}) (*gitlab.TaskMRAutomationResponse, bool) {
	switch v := data.(type) {
	case *gitlab.TaskMRAutomationResponse:
		return v, true
	case map[string]interface{}:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var payload gitlab.TaskMRAutomationResponse
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, false
		}
		return &payload, true
	default:
		return nil, false
	}
}

// mrAutomationInFlightKey identifies a linked MR for single-flight dedup.
// RepositoryID alone doesn't distinguish MRs when it's empty (self-managed
// GitLab hosts without a numeric project ID), so ProjectPath is included —
// two projects sharing an empty RepositoryID and the same IID would otherwise
// collide and suppress each other's lifecycle evaluation indefinitely.
func mrAutomationInFlightKey(mr *gitlab.TaskMR) string {
	return fmt.Sprintf("%s|%s|%s|%d", mr.TaskID, mr.RepositoryID, mr.ProjectPath, mr.MRIID)
}

// startTaskMRLifecycleAutomation runs the lifecycle evaluation pass in a
// detached, single-flight goroutine keyed by (task, repository, project,
// iid) — mirrors startTaskPRCIAutomationWithRefresh (AC23).
func (s *Service) startTaskMRLifecycleAutomation(ctx context.Context, mr *gitlab.TaskMR) {
	s.startTaskMRLifecycleAutomationWithObservation(ctx, mr, nil)
}

func (s *Service) startTaskMRLifecycleAutomationWithObservation(
	ctx context.Context, mr *gitlab.TaskMR, observation *taskMRReviewerObservation,
) {
	if mr == nil {
		return
	}
	key := mrAutomationInFlightKey(mr)
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
		if err := s.handleTaskMRLifecycleAutomationWithObservation(automationCtx, mr, observation); err != nil {
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
