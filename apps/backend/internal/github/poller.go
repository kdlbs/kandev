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
	defaultPRPollInterval     = 30 * time.Second
	defaultReviewPollInterval = 5 * time.Minute
	defaultIssuePollInterval  = 5 * time.Minute
	// rateLimitedSleepCap bounds the rate-limit sleep so a misreported reset
	// (e.g. far-future reset_at after a CLI failure) cannot wedge the loop.
	rateLimitedSleepCap = 10 * time.Minute
)

// searchBucketExhausted reports whether the search bucket is currently
// exhausted. Used by per-watch loops to bail out mid-cycle once a single
// search request has tripped the limit, so the remaining watches don't issue
// doomed searches that worsen secondary-limit penalties. Returns false when
// the tracker is unavailable (tests, disabled feature).
func (p *Poller) searchBucketExhausted(loop string) bool {
	if p.service == nil || p.service.rateTracker == nil {
		return false
	}
	if p.service.rateTracker.WaitDuration(ResourceSearch) <= 0 {
		return false
	}
	p.logger.Debug("search bucket exhausted; stopping watch loop early",
		zap.String("loop", loop))
	return true
}

// waitForRateLimit returns true after sleeping the loop until the relevant
// resource bucket has quota again, or false if ctx was cancelled. When the
// bucket is healthy the call is a no-op and returns true immediately.
func (p *Poller) waitForRateLimit(ctx context.Context, resource Resource, loop string) bool {
	if p.service == nil || p.service.rateTracker == nil {
		return true
	}
	d := p.service.rateTracker.WaitDuration(resource)
	if d <= 0 {
		return true
	}
	if d > rateLimitedSleepCap {
		d = rateLimitedSleepCap
	}
	p.logger.Info("github rate-limited; pausing poller loop",
		zap.String("loop", loop),
		zap.String("resource", string(resource)),
		zap.Duration("sleep", d))
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// TaskBranchInfo describes a task+session that may need a PR watch.
// RepositoryID scopes multi-repo tasks: each repo is a separate watch row.
// Empty for legacy single-repo tasks.
type TaskBranchInfo struct {
	TaskID       string
	SessionID    string
	RepositoryID string
	Owner        string
	Repo         string
	Branch       string
}

// TaskBranchProvider lists tasks that should have PR watches and resolves branches.
type TaskBranchProvider interface {
	ListTasksNeedingPRWatch(ctx context.Context) ([]TaskBranchInfo, error)
	// ResolveBranchForSession returns the current branch for a task+session pair.
	// Used to detect branch renames and update stale PR watches.
	ResolveBranchForSession(ctx context.Context, taskID, sessionID string) string
}

// Poller runs background loops for PR monitoring and review queue checking.
type Poller struct {
	service            *Service
	eventBus           bus.EventBus
	logger             *logger.Logger
	taskBranchProvider TaskBranchProvider

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

	p.wg.Add(3) //nolint:mnd
	go p.prMonitorLoop(ctx)
	go p.reviewQueueLoop(ctx)
	go p.issueWatchLoop(ctx)

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
	if p.waitForRateLimit(ctx, ResourceGraphQL, "pr_monitor") {
		p.checkPRWatches(ctx)
	}

	ticker := time.NewTicker(defaultPRPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !p.waitForRateLimit(ctx, ResourceGraphQL, "pr_monitor") {
				return
			}
			p.checkPRWatches(ctx)
		}
	}
}

func (p *Poller) checkPRWatches(ctx context.Context) {
	p.reconcileWatches(ctx)

	watches, err := p.service.ListActivePRWatches(ctx)
	if err != nil {
		p.logger.Error("failed to list PR watches", zap.Error(err))
		return
	}
	if len(watches) == 0 {
		return
	}

	// Try the batched GraphQL path first — collapses N HTTP requests into a
	// handful of multi-aliased GraphQL calls. On error or unsupported client
	// (e.g. NoopClient), fall back to per-watch checks so one bad poll cycle
	// doesn't lose state.
	if p.tryBatchedPRWatchCheck(ctx, watches) {
		return
	}
	for _, watch := range watches {
		p.checkSinglePRWatch(ctx, watch)
	}
}

