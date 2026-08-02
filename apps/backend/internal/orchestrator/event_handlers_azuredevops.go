package orchestrator

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// SetAzureDevOpsService wires Azure's watcher dedup surface into the shared
// watcher coordinator. It is nil-safe so test and partially configured boots
// can still subscribe without creating tasks.
func (s *Service) SetAzureDevOpsService(svc AzureDevOpsWatchService) {
	s.azureDevOpsService = svc
	s.azureWorkItemSource = NewAzureDevOpsWorkItemWatcherSource(svc, s.logger)
	s.azurePullRequestSource = NewAzureDevOpsPullRequestWatcherSource(svc, s.logger)
}

func (s *Service) subscribeAzureDevOpsEvents() {
	if s.eventBus == nil {
		return
	}
	if _, err := s.eventBus.Subscribe(events.AzureDevOpsWorkItemWatchMatch, s.handleAzureDevOpsWorkItemWatchMatch); err != nil {
		s.logger.Error("subscribe azure_devops.work_item_watch.match", zap.Error(err))
	}
	if _, err := s.eventBus.Subscribe(events.AzureDevOpsPullRequestWatchMatch, s.handleAzureDevOpsPullRequestWatchMatch); err != nil {
		s.logger.Error("subscribe azure_devops.pull_request_watch.match", zap.Error(err))
	}
}

func (s *Service) handleAzureDevOpsWorkItemWatchMatch(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*azuredevops.WorkItemWatchEvent)
	if !ok || evt == nil {
		return nil
	}
	src := s.azureWorkItemSource
	if src == nil {
		src = NewAzureDevOpsWorkItemWatcherSource(nil, s.logger)
	}
	s.dispatchWatcherEvent(ctx, src, evt,
		zap.String("work_item_watch_id", evt.WatchID), zap.String("project", evt.ProjectID), zap.Int("work_item_id", evt.WorkItem.ID))
	return nil
}

func (s *Service) handleAzureDevOpsPullRequestWatchMatch(ctx context.Context, event *bus.Event) error {
	evt, ok := event.Data.(*azuredevops.PullRequestWatchEvent)
	if !ok || evt == nil {
		return nil
	}
	src := s.azurePullRequestSource
	if src == nil {
		src = NewAzureDevOpsPullRequestWatcherSource(nil, s.logger)
	}
	s.dispatchWatcherEvent(ctx, src, evt,
		zap.String("pull_request_watch_id", evt.WatchID), zap.String("project", evt.ProjectID), zap.Int("pull_request_id", evt.PullRequest.ID))
	return nil
}
