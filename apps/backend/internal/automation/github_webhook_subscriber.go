package automation

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

// GitHubWebhookSubscriber turns webhook-driven GitHub bus events (push,
// completed check runs) into fired github_push / github_ci automation
// triggers. It owns its subscriptions: Start registers handlers on the bus,
// Stop unsubscribes. The bus delivers to handlers synchronously on the
// publisher's goroutine, so this type spawns no goroutines of its own.
type GitHubWebhookSubscriber struct {
	svc      *Service
	eventBus bus.EventBus
	logger   *logger.Logger

	subs    []bus.Subscription
	started bool
}

// NewGitHubWebhookSubscriber creates a new subscriber.
func NewGitHubWebhookSubscriber(svc *Service, eventBus bus.EventBus, log *logger.Logger) *GitHubWebhookSubscriber {
	return &GitHubWebhookSubscriber{svc: svc, eventBus: eventBus, logger: log}
}

// Start subscribes to the GitHub webhook bus events. Idempotent.
func (s *GitHubWebhookSubscriber) Start(_ context.Context) {
	if s.started || s.eventBus == nil {
		return
	}
	s.started = true

	pushSub, err := s.eventBus.Subscribe(events.GitHubPushReceived, s.handlePush)
	if err != nil {
		s.logger.Error("failed to subscribe to github.push_received", zap.Error(err))
	} else {
		s.subs = append(s.subs, pushSub)
	}

	ciSub, err := s.eventBus.Subscribe(events.GitHubCheckRunCompleted, s.handleCheckRun)
	if err != nil {
		s.logger.Error("failed to subscribe to github.check_run_completed", zap.Error(err))
	} else {
		s.subs = append(s.subs, ciSub)
	}

	s.logger.Info("automation GitHub webhook subscriber started")
}

// Stop unsubscribes from the bus. Idempotent.
func (s *GitHubWebhookSubscriber) Stop() {
	if !s.started {
		return
	}
	for _, sub := range s.subs {
		if sub != nil {
			_ = sub.Unsubscribe()
		}
	}
	s.subs = nil
	s.started = false
	s.logger.Info("automation GitHub webhook subscriber stopped")
}

func (s *GitHubWebhookSubscriber) handlePush(ctx context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*github.GitHubPushEventPayload)
	if !ok || payload == nil {
		return nil
	}
	triggers, err := s.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubPush)
	if err != nil {
		s.logger.Error("failed to list github_push triggers", zap.Error(err))
		return nil
	}
	for i := range triggers {
		s.checkPushTrigger(ctx, &triggers[i], payload)
	}
	return nil
}

func (s *GitHubWebhookSubscriber) checkPushTrigger(
	ctx context.Context, t *AutomationTrigger, payload *github.GitHubPushEventPayload,
) {
	var cfg GitHubPushTriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		s.logger.Debug("invalid github_push trigger config", zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}
	if !s.triggerMatchesWorkspaceAndRepo(ctx, t, payload.WorkspaceIDs, payload.Owner, payload.Name, cfg.Repos) {
		return
	}
	if !matchesBranches(payload.Branch, cfg.Branches) {
		return
	}

	dedupKey := fmt.Sprintf("push:%s/%s@%s", payload.Owner, payload.Name, payload.SHA)
	data, _ := json.Marshal(map[string]interface{}{
		"repo":         fmt.Sprintf("%s/%s", payload.Owner, payload.Name),
		"branch":       payload.Branch,
		"sha":          payload.SHA,
		"pusher_login": payload.PusherLogin,
	})
	if err := s.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeGitHubPush, data, dedupKey); err != nil {
		s.logger.Error("failed to fire push trigger", zap.String("trigger_id", t.ID), zap.Error(err))
	}
}

func (s *GitHubWebhookSubscriber) handleCheckRun(ctx context.Context, event *bus.Event) error {
	payload, ok := event.Data.(*github.GitHubCheckRunEventPayload)
	if !ok || payload == nil {
		return nil
	}
	triggers, err := s.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubCI)
	if err != nil {
		s.logger.Error("failed to list github_ci triggers", zap.Error(err))
		return nil
	}
	for i := range triggers {
		s.checkCITrigger(ctx, &triggers[i], payload)
	}
	return nil
}

func (s *GitHubWebhookSubscriber) checkCITrigger(
	ctx context.Context, t *AutomationTrigger, payload *github.GitHubCheckRunEventPayload,
) {
	var cfg GitHubCITriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		s.logger.Debug("invalid github_ci trigger config", zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}
	if !s.triggerMatchesWorkspaceAndRepo(ctx, t, payload.WorkspaceIDs, payload.Owner, payload.Name, cfg.Repos) {
		return
	}
	if !matchesAuthors(payload.Conclusion, cfg.Conclusions) || !matchesAuthors(payload.CheckName, cfg.CheckNames) {
		return
	}

	dedupKey := fmt.Sprintf("ci:%s/%s#%d", payload.Owner, payload.Name, payload.CheckRunID)
	data, _ := json.Marshal(map[string]interface{}{
		"repo":         fmt.Sprintf("%s/%s", payload.Owner, payload.Name),
		"branch":       payload.Branch,
		"sha":          payload.SHA,
		"check_name":   payload.CheckName,
		"conclusion":   payload.Conclusion,
		"check_run_id": payload.CheckRunID,
	})
	if err := s.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeGitHubCI, data, dedupKey); err != nil {
		s.logger.Error("failed to fire CI trigger", zap.String("trigger_id", t.ID), zap.Error(err))
	}
}

// triggerMatchesWorkspaceAndRepo loads the trigger's owning automation and
// checks its workspace is among the event's resolved workspaces, and that
// the event's repo matches the trigger's repo filter (empty repo filter
// never matches — these webhook triggers cannot act without a repo).
func (s *GitHubWebhookSubscriber) triggerMatchesWorkspaceAndRepo(
	ctx context.Context, t *AutomationTrigger, workspaceIDs []string, owner, name string, repos []github.RepoFilter,
) bool {
	if !matchesRepo(owner, name, repos) {
		return false
	}
	a, err := s.svc.GetAutomation(ctx, t.AutomationID)
	if err != nil || a == nil {
		s.logger.Debug("automation trigger is missing workspace ownership",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return false
	}
	for _, id := range workspaceIDs {
		if id == a.WorkspaceID {
			return true
		}
	}
	return false
}