// tryBatchedPRWatchCheck runs the batched GraphQL flow for the supplied
// watches. Returns true when the path succeeded so the caller can skip the
// per-watch fallback.
func (p *Poller) tryBatchedPRWatchCheck(ctx context.Context, watches []*PRWatch) bool {
	exec, err := graphQLExecutorFor(p.service.client)
	if err != nil {
		return false
	}
	numbered, searching := splitPRWatches(watches)
	statusByKey, ok := p.fetchBatchedStatuses(ctx, exec, numbered, searching)
	if !ok {
		return false
	}
	for _, w := range numbered {
		key := prStatusCacheKey(w.Owner, w.Repo, w.PRNumber)
		status, ok := statusByKey[key]
		if !ok {
			// Alias missing — PR may have been deleted; fall through to the
			// per-watch path which already handles the not-found case.
			p.checkSinglePRWatch(ctx, w)
			continue
		}
		p.applyPRStatus(ctx, w, status)
	}
	for _, w := range searching {
		key := graphqlBranchKey(w.Owner, w.Repo, w.Branch)
		status, ok := statusByKey[key]
		if !ok || status == nil || status.PR == nil {
			// No PR yet for this branch — record the timestamp like the
			// per-watch path so liveness updates.
			now := time.Now().UTC()
			_ = p.service.store.UpdatePRWatchTimestamps(ctx, w.ID, now, nil, "", "")
			continue
		}
		p.applyDetectedPR(ctx, w, status.PR)
	}
	return true
}

// fetchBatchedStatuses runs both the numbered-PR query and the branch query
// against GraphQL. The combined map keys numbered watches by
// prStatusCacheKey and searching watches by graphqlBranchKey, so callers can
// look up either kind in one place. Returns (nil, false) when any required
// query failed so the caller falls back to per-watch checks rather than
// silently absorbing the failure as "no result".
func (p *Poller) fetchBatchedStatuses(
	ctx context.Context, exec GraphQLExecutor, numbered, searching []*PRWatch,
) (map[string]*PRStatus, bool) {
	combined := make(map[string]*PRStatus)

	if len(numbered) > 0 {
		refs := make([]graphQLPRRef, 0, len(numbered))
		for _, w := range numbered {
			refs = append(refs, graphQLPRRef{Owner: w.Owner, Repo: w.Repo, Number: w.PRNumber})
		}
		out, err := runBatchedPRQuery(ctx, exec, refs)
		if err != nil {
			p.logger.Debug("batched PR status query failed", zap.Error(err))
			return nil, false
		}
		for k, v := range out {
			combined[k] = v
		}
	}
	if len(searching) > 0 {
		refs := make([]graphQLBranchRef, 0, len(searching))
		for _, w := range searching {
			refs = append(refs, graphQLBranchRef{Owner: w.Owner, Repo: w.Repo, Branch: w.Branch})
		}
		out, err := runBatchedBranchQuery(ctx, exec, refs)
		if err != nil {
			p.logger.Debug("batched branch query failed", zap.Error(err))
			return nil, false
		}
		for k, v := range out {
			combined[k] = v
		}
	}
	return combined, true
}

// splitPRWatches partitions watches into "we know the PR number" and "still
// searching for one on this branch" buckets so each gets its own batched
// query shape.
func splitPRWatches(watches []*PRWatch) (numbered, searching []*PRWatch) {
	for _, w := range watches {
		if w.PRNumber == 0 {
			searching = append(searching, w)
		} else {
			numbered = append(numbered, w)
		}
	}
	return numbered, searching
}

// applyPRStatus is the post-fetch processing extracted from
// checkSinglePRWatch so the batched path can reuse it.
func (p *Poller) applyPRStatus(ctx context.Context, watch *PRWatch, status *PRStatus) {
	if status == nil {
		return
	}
	hasNew := status.ChecksState != watch.LastCheckStatus || status.ReviewState != watch.LastReviewState
	now := time.Now().UTC()
	if err := p.service.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, nil, status.ChecksState, status.ReviewState); err != nil {
		p.logger.Error("failed to update PR watch timestamps", zap.String("id", watch.ID), zap.Error(err))
	}
	if syncErr := p.service.SyncTaskPR(ctx, watch.TaskID, status); syncErr != nil {
		p.logger.Error("failed to sync task PR",
			zap.String("task_id", watch.TaskID), zap.Error(syncErr))
		return
	}
	if status.PR != nil && (status.PR.State == prStateMerged || status.PR.State == prStateClosed) {
		p.publishPRStatusEvent(ctx, watch, status)
		if resetErr := p.service.store.UpdatePRWatchPRNumber(ctx, watch.ID, 0); resetErr != nil {
			p.logger.Error("failed to reset completed PR watch",
				zap.String("id", watch.ID), zap.Error(resetErr))
		}
		return
	}
	if hasNew {
		p.publishPRStatusEvent(ctx, watch, status)
	}
}

