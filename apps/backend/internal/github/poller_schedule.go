package github

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	searchFastPollInterval     = 1 * time.Minute
	searchIdlePollInterval     = 15 * time.Minute
	searchVeryIdlePollInterval = 30 * time.Minute
	searchIdleThreshold        = 2 * time.Hour
	searchVeryIdleThreshold    = 24 * time.Hour
)

// PRWatchTaskActivity is the narrow task-domain projection used to decide
// whether a searching PR watch should be checked. It intentionally excludes
// generic task/watch UpdatedAt values because provider synchronization can
// write those fields.
type PRWatchTaskActivity struct {
	LastActivityAt time.Time
	Running        bool
}

// TaskActivityProvider supplies persisted activity for a bulk set of tasks.
// Implementations must use task-local evidence only; the poller never asks a
// provider or remote Git host to classify idleness.
type TaskActivityProvider interface {
	LoadPRWatchTaskActivity(ctx context.Context, taskIDs []string) (map[string]PRWatchTaskActivity, error)
}

func (p *Poller) selectDuePRWatches(ctx context.Context, watches []*PRWatch) []*PRWatch {
	return selectDuePRWatches(ctx, watches, p.taskActivityProvider, p.now(), p.logger)
}

func selectDuePRWatches(
	ctx context.Context,
	watches []*PRWatch,
	provider TaskActivityProvider,
	now time.Time,
	log *logger.Logger,
) []*PRWatch {
	if len(watches) == 0 {
		return nil
	}
	activityByTask := loadPRWatchTaskActivities(ctx, watches, provider, log)
	groupDue := duePRWatchGroups(now, watches, activityByTask)

	due := make([]*PRWatch, 0, len(watches))
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		if groupDue[prWatchDiscoveryTargetKey(watch)] {
			due = append(due, watch)
		}
	}
	return due
}

func loadPRWatchTaskActivities(
	ctx context.Context,
	watches []*PRWatch,
	provider TaskActivityProvider,
	log *logger.Logger,
) map[string]PRWatchTaskActivity {
	if provider == nil {
		return nil
	}
	taskIDs := prWatchActivityTaskIDs(watches)
	if len(taskIDs) == 0 {
		return nil
	}
	loaded, err := provider.LoadPRWatchTaskActivity(ctx, taskIDs)
	if err != nil {
		if log != nil {
			log.Debug("failed to load PR watch task activity", zap.Error(err))
		}
		return nil
	}
	return loaded
}

func prWatchActivityTaskIDs(watches []*PRWatch) []string {
	seen := make(map[string]struct{}, len(watches))
	taskIDs := make([]string, 0, len(watches))
	for _, watch := range watches {
		if watch == nil || watch.PRNumber != 0 || watch.TaskID == "" {
			continue
		}
		if _, ok := seen[watch.TaskID]; ok {
			continue
		}
		seen[watch.TaskID] = struct{}{}
		taskIDs = append(taskIDs, watch.TaskID)
	}
	sort.Strings(taskIDs)
	return taskIDs
}

func duePRWatchGroups(
	now time.Time,
	watches []*PRWatch,
	activityByTask map[string]PRWatchTaskActivity,
) map[string]bool {
	groupDue := make(map[string]bool, len(watches))
	for _, watch := range watches {
		markDuePRWatchGroup(groupDue, now, watch, activityByTask)
	}
	return groupDue
}

func markDuePRWatchGroup(
	groupDue map[string]bool,
	now time.Time,
	watch *PRWatch,
	activityByTask map[string]PRWatchTaskActivity,
) {
	if watch == nil {
		return
	}
	key := prWatchDiscoveryTargetKey(watch)
	if watch.PRNumber != 0 {
		groupDue[key] = true
		return
	}
	activity := activityByTask[watch.TaskID]
	if prWatchIsDue(now, watch, activity) {
		groupDue[key] = true
	}
}

func prWatchSearchInterval(now time.Time, watch *PRWatch, activity PRWatchTaskActivity) time.Duration {
	if watch == nil || watch.PRNumber != 0 {
		return defaultPRPollInterval
	}
	if activity.Running || activity.LastActivityAt.IsZero() {
		return searchFastPollInterval
	}
	activityAt := activity.LastActivityAt
	if watch != nil && watch.CreatedAt.After(activityAt) {
		activityAt = watch.CreatedAt
	}
	idleFor := now.Sub(activityAt)
	if idleFor < searchIdleThreshold {
		return searchFastPollInterval
	}
	if idleFor < searchVeryIdleThreshold {
		return searchIdlePollInterval
	}
	return searchVeryIdlePollInterval
}

func prWatchIsDue(now time.Time, watch *PRWatch, activity PRWatchTaskActivity) bool {
	if watch == nil {
		return false
	}
	if watch.LastCheckedAt == nil {
		return true
	}
	return now.Sub(*watch.LastCheckedAt) >= prWatchSearchInterval(now, watch, activity)
}

func prWatchDiscoveryTargetKey(watch *PRWatch) string {
	if watch == nil {
		return ""
	}
	mode := "search"
	if watch.PRNumber != 0 {
		mode = "pr:" + strconv.Itoa(watch.PRNumber)
	}
	return watch.WorkspaceID + "\x00" + mode + "\x00" + watch.Owner + "\x00" + watch.Repo + "\x00" + watch.Branch
}

func (p *Poller) now() time.Time {
	if p != nil && p.clock != nil {
		return p.clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) now() time.Time {
	if s == nil {
		return time.Now().UTC()
	}
	s.clockMu.RLock()
	clock := s.clock
	s.clockMu.RUnlock()
	if clock != nil {
		return clock().UTC()
	}
	return time.Now().UTC()
}
