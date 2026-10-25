package github

import (
	"context"
	"sort"
	"time"

	"go.uber.org/zap"
)

type queuedWatch struct {
	key    string
	issue  *IssueWatch
	review *ReviewWatch
}

// watchQueueLoop rotates one inventory across both kinds of search consumers.
// Independent timers with fixed ordering can repeatedly spend the same slot.
func (p *Poller) watchQueueLoop(ctx context.Context) {
	defer p.wg.Done()
	p.checkWatchQueues(ctx)
	ticker := time.NewTicker(defaultWatchPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkWatchQueues(ctx)
		}
	}
}

func (p *Poller) checkWatchQueues(ctx context.Context) {
	issues, issueErr := p.service.store.ListEnabledIssueWatches(ctx)
	reviews, reviewErr := p.service.store.ListEnabledReviewWatches(ctx)
	if issueErr != nil {
		p.logger.Warn("failed to list issue watches", zap.Error(issueErr))
	}
	if reviewErr != nil {
		p.logger.Warn("failed to list review watches", zap.Error(reviewErr))
	}
	p.pollWatchQueue(ctx, issues, reviews, issueErr == nil, reviewErr == nil)
}

func (p *Poller) checkReviewWatches(ctx context.Context) {
	watches, err := p.service.store.ListEnabledReviewWatches(ctx)
	if err != nil {
		p.logger.Warn("failed to list review watches", zap.Error(err))
		return
	}
	p.pollWatchQueue(ctx, nil, watches, false, true)
}

func (p *Poller) checkIssueWatches(ctx context.Context) {
	watches, err := p.service.store.ListEnabledIssueWatches(ctx)
	if err != nil {
		p.logger.Warn("failed to list issue watches", zap.Error(err))
		return
	}
	p.pollWatchQueue(ctx, watches, nil, true, false)
}

func (p *Poller) pollWatchQueue(ctx context.Context, issues []*IssueWatch, reviews []*ReviewWatch, issueLoaded, reviewLoaded bool) {
	if !p.watchQueueMu.TryLock() {
		return
	}
	defer p.watchQueueMu.Unlock()
	queue := make([]queuedWatch, 0, len(issues)+len(reviews))
	liveIssues, liveReviews := map[string]bool{}, map[string]bool{}
	cleanupCycle := newReviewCleanupCycle(ctx, p)
	for _, watch := range issues {
		liveIssues[watch.ID] = true
		queue = append(queue, queuedWatch{key: "issue:" + watch.ID, issue: watch})
	}
	for _, watch := range reviews {
		liveReviews[watch.ID] = true
		cleanupCycle.registerWatch(watch)
		queue = append(queue, queuedWatch{key: "review:" + watch.ID, review: watch})
	}
	if issueLoaded {
		p.service.pruneWatchFilterProgress("issue", liveIssues)
	}
	if reviewLoaded {
		p.service.pruneWatchFilterProgress("review", liveReviews)
	}
	sort.Slice(queue, func(i, j int) bool { return queue[i].key < queue[j].key })
	if len(queue) > 0 {
		start := p.watchCursor % len(queue)
		p.watchCursor = (start + 1) % len(queue)
		for i := 0; i < len(queue); i++ {
			if ctx.Err() != nil {
				return
			}
			watch := queue[(start+i)%len(queue)]
			if watch.issue != nil {
				p.pollIssueWatch(ctx, watch.issue)
			} else {
				p.pollReviewWatch(ctx, watch.review, cleanupCycle)
			}
		}
	}
	if issueLoaded {
		p.finishIssueWatchCleanup(ctx)
	}
	if reviewLoaded {
		p.finishReviewWatchCleanup(ctx, cleanupCycle)
	}
}

