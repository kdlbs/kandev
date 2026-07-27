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

// Start subscribes to the GitHub webhook bus events. Idempotent, and
// retryable: a subscription failure leaves the subscriber un-started (partial
// subscriptions are rolled back) so a later Start re-attempts both, rather than
// permanently disabling push/CI processing.
func (s *GitHubWebhookSubscriber) Start(_ context.Context) {
	if s.started || s.eventBus == nil {
		return
	}

	subs, err := s.subscribeAll()
	if err != nil {
		for _, sub := range subs {
			_ = sub.Unsubscribe()
		}
		s.logger.Error("failed to subscribe automation GitHub webhook events; will retry on next Start",
			zap.Error(err))
		return
	}

	s.subs = subs
	s.started = true
	s.logger.Info("automation GitHub webhook subscriber started")
}

// subscribeAll subscribes to both webhook events, returning any subscriptions
// created so far alongside the first error (for rollback by the caller).
func (s *GitHubWebhookSubscriber) subscribeAll() ([]bus.Subscription, error) {
	var subs []bus.Subscription
	pushSub, err := s.eventBus.Subscribe(events.GitHubPushReceived, s.handlePush)
	if err != nil {
		return subs, fmt.Errorf("subscribe %s: %w", events.GitHubPushReceived, err)
	}
	subs = append(subs, pushSub)
	ciSub, err := s.eventBus.Subscribe(events.GitHubCheckRunCompleted, s.handleCheckRun)
	if err != nil {
		return subs, fmt.Errorf("subscribe %s: %w", events.GitHubCheckRunCompleted, err)
	}
	return append(subs, ciSub), nil
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

	// The same commit SHA can be pushed to multiple matching branches; key the
	// dedup on the branch too so a later branch's push isn't suppressed.
	dedupKey := fmt.Sprintf("push:%s/%s@%s@%s", payload.Owner, payload.Name, payload.Branch, payload.SHA)
	data, _ := json.Marshal(map[string]interface{}{
		"repo":         fmt.Sprintf("%s/%s", payload.Owner, payload.Name),
		"branch":       payload.Branch,
		"sha":          payload.SHA,
		"pusher_login": payload.PusherLogin,
		"message":      payload.HeadCommitMsg,
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
	if !matchesFilterValue(payload.Conclusion, cfg.Conclusions) ||
		!matchesFilterValue(payload.CheckName, cfg.CheckNames) ||
		!matchesBranches(payload.Branch, cfg.Branches) {
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
		"html_url":     payload.HTMLURL,
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
