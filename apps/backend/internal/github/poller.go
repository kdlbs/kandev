package github

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	defaultPRPollInterval     = 1 * time.Minute
	defaultReviewPollInterval = 5 * time.Minute
)

// Poller runs background loops for PR monitoring and review queue checking.
type Poller struct {
	service  *Service
	eventBus bus.EventBus
	logger   *logger.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewPoller creates a new background poller.
func NewPoller(svc *Service, eventBus bus.EventBus, log *logger.Logger) *Poller {
	return &Poller{
		service:  svc,
		eventBus: eventBus,
		logger:   log,
	}
}

// Start begins the background polling loops.
// Calling Start more than once without Stop is a no-op.
func (p *Poller) Start(ctx context.Context) {
	if p.started {
		return
	}
	p.started = true
	ctx, p.cancel = context.WithCancel(ctx)

	p.wg.Add(2) //nolint:mnd
	go p.prMonitorLoop(ctx)
	go p.reviewQueueLoop(ctx)

	p.logger.Info("GitHub poller started")
}

// Stop cancels the polling loops and waits for them to finish.
func (p *Poller) Stop() {
	if !p.started {
		return
	}
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	p.started = false
	p.logger.Info("GitHub poller stopped")
}

// prMonitorLoop polls PR watches for new feedback.
func (p *Poller) prMonitorLoop(ctx context.Context) {
	defer p.wg.Done()

	// Run an initial check immediately so existing watches are evaluated on startup.
	p.checkPRWatches(ctx)

	ticker := time.NewTicker(defaultPRPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkPRWatches(ctx)
		}
	}
}

func (p *Poller) checkPRWatches(ctx context.Context) {
	watches, err := p.service.ListActivePRWatches(ctx)
	if err != nil {
		p.logger.Error("failed to list PR watches", zap.Error(err))
		return
	}
	for _, watch := range watches {
		p.checkSinglePRWatch(ctx, watch)
	}
}

func (p *Poller) checkSinglePRWatch(ctx context.Context, watch *PRWatch) {
	// PRWatch with pr_number=0 means we're still searching for a PR on this branch.
	if watch.PRNumber == 0 {
		p.detectPRForWatch(ctx, watch)
		return
	}

	feedback, hasNew, err := p.service.CheckPRWatch(ctx, watch)
	if err != nil {
		p.logger.Debug("failed to check PR watch",
			zap.String("id", watch.ID), zap.Error(err))
		return
	}
	if feedback == nil {
		return
	}

	// Auto-cleanup: remove watch when PR is merged or closed.
	if feedback.PR != nil && (feedback.PR.State == prStateMerged || feedback.PR.State == prStateClosed) {
		if delErr := p.service.DeletePRWatch(ctx, watch.ID); delErr != nil {
			p.logger.Error("failed to delete completed PR watch",
				zap.String("id", watch.ID), zap.Error(delErr))
		} else {
			p.logger.Info("removed PR watch for completed PR",
				zap.String("id", watch.ID),
				zap.String("state", feedback.PR.State),
				zap.Int("pr_number", watch.PRNumber))
		}
		return
	}

	if !hasNew {
		return
	}

	// Sync task-PR record with latest data
	if syncErr := p.service.SyncTaskPR(ctx, watch.TaskID, feedback); syncErr != nil {
		p.logger.Error("failed to sync task PR", zap.String("task_id", watch.TaskID), zap.Error(syncErr))
	}

	// Publish feedback event for UI notification
	p.publishPRFeedbackEvent(ctx, watch, feedback)
}

// detectPRForWatch searches GitHub for a PR on the watch's branch.
// If found, updates the watch with the PR number and creates the TaskPR association.
func (p *Poller) detectPRForWatch(ctx context.Context, watch *PRWatch) {
	if p.service.client == nil {
		return
	}

	pr, err := p.service.client.FindPRByBranch(ctx, watch.Owner, watch.Repo, watch.Branch)
	if err != nil {
		p.logger.Debug("failed to search for PR by branch",
			zap.String("watch_id", watch.ID),
			zap.String("branch", watch.Branch),
			zap.Error(err))
		return
	}

	// Update last_checked_at regardless of result
	now := time.Now().UTC()
	_ = p.service.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, nil, "")

	if pr == nil {
		return
	}

	// Found a PR — update the watch and create association
	if updateErr := p.service.store.UpdatePRWatchPRNumber(ctx, watch.ID, pr.Number); updateErr != nil {
		p.logger.Error("failed to update PR watch with detected PR",
			zap.String("watch_id", watch.ID),
			zap.Int("pr_number", pr.Number),
			zap.Error(updateErr))
		return
	}

	if _, assocErr := p.service.AssociatePRWithTask(ctx, watch.TaskID, pr); assocErr != nil {
		p.logger.Error("failed to associate detected PR with task",
			zap.String("task_id", watch.TaskID),
			zap.Int("pr_number", pr.Number),
			zap.Error(assocErr))
		return
	}

	p.logger.Info("detected PR for session branch",
		zap.String("watch_id", watch.ID),
		zap.String("branch", watch.Branch),
		zap.Int("pr_number", pr.Number))
}

func (p *Poller) publishPRFeedbackEvent(ctx context.Context, watch *PRWatch, feedback *PRFeedback) {
	if p.eventBus == nil {
		return
	}
	evt := &PRFeedbackEvent{
		SessionID:      watch.SessionID,
		TaskID:         watch.TaskID,
		PRNumber:       watch.PRNumber,
		Owner:          watch.Owner,
		Repo:           watch.Repo,
		NewComments:    len(feedback.Comments),
		ChecksChanged:  true,
		NewCheckStatus: computeOverallCheckStatus(feedback.Checks),
		NewReviewState: computeOverallReviewState(feedback.Reviews),
	}
	event := bus.NewEvent(events.GitHubPRFeedback, "github_poller", evt)
	if err := p.eventBus.Publish(ctx, events.GitHubPRFeedback, event); err != nil {
		p.logger.Debug("failed to publish PR feedback event", zap.Error(err))
	}
}

// reviewQueueLoop polls review watches for new PRs.
func (p *Poller) reviewQueueLoop(ctx context.Context) {
	defer p.wg.Done()

	// Run an initial check immediately so existing watches are evaluated on startup.
	p.checkReviewWatches(ctx)

	ticker := time.NewTicker(defaultReviewPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkReviewWatches(ctx)
		}
	}
}

func (p *Poller) checkReviewWatches(ctx context.Context) {
	watches, err := p.service.store.ListEnabledReviewWatches(ctx)
	if err != nil {
		p.logger.Error("failed to list review watches", zap.Error(err))
		return
	}
	if len(watches) == 0 {
		return
	}
	p.logger.Debug("checking review watches", zap.Int("count", len(watches)))
	for _, watch := range watches {
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
			continue
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
	}
}
