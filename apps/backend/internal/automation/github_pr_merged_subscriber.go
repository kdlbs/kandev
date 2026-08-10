package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
)

// GitHubPRMergedSubscriber subscribes to github.task_pr.updated events and
// fires github_pr_merged automation triggers. The subscriber enforces all
// per-event gates (payload validity, merged state, repo identity, task
// lookup) and carries a per-event fired-key set to guarantee exactly one
// run per automation per event when multiple triggers match.
type GitHubPRMergedSubscriber struct {
	svc      *Service
	eventBus bus.EventBus
	logger   *logger.Logger

	sub     bus.Subscription
	started bool
}

// NewGitHubPRMergedSubscriber creates a new subscriber.
func NewGitHubPRMergedSubscriber(svc *Service, eventBus bus.EventBus, log *logger.Logger) *GitHubPRMergedSubscriber {
	return &GitHubPRMergedSubscriber{svc: svc, eventBus: eventBus, logger: log}
}

// Start subscribes to the PR-updated bus event. Idempotent; a second Start
// on an already-started subscriber is a no-op and emits neither log line.
func (s *GitHubPRMergedSubscriber) Start(_ context.Context) {
	if s.started {
		return
	}

	if s.eventBus == nil {
		s.started = true
		return
	}

	sub, err := s.eventBus.Subscribe(events.GitHubTaskPRUpdated, s.handlePRUpdated)
	if err != nil {
		s.logger.Error("failed to subscribe to github.task_pr.updated; github_pr_merged triggers will not fire",
			zap.Error(err))
		return
	}
	s.sub = sub
	s.started = true
	if s.svc.TaskOriginLookup() == nil {
		s.logger.Warn("github_pr_merged trigger will not fire until a task-origin lookup is wired",
			zap.String("trigger_type", string(TriggerTypeGitHubPRMerged)))
	} else {
		s.logger.Info("github_pr_merged subscription active")
	}
}

// Stop unsubscribes from the bus. Idempotent.
func (s *GitHubPRMergedSubscriber) Stop() {
	if !s.started {
		return
	}
	if s.sub != nil {
		_ = s.sub.Unsubscribe()
		s.sub = nil
	}
	s.started = false
	s.logger.Info("github_pr_merged subscriber stopped")
}

// firedKey is the per-event in-memory set element that prevents the same
// automation from firing more than once per event when it has multiple
// matching triggers.
type firedKey struct {
	automationID string
	dedupKey     string
}

func (s *GitHubPRMergedSubscriber) handlePRUpdated(ctx context.Context, event *bus.Event) error {
	// Gate 1: payload must be a typed TaskPR or its JSON-decoded bus form.
	pr, ok := normalizeTaskPR(event.Data)
	if !ok {
		return nil
	}

	// Gate 2: task id must be non-empty.
	if pr.TaskID == "" {
		return nil
	}

	// Gate 3: state must equal "merged" (case-insensitive, trimmed).
	if !strings.EqualFold(strings.TrimSpace(pr.State), "merged") {
		return nil
	}

	// Gate 4: owner and repo must both be non-empty.
	if pr.Owner == "" || pr.Repo == "" {
		return nil
	}

	// Gate 5: PR number must be positive.
	if pr.PRNumber <= 0 {
		return nil
	}

	// Gate 6: resolve the task — once per event, reused for every trigger.
	lookup := s.svc.TaskOriginLookup()
	if lookup == nil {
		return nil
	}
	workspaceID, isAutomationRun, resolved := lookup.TaskWorkspaceAndAutomationOrigin(ctx, pr.TaskID)
	if !resolved {
		return nil
	}

	// Gate 7: the task must not be an automation run (loop guard).
	if isAutomationRun {
		return nil
	}

	// List all enabled github_pr_merged triggers.
	triggers, err := s.svc.Store().ListEnabledTriggersByType(ctx, TriggerTypeGitHubPRMerged)
	if err != nil {
		s.logger.Error("failed to list github_pr_merged triggers", zap.Error(err))
		return nil
	}

	// Per-event fired-key set: (automation_id, dedup_key) pairs already
	// handed to FireTrigger during this event. Prevents a second trigger on
	// the same automation from duplicating the run.
	fired := make(map[firedKey]struct{})

	for i := range triggers {
		s.checkMergedTrigger(ctx, &triggers[i], pr, workspaceID, fired)
	}
	return nil
}