// applyDetectedPR is the searching-watch counterpart to applyPRStatus —
// promotes a PRWatch from "searching" to a known PR number and records the
// task association.
func (p *Poller) applyDetectedPR(ctx context.Context, watch *PRWatch, pr *PR) {
	now := time.Now().UTC()
	_ = p.service.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, nil, "", "")
	if err := p.service.store.UpdatePRWatchPRNumber(ctx, watch.ID, pr.Number); err != nil {
		p.logger.Error("failed to update PR watch with detected PR",
			zap.String("watch_id", watch.ID), zap.Int("pr_number", pr.Number), zap.Error(err))
		return
	}
	if _, err := p.service.AssociatePRWithTask(ctx, watch.TaskID, watch.RepositoryID, pr); err != nil {
		p.logger.Error("failed to associate detected PR with task",
			zap.String("task_id", watch.TaskID), zap.Int("pr_number", pr.Number), zap.Error(err))
		return
	}
	p.logger.Info("detected PR for session branch (batched)",
		zap.String("watch_id", watch.ID),
		zap.String("branch", watch.Branch),
		zap.Int("pr_number", pr.Number))
}

func (p *Poller) checkSinglePRWatch(ctx context.Context, watch *PRWatch) {
	// PRWatch with pr_number=0 means we're still searching for a PR on this branch.
	if watch.PRNumber == 0 {
		p.detectPRForWatch(ctx, watch)
		return
	}

	status, hasNew, err := p.service.CheckPRWatch(ctx, watch)
	if err != nil {
		p.logger.Debug("failed to check PR watch",
			zap.String("id", watch.ID), zap.Error(err))
		return
	}
	if status == nil {
		return
	}

	// Always sync latest PR state to the task-PR record.
	if syncErr := p.service.SyncTaskPR(ctx, watch.TaskID, status); syncErr != nil {
		p.logger.Error("failed to sync task PR",
			zap.String("task_id", watch.TaskID), zap.Error(syncErr))
		return // Keep watch so the next cycle can retry
	}

	// When the tracked PR is merged or closed, reset the watch back to the
	// "searching" state (pr_number=0) rather than deleting it. This lets the
	// poller discover a follow-up PR opened on the same branch (e.g. the user
	// closes #1 and opens #2 as a replacement) without requiring manual
	// intervention. The watch is only deleted when its owning session is gone.
	if status.PR != nil && (status.PR.State == prStateMerged || status.PR.State == prStateClosed) {
		p.publishPRStatusEvent(ctx, watch, status)
		if resetErr := p.service.store.UpdatePRWatchPRNumber(ctx, watch.ID, 0); resetErr != nil {
			p.logger.Error("failed to reset completed PR watch",
				zap.String("id", watch.ID), zap.Error(resetErr))
		} else {
			p.logger.Info("reset PR watch after PR completion",
				zap.String("id", watch.ID),
				zap.String("state", status.PR.State),
				zap.Int("pr_number", watch.PRNumber))
		}
		return
	}

	if !hasNew {
		return
	}

	p.publishPRStatusEvent(ctx, watch, status)
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
	_ = p.service.store.UpdatePRWatchTimestamps(ctx, watch.ID, now, nil, "", "")

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

	if _, assocErr := p.service.AssociatePRWithTask(ctx, watch.TaskID, watch.RepositoryID, pr); assocErr != nil {
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

func (p *Poller) publishPRStatusEvent(ctx context.Context, watch *PRWatch, status *PRStatus) {
	if p.eventBus == nil {
		return
	}
	evt := &PRFeedbackEvent{
		SessionID:      watch.SessionID,
		TaskID:         watch.TaskID,
		PRNumber:       watch.PRNumber,
		Owner:          watch.Owner,
		Repo:           watch.Repo,
		NewCheckStatus: status.ChecksState,
		NewReviewState: status.ReviewState,
	}
	event := bus.NewEvent(events.GitHubPRFeedback, "github_poller", evt)
	if err := p.eventBus.Publish(ctx, events.GitHubPRFeedback, event); err != nil {
		p.logger.Debug("failed to publish PR feedback event", zap.Error(err))
	}
}

// SetTaskBranchProvider sets the provider used for watch reconciliation.
func (p *Poller) SetTaskBranchProvider(provider TaskBranchProvider) {
	p.taskBranchProvider = provider
}

// reconcileWatches ensures PR watches exist for all tasks that need them,
// and refreshes stale branches on existing watches that haven't found a PR yet.
func (p *Poller) reconcileWatches(ctx context.Context) {
	if p.taskBranchProvider == nil {
		return
	}

	// 1. Refresh branches on existing pr_number=0 watches (branch may have changed).
	p.refreshStaleBranches(ctx)

	// 2. Create new watches for sessions that don't have one.
	tasks, err := p.taskBranchProvider.ListTasksNeedingPRWatch(ctx)
	if err != nil {
		p.logger.Error("failed to list tasks needing PR watch", zap.Error(err))
		return
	}
	for _, task := range tasks {
		if _, ensureErr := p.service.EnsurePRWatch(
			ctx, task.SessionID, task.TaskID, task.RepositoryID, task.Owner, task.Repo, task.Branch,
		); ensureErr != nil {
			p.logger.Error("failed to ensure PR watch",
				zap.String("session_id", task.SessionID), zap.Error(ensureErr))
		}
	}
}

// refreshStaleBranches re-resolves branches for watches that haven't found a PR yet.
// If the user renamed/changed the branch, the watch is updated so the next poll
// searches on the correct branch.
func (p *Poller) refreshStaleBranches(ctx context.Context) {
	watches, err := p.service.ListActivePRWatches(ctx)
	if err != nil {
		return
	}
	for _, watch := range watches {
		if watch.PRNumber != 0 {
			continue // already found a PR, branch is correct
		}
		currentBranch := p.taskBranchProvider.ResolveBranchForSession(
			ctx, watch.TaskID, watch.SessionID,
		)
		if currentBranch == "" || currentBranch == watch.Branch {
			continue
		}
		p.logger.Info("PR watch branch changed, updating",
			zap.String("session_id", watch.SessionID),
			zap.String("old_branch", watch.Branch),
			zap.String("new_branch", currentBranch))
		if updateErr := p.service.UpdatePRWatchBranchIfSearching(ctx, watch.ID, currentBranch); updateErr != nil {
			p.logger.Error("failed to update PR watch branch",
				zap.String("watch_id", watch.ID), zap.Error(updateErr))
		}
	}
}

// reviewQueueLoop polls review watches for new PRs.
func (p *Poller) reviewQueueLoop(ctx context.Context) {
	defer p.wg.Done()

	if p.waitForRateLimit(ctx, ResourceSearch, "review_queue") {
		p.checkReviewWatches(ctx)
	}

	ticker := time.NewTicker(defaultReviewPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !p.waitForRateLimit(ctx, ResourceSearch, "review_queue") {
				return
			}
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
		// A previous iteration in this same cycle may have exhausted the
		// search bucket. Stop early instead of issuing another doomed search
		// that would only deepen the secondary-limit penalty.
		if p.searchBucketExhausted("review_watch") {
			return
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
		// Clean up tasks for merged/closed PRs that the user hasn't opened.
		if cleaned, err := p.service.CleanupMergedReviewTasks(ctx, watch); err != nil {
			p.logger.Warn("failed to cleanup merged review tasks",
				zap.String("watch_id", watch.ID), zap.Error(err))
		} else if cleaned > 0 {
			p.logger.Info("cleaned up merged review tasks",
				zap.String("watch_id", watch.ID), zap.Int("deleted", cleaned))
		}
	}
}

// issueWatchLoop polls issue watches for new GitHub issues.
func (p *Poller) issueWatchLoop(ctx context.Context) {
	defer p.wg.Done()

	if p.waitForRateLimit(ctx, ResourceSearch, "issue_watch") {
		p.checkIssueWatches(ctx)
	}

	ticker := time.NewTicker(defaultIssuePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !p.waitForRateLimit(ctx, ResourceSearch, "issue_watch") {
				return
			}
			p.checkIssueWatches(ctx)
		}
	}
}

func (p *Poller) checkIssueWatches(ctx context.Context) {
	watches, err := p.service.store.ListEnabledIssueWatches(ctx)
	if err != nil {
		p.logger.Error("failed to list issue watches", zap.Error(err))
		return
	}
	if len(watches) == 0 {
		return
	}
	p.logger.Debug("checking issue watches", zap.Int("count", len(watches)))
	for _, watch := range watches {
		if p.searchBucketExhausted("issue_watch") {
			return
		}
		p.logger.Debug("polling issue watch",
			zap.String("watch_id", watch.ID),
			zap.String("workspace_id", watch.WorkspaceID),
			zap.String("custom_query", watch.CustomQuery),
			zap.Int("repo_filters", len(watch.Repos)))

		newIssues, err := p.service.CheckIssueWatch(ctx, watch)
		if err != nil {
			p.logger.Debug("failed to check issue watch",
				zap.String("id", watch.ID), zap.Error(err))
			continue
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
		} else if cleaned > 0 {
			p.logger.Info("cleaned up closed issue tasks",
				zap.String("watch_id", watch.ID), zap.Int("deleted", cleaned))
		}
	}
}