func (p *Poller) pollReviewWatch(ctx context.Context, watch *ReviewWatch, cleanupCycle *reviewCleanupCycle) {
	cleanupAdmission := scheduledReviewCleanupAdmission{
		workspace: cleanupCycle.admitWorkspace,
		record:    cleanupCycle.admit,
	}
	p.logger.Debug("polling review watch",
		zap.String("watch_id", watch.ID),
		zap.String("workspace_id", watch.WorkspaceID),
		zap.String("custom_query", watch.CustomQuery),
		zap.Int("repo_filters", len(watch.Repos)),
		zap.String("review_scope", watch.ReviewScope))

	newPRs, err := p.service.CheckReviewWatch(ctx, watch)
	if err != nil {
		p.logger.Debug("failed to check review watch",
			zap.String("id", watch.ID), zap.Error(err))
		return
	}
	p.logger.Debug("review watch checked",
		zap.String("watch_id", watch.ID),
		zap.Int("new_prs", len(newPRs)))
	for _, pr := range newPRs {
		p.logger.Info("new PR found for review",
			zap.String("watch_id", watch.ID),
			zap.String("repo", pr.RepoOwner+"/"+pr.RepoName),
			zap.Int("pr_number", pr.Number),
			zap.String("title", pr.Title))
		p.service.publishNewReviewPREvent(ctx, watch, pr)
	}
	// Clean up tasks for merged/closed PRs that the user hasn't opened.
	cleanup, err := p.service.cleanupScheduledMergedReviewTasks(ctx, watch, cleanupAdmission)
	if err != nil {
		p.logCleanupError("failed to cleanup merged review tasks", err,
			zap.String("watch_id", watch.ID))
	} else {
		cleanupCycle.apply(cleanup, watch.WorkspaceID, watch)
	}
	if cleanup.Deleted > 0 {
		p.logger.Info("cleaned up merged review tasks",
			zap.String("watch_id", watch.ID), zap.Int("deleted", cleanup.Deleted))
	}
}

func (p *Poller) finishReviewWatchCleanup(ctx context.Context, cleanupCycle *reviewCleanupCycle) {
	cleanupAdmission := scheduledReviewCleanupAdmission{
		workspace: cleanupCycle.admitWorkspace,
		record:    cleanupCycle.admit,
	}
	// Global orphan sweep: catches dedup rows whose watch was deleted or
	// disabled. Without this pass those rows (and the tasks they reference)
	// would never be re-examined, since the per-watch loop only iterates
	// enabled watches.
	cleanup, err := p.service.cleanupScheduledOrphanedReviewTasks(ctx, cleanupAdmission)
	if err != nil {
		p.logCleanupError("failed to sweep orphaned review tasks", err)
	} else {
		cleanupCycle.apply(cleanup, "", nil)
		if cleanup.Deleted > 0 {
			p.logger.Info("swept orphaned review tasks", zap.Int("deleted", cleanup.Deleted))
		}
	}
	p.pruneReviewCleanupRecordCircuits(ctx)
}

func (p *Poller) pollIssueWatch(ctx context.Context, watch *IssueWatch) {
	p.logger.Debug("polling issue watch",
		zap.String("watch_id", watch.ID),
		zap.String("workspace_id", watch.WorkspaceID),
		zap.String("custom_query", watch.CustomQuery),
		zap.Int("repo_filters", len(watch.Repos)))

	newIssues, err := p.service.CheckIssueWatch(ctx, watch)
	if err != nil {
		p.logger.Debug("failed to check issue watch",
			zap.String("id", watch.ID), zap.Error(err))
		return
	}
	p.logger.Debug("issue watch checked",
		zap.String("watch_id", watch.ID),
		zap.Int("new_issues", len(newIssues)))
	for _, issue := range newIssues {
		p.logger.Info("new issue found for watch",
			zap.String("watch_id", watch.ID),
			zap.String("repo", issue.RepoOwner+"/"+issue.RepoName),
			zap.Int("issue_number", issue.Number),
			zap.String("title", issue.Title))
		p.service.publishNewIssueEvent(ctx, watch, issue)
	}
	// Clean up tasks for closed issues that the user hasn't opened.
	if cleaned, err := p.service.CleanupClosedIssueTasks(ctx, watch); err != nil {
		p.logger.Warn("failed to cleanup closed issue tasks",
			zap.String("watch_id", watch.ID), zap.Error(err))
		if cleanupBatchShouldStop(err) {
			return
		}
	} else if cleaned > 0 {
		p.logger.Info("cleaned up closed issue tasks",
			zap.String("watch_id", watch.ID), zap.Int("deleted", cleaned))
	}
}

func (p *Poller) finishIssueWatchCleanup(ctx context.Context) {
	if cleaned, err := p.service.CleanupAllOrphanedIssueTasks(ctx); err != nil {
		p.logger.Warn("failed to sweep orphaned issue tasks", zap.Error(err))
	} else if cleaned > 0 {
		p.logger.Info("swept orphaned issue tasks", zap.Int("deleted", cleaned))
	}
}