// normalizeTaskPR accepts both event representations used by the configured
// event buses. The in-memory bus preserves the typed pointer, while NATS
// decodes JSON data into an untyped map before invoking subscribers.
func normalizeTaskPR(data interface{}) (*github.TaskPR, bool) {
	switch pr := data.(type) {
	case *github.TaskPR:
		return pr, pr != nil
	case github.TaskPR:
		return &pr, true
	default:
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, false
		}
		var decoded github.TaskPR
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return nil, false
		}
		return &decoded, true
	}
}

func (s *GitHubPRMergedSubscriber) checkMergedTrigger(
	ctx context.Context,
	t *AutomationTrigger,
	pr *github.TaskPR,
	resolvedWorkspaceID string,
	fired map[firedKey]struct{},
) {
	// Parse trigger config; skip on error.
	var cfg GitHubPRMergedTriggerConfig
	if err := json.Unmarshal(t.Config, &cfg); err != nil {
		s.logger.Debug("invalid github_pr_merged trigger config",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}

	// Gate 8: workspace must match (lookup's workspace is authoritative).
	a, err := s.svc.GetAutomation(ctx, t.AutomationID)
	if err != nil || a == nil || a.WorkspaceID == "" {
		s.logger.Debug("could not load automation for github_pr_merged trigger",
			zap.String("trigger_id", t.ID), zap.Error(err))
		return
	}
	if a.WorkspaceID != resolvedWorkspaceID {
		return
	}

	// Gate 9: repository filter.
	if !matchesMergedRepo(pr.Owner, pr.Repo, cfg) {
		return
	}

	// Gate 10: base-branch filter (empty list = match all; trim and drop empties at match time).
	if !matchesMergedBranches(pr.BaseBranch, cfg.BaseBranches) {
		return
	}

	// Build the dedup key.
	dedupKey := fmt.Sprintf("pr_merged:%s:%s/%s#%d",
		pr.TaskID,
		strings.ToLower(pr.Owner),
		strings.ToLower(pr.Repo),
		pr.PRNumber,
	)

	// Gate 11: per-event fired-key set.
	fk := firedKey{automationID: t.AutomationID, dedupKey: dedupKey}
	if _, alreadyFired := fired[fk]; alreadyFired {
		return
	}
	fired[fk] = struct{}{}

	// Build the trigger data map (narrow, no free-form prose).
	var mergedAt string
	if pr.MergedAt != nil {
		mergedAt = pr.MergedAt.UTC().Format(time.RFC3339)
	}
	data, _ := json.Marshal(map[string]interface{}{
		"task_id":     pr.TaskID,
		"repo":        fmt.Sprintf("%s/%s", pr.Owner, pr.Repo),
		"pr_number":   pr.PRNumber,
		"pr_url":      pr.PRURL,
		"base_branch": pr.BaseBranch,
		"merged_at":   mergedAt,
	})

	// Pass the dedup key so FireTrigger's store-level check prevents duplicate
	// runs for the same PR event. If the concurrent-run cap is reached inside
	// FireTrigger, the skipped-run record is written with an empty key so the
	// dedup slot is not consumed (see maybeSkipForConcurrencyCap).
	if _, err := s.svc.FireTrigger(ctx, t.AutomationID, t.ID, TriggerTypeGitHubPRMerged, data, dedupKey); err != nil {
		s.logger.Error("failed to fire github_pr_merged trigger",
			zap.String("trigger_id", t.ID), zap.Error(err))
	}
}

// matchesMergedRepo implements the repository-matching semantics for
// github_pr_merged, which differ from the shared matchesRepo helper:
//   - all_repos=true: every repository matches.
//   - all_repos=false + non-empty list: match by owner (case-insensitive) and
//     name (case-insensitive, empty name = org wildcard).
//   - all_repos=false + empty list: never matches.
func matchesMergedRepo(owner, name string, cfg GitHubPRMergedTriggerConfig) bool {
	if cfg.AllRepos {
		return true
	}
	for _, r := range cfg.Repos {
		if r.Owner == "" {
			// No owner → matches nothing.
			continue
		}
		if !strings.EqualFold(r.Owner, owner) {
			continue
		}
		// Empty name = organization wildcard: any name under this owner.
		if r.Name == "" || strings.EqualFold(r.Name, name) {
			return true
		}
	}
	return false
}

// matchesMergedBranches checks whether baseBranch matches the filter list,
// trimming entries and dropping empties before matching. An empty list after
// that normalization (originally empty or all-empty entries) matches everything.
func matchesMergedBranches(baseBranch string, filters []string) bool {
	var effective []string
	for _, f := range filters {
		if t := strings.TrimSpace(f); t != "" {
			effective = append(effective, t)
		}
	}
	if len(effective) == 0 {
		return true
	}
	for _, f := range effective {
		if matchGlob(f, baseBranch) {
			return true
		}
	}
	return false
}
