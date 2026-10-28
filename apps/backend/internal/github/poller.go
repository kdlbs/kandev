package github

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const (
	defaultPRPollInterval          = 1 * time.Minute
	defaultWatchPollInterval       = 5 * time.Minute
	prWatchFallbackWorkspaceBudget = 5
	prWatchFallbackCycleBudget     = 10
)

// TaskBranchInfo describes a task+session that may need a PR watch.
// RepositoryID scopes multi-repo tasks: each repo is a separate watch row.
// Empty for legacy single-repo tasks.
type TaskBranchInfo struct {
	WorkspaceID  string
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
	// ResolveBranchForWatch resolves a branch within the watch's source session and repository.
	// An empty result leaves the watch unchanged.
	ResolveBranchForWatch(ctx context.Context, watch *PRWatch) string
}

type multiBranchTaskBranchProvider interface {
	ResolveBranchesForRepository(ctx context.Context, taskID, repositoryID string) []string
}

type repositoryTaskBranchProvider interface {
	ResolveBranchForRepository(ctx context.Context, taskID, repositoryID string) string
}

// Poller runs background loops for PR monitoring and review queue checking.
type Poller struct {
	service              *Service
	eventBus             bus.EventBus
	logger               *logger.Logger
	taskBranchProvider   TaskBranchProvider
	taskActivityProvider TaskActivityProvider
	clock                func() time.Time

	// circuits tracks per-workspace auth/config backoff for the PR-monitor
	// loop only (see poller_circuit.go). Safe for concurrent use; never nil.
	circuits              *pollerCircuits
	reviewCleanupCircuits *reviewCleanupCircuits

	batchMu                 sync.Mutex
	batchWorkspaceCursor    int
	fallbackMu              sync.Mutex
	fallbackWorkspaceCursor int
	fallbackTargetCursors   map[string]int
	pendingMu               sync.Mutex
	pending                 map[string]*prMonitorPendingWork
	pendingCursor           int

	cancel       context.CancelFunc
	watchQueueMu sync.Mutex
	watchCursor  int
	wg           sync.WaitGroup
	started      bool
}

// NewPoller creates a new background poller.
func NewPoller(svc *Service, eventBus bus.EventBus, log *logger.Logger) *Poller {
	return &Poller{
		service:               svc,
		eventBus:              eventBus,
		logger:                log,
		circuits:              newPollerCircuits(),
		reviewCleanupCircuits: newReviewCleanupCircuits(),
		fallbackTargetCursors: make(map[string]int),
		pending:               make(map[string]*prMonitorPendingWork),
		clock:                 time.Now,
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
	ctx = pollerContext(ctx)

	p.wg.Add(2)
	go p.prMonitorLoop(ctx)
	go p.watchQueueLoop(ctx)

	p.logger.Info("GitHub poller started")
}

func pollerContext(ctx context.Context) context.Context {
	return WithNonBlockingGitHubAdmission(
		WithGitHubWorkClass(ctx, WorkClassBackground),
	)
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
	p.service.clearWatchFilterProgress("")
	p.clearPRMonitorPending()
	p.started = false
	p.logger.Info("GitHub poller stopped")
}

// prMonitorLoop polls PR watches for new feedback. One minute is the inventory
// cadence; unfinished provider phases resume through the monitor-owned timer at
// their own admission boundary.
func (p *Poller) prMonitorLoop(ctx context.Context) {
	defer p.wg.Done()

	// Run an initial check immediately so existing watches are evaluated on startup.
	p.checkPRWatches(ctx)

	ticker := time.NewTicker(defaultPRPollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		var retryC <-chan time.Time
		if deadline, ok := p.prMonitorPendingDeadline(); ok {
			resetPollerTimer(timer, deadline.Sub(p.now()))
			retryC = timer.C
		}
		select {
		case <-ctx.Done():
			p.clearPRMonitorPending()
			return
		case <-ticker.C:
			p.checkPRWatches(ctx)
		case <-retryC:
			p.resumePRMonitorPending(ctx)
		}
	}
}

// resetPollerTimer re-arms a reusable timer without leaking a stale tick.
func resetPollerTimer(timer *time.Timer, wait time.Duration) {
	if wait < 0 {
		wait = 0
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(wait)
}

func (p *Poller) checkPRWatches(ctx context.Context) {
	p.reconcileWatches(ctx)

	watches, err := p.service.ListActivePRWatches(ctx)
	if err != nil {
		p.logger.Error("failed to list PR watches", zap.Error(err))
		return
	}
	if cardinality, metricErr := p.service.PRWatchCardinality(ctx); metricErr != nil {
		p.logger.Debug("failed to observe PR watch cardinality", zap.Error(metricErr))
	} else {
		recordPRWatchCardinality(cardinality)
	}
	p.prunePRMonitorPending(ctx, watches)
	if len(watches) == 0 {
		return
	}

	watches = p.selectMonitorTickPRWatches(watches)
	watches = p.filterOpenCircuitWatches(ctx, watches)
	if len(watches) == 0 {
		return
	}
	watches = p.selectDuePRWatches(ctx, watches)
	if len(watches) == 0 {
		return
	}

	// Try the batched GraphQL path first. Each workspace reports whether its
	// watches completed, were deferred by a provider gate, or may use the
	// bounded REST fallback. A failed workspace must not replay successful
	// batches from its siblings, and completed phases stay applied.
	outcomes := p.tryBatchedPRWatchCheck(ctx, watches)
	p.recordPRMonitorPending(ctx, outcomes, p.now())
	p.runPRWatchFallback(ctx, fallbackEligiblePRWatches(outcomes))
}

// filterOpenCircuitWatches refreshes each present workspace's credential
// fingerprint at most once per cycle (resetting an open circuit on
// rotate/reconnect/re-auth) and excludes every watch belonging to a
// workspace whose circuit is still open afterward — so a broken automation
// credential does not cost a GitHub call every poll cycle, only one
// exploratory probe per backoff window (see authcircuit.PermanentBackoff).
func (p *Poller) filterOpenCircuitWatches(ctx context.Context, watches []*PRWatch) []*PRWatch {
	now := time.Now().UTC()
	refreshed := make(map[string]bool, len(watches))
	filtered := make([]*PRWatch, 0, len(watches))
	for _, watch := range watches {
		if watch == nil || watch.WorkspaceID == "" {
			filtered = append(filtered, watch)
			continue
		}
		if !refreshed[watch.WorkspaceID] {
			refreshed[watch.WorkspaceID] = true
			p.refreshWorkspaceCircuitFingerprint(ctx, watch.WorkspaceID)
		}
		if p.circuits.open(watch.WorkspaceID, now) {
			incAuthCircuitSkip()
			continue
		}
		filtered = append(filtered, watch)
	}
	return filtered
}

// refreshWorkspaceCircuitFingerprint resets workspaceID's circuit when its
// GitHub connection's non-secret credential fingerprint has
// changed since last observed. A lookup error or empty fingerprint (no
// connection, or the connection was just deleted) is treated as "unknown"
// and never resets an open circuit — see
// authcircuit.State.ResetIfFingerprintChanged.
func (p *Poller) refreshWorkspaceCircuitFingerprint(ctx context.Context, workspaceID string) {
	if p.service == nil {
		return
	}
	fingerprint, err := p.service.WorkspaceConnectionFingerprint(ctx, workspaceID)
	if err != nil || fingerprint == "" {
		return
	}
	if p.circuits.resetIfFingerprintChanged(workspaceID, fingerprint) {
		incAuthCircuitReset()
	}
}

type prWatchBatchOutcomeState string

const (
	prWatchBatchCompleted        prWatchBatchOutcomeState = "completed"
	prWatchBatchDeferred         prWatchBatchOutcomeState = "deferred"
	prWatchBatchFallbackEligible prWatchBatchOutcomeState = "fallback_eligible"
)

type prWatchWorkspaceBatchOutcome struct {
	workspaceID string
	watches     []*PRWatch
	// unfinished lists the watches whose provider phase did not complete, so
	// only they can enter transport fallback or wait for admission.
	unfinished []*PRWatch
	state      prWatchBatchOutcomeState
	results    []PRWatchSyncResult
	deferred   *AdmissionDeferredError
	err        error
}

// tryBatchedPRWatchCheck runs the batched GraphQL flow for the supplied
// watches via the shared Service sync seam. It returns one outcome per
// workspace so a fallback can be bounded and scoped.
// The poller's only extra responsibility on top of the service apply is
// publishing PRFeedback events for status changes and merge/close events
// (the on-demand sync path intentionally doesn't publish these).
func (p *Poller) tryBatchedPRWatchCheck(ctx context.Context, watches []*PRWatch) []prWatchWorkspaceBatchOutcome {
	byWorkspace := make(map[string][]*PRWatch)
	for _, watch := range watches {
		if watch == nil || watch.WorkspaceID == "" {
			p.logger.Warn("PR watch is missing workspace ownership", zap.String("watch_id", watchID(watch)))
			continue
		}
		byWorkspace[watch.WorkspaceID] = append(byWorkspace[watch.WorkspaceID], watch)
	}
	if len(byWorkspace) == 0 {
		return nil
	}
	return p.runBatchedPRWatchCheck(ctx, byWorkspace)
}

// runBatchedPRWatchCheck runs one batched pass per workspace, rotating the
// first workspace so a blocked principal cannot starve its siblings.
func (p *Poller) runBatchedPRWatchCheck(
	ctx context.Context, byWorkspace map[string][]*PRWatch,
) []prWatchWorkspaceBatchOutcome {
	workspaceIDs := make([]string, 0, len(byWorkspace))
	for workspaceID := range byWorkspace {
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	sort.Strings(workspaceIDs)
	if len(workspaceIDs) == 0 {
		return nil
	}
	p.batchMu.Lock()
	start := p.batchWorkspaceCursor % len(workspaceIDs)
	p.batchWorkspaceCursor = (start + 1) % len(workspaceIDs)
	p.batchMu.Unlock()
	outcomes := make([]prWatchWorkspaceBatchOutcome, 0, len(workspaceIDs))
	for i := range workspaceIDs {
		workspaceID := workspaceIDs[(start+i)%len(workspaceIDs)]
		outcomes = append(outcomes, p.checkWorkspacePRWatches(ctx, workspaceID, byWorkspace[workspaceID]))
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			break
		}
	}
	return outcomes
}

func (p *Poller) checkWorkspacePRWatches(
	ctx context.Context, workspaceID string, watches []*PRWatch,
) prWatchWorkspaceBatchOutcome {
	incCanonicalPollRequests(len(watches))
	syncOutcome := p.service.syncWorkspaceWatchesBatchedForPoller(ctx, workspaceID, watches)
	// Completed phases are written and published before any unfinished phase is
	// classified, so a later deferral cannot withhold a finished update.
	p.publishBatchedPRWatchResults(ctx, syncOutcome.Results)
	outcome := prWatchWorkspaceBatchOutcome{
		workspaceID: workspaceID,
		watches:     watches,
		unfinished:  prWatchPendingWatches(syncOutcome.Pending),
		results:     syncOutcome.Results,
		deferred:    syncOutcome.Deferred,
		err:         syncOutcome.Err,
	}
	if errors.Is(syncOutcome.Err, errGraphQLUnsupported) {
		// A client without GraphQL support deliberately falls back to the
		// per-watch REST path. It is not an authentication failure, so it must
		// not open the workspace circuit.
		outcome.unfinished = watches
		outcome.state = prWatchBatchFallbackEligible
		return outcome
	}
	// A completed pass clears any earlier failure backoff, while a local
	// deferral is not a provider failure and other errors keep their class.
	// Recording it here is what lets a successful probe reset the schedule.
	p.circuits.recordError(workspaceID, syncOutcome.Err, time.Now().UTC())
	if syncOutcome.Err == nil && len(outcome.unfinished) == 0 {
		outcome.state = prWatchBatchCompleted
		return outcome
	}
	p.logger.Debug("batched PR watch check incomplete",
		zap.String("workspace_id", workspaceID),
		zap.Bool("deferred", syncOutcome.Deferred != nil),
		zap.Error(syncOutcome.Err))
	if len(outcome.unfinished) == 0 {
		outcome.unfinished = watches
	}
	outcome.state = prWatchBatchDeferred
	if syncOutcome.Deferred == nil && prWatchBatchFallbackAllowed(syncOutcome.Err) {
		outcome.state = prWatchBatchFallbackEligible
	}
	return outcome
}

func (p *Poller) publishBatchedPRWatchResults(ctx context.Context, results []PRWatchSyncResult) {
	for _, r := range results {
		if r.FeedbackPublished {
			continue
		}
		if !r.Found || r.Status == nil {
			continue
		}
		// Skip publish when the underlying SyncTaskPR failed — the task_pr
		// row is still stale, so emitting a "PR merged" / "checks changed"
		// event would put the frontend ahead of the DB until the next poll.
		if r.SyncFailed {
			continue
		}
		// Publish PRFeedback event when the watch was previously numbered and
		// either the PR transitioned to merged/closed or check/review state
		// changed. Searching watches (PRNumber==0) that just got promoted
		// don't fire a feedback event — they get an associate-event via
		// SyncTaskPR / AssociatePRWithTask inside the service apply.
		if r.Watch.PRNumber == 0 {
			continue
		}
		effectiveTaskID := p.service.reconcileTaskPROwnership(
			ctx, r.Watch.SessionID, r.Watch.TaskID, r.Watch.RepositoryID, r.Watch.PRNumber,
		)
		pr := r.Status.PR
		if pr != nil && (pr.State == prStateMerged || pr.State == prStateClosed) {
			p.publishPRStatusEvent(ctx, r.Watch, effectiveTaskID, r.Status)
			continue
		}
		if r.Changed {
			p.publishPRStatusEvent(ctx, r.Watch, effectiveTaskID, r.Status)
		}
	}
}

func fallbackEligiblePRWatches(outcomes []prWatchWorkspaceBatchOutcome) []*PRWatch {
	var watches []*PRWatch
	for _, outcome := range outcomes {
		if outcome.state != prWatchBatchFallbackEligible {
			continue
		}
		if len(outcome.unfinished) > 0 {
			watches = append(watches, outcome.unfinished...)
			continue
		}
		watches = append(watches, outcome.watches...)
	}
	return watches
}

func prWatchBatchFallbackAllowed(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Local admission deferral is not a provider observation. Retrying it over
	// REST would spend the same blocked budget and diagnose a healthy client.
	if admissionDeferralOf(err) != nil {
		return false
	}
	category := classifyPRDiscoveryError(err)
	if category == PRDiscoveryHealthRateLimited || category == PRDiscoveryHealthInvalidQuery {
		return false
	}
	switch classifyPollErr(err) {
	case authcircuit.FailureClassAuth, authcircuit.FailureClassConfig:
		return false
	default:
		return true
	}
}

func (p *Poller) runPRWatchFallback(ctx context.Context, watches []*PRWatch) {
	selected := p.selectPRWatchFallbackTargets(watches)
	groups := groupPRWatchFallbackTargets(watches)
	stopped := make(map[string]bool)
	for _, watch := range selected {
		if ctx.Err() != nil {
			return
		}
		if watch == nil || stopped[watch.WorkspaceID] {
			continue
		}
		err := p.checkPRWatchGroupWithError(ctx, groups[prWatchFallbackTargetKey(watch)])
		if err == nil {
			continue
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if prWatchFallbackShouldStopWorkspace(err) {
			stopped[watch.WorkspaceID] = true
		}
	}
}

func groupPRWatchFallbackTargets(watches []*PRWatch) map[string][]*PRWatch {
	groups := make(map[string][]*PRWatch, len(watches))
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		key := prWatchFallbackTargetKey(watch)
		groups[key] = append(groups[key], watch)
	}
	return groups
}

func deduplicatePRWatchFallbackTargets(watches []*PRWatch) (map[string][]*PRWatch, []string) {
	byWorkspace := make(map[string]map[string]*PRWatch)
	for _, watch := range watches {
		if watch != nil {
			workspaceTargets := byWorkspace[watch.WorkspaceID]
			if workspaceTargets == nil {
				workspaceTargets = make(map[string]*PRWatch)
				byWorkspace[watch.WorkspaceID] = workspaceTargets
			}
			key := prWatchFallbackTargetKey(watch)
			if _, exists := workspaceTargets[key]; !exists {
				workspaceTargets[key] = watch
			}
		}
	}
	workspaceIDs := make([]string, 0, len(byWorkspace))
	workspaceGroups := make(map[string][]*PRWatch, len(byWorkspace))
	for workspaceID, targetWatches := range byWorkspace {
		workspaceWatches := make([]*PRWatch, 0, len(targetWatches))
		for _, watch := range targetWatches {
			workspaceWatches = append(workspaceWatches, watch)
		}
		sort.SliceStable(workspaceWatches, func(i, j int) bool {
			return prWatchFallbackTargetKey(workspaceWatches[i]) < prWatchFallbackTargetKey(workspaceWatches[j])
		})
		workspaceGroups[workspaceID] = workspaceWatches
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	sort.Strings(workspaceIDs)
	return workspaceGroups, workspaceIDs
}

func (p *Poller) selectPRWatchFallbackTargets(watches []*PRWatch) []*PRWatch {
	workspaceGroups, workspaceIDs := deduplicatePRWatchFallbackTargets(watches)
	if len(workspaceIDs) == 0 {
		return nil
	}

	p.fallbackMu.Lock()
	defer p.fallbackMu.Unlock()
	if p.fallbackTargetCursors == nil {
		p.fallbackTargetCursors = make(map[string]int)
	}
	start := p.fallbackWorkspaceCursor % len(workspaceIDs)
	selected := make([]*PRWatch, 0, minInt(prWatchFallbackCycleBudget, len(watches)))
	selectedByWorkspace := make(map[string]int, len(workspaceIDs))
	for len(selected) < prWatchFallbackCycleBudget {
		added := false
		for offset := 0; offset < len(workspaceIDs) && len(selected) < prWatchFallbackCycleBudget; offset++ {
			workspaceID := workspaceIDs[(start+offset)%len(workspaceIDs)]
			group := workspaceGroups[workspaceID]
			count := selectedByWorkspace[workspaceID]
			if count >= prWatchFallbackWorkspaceBudget || count >= len(group) {
				continue
			}
			cursor := p.fallbackTargetCursors[workspaceID] % len(group)
			selected = append(selected, group[(cursor+count)%len(group)])
			selectedByWorkspace[workspaceID] = count + 1
			added = true
		}
		if !added {
			break
		}
	}
	for workspaceID, count := range selectedByWorkspace {
		group := workspaceGroups[workspaceID]
		cursor := p.fallbackTargetCursors[workspaceID] % len(group)
		p.fallbackTargetCursors[workspaceID] = (cursor + count) % len(group)
	}
	p.fallbackWorkspaceCursor = (start + 1) % len(workspaceIDs)
	return selected
}

func prWatchFallbackTargetKey(watch *PRWatch) string {
	if watch == nil {
		return ""
	}
	return watch.WorkspaceID + "\x00" + defaultAuthFlowHost(watch.GitHubHost) + "\x00" + watch.Owner + "\x00" + watch.Repo + "\x00" + watch.Branch + "\x00" + fmt.Sprintf("%010d", watch.PRNumber)
}

func prWatchFallbackShouldStopWorkspace(err error) bool {
	if err == nil {
		return false
	}
	// Local admission deferral leaves the remaining targets equally deferred;
	// stop this workspace's probes and let the monitor resume at the boundary.
	if admissionDeferralOf(err) != nil {
		return true
	}
	category := classifyPRDiscoveryError(err)
	if category == PRDiscoveryHealthRateLimited || category == PRDiscoveryHealthInvalidQuery {
		return true
	}
	switch classifyPollErr(err) {
	case authcircuit.FailureClassAuth, authcircuit.FailureClassConfig:
		return true
	default:
		return false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func watchID(watch *PRWatch) string {
	if watch == nil {
		return ""
	}
	return watch.ID
}

func (p *Poller) checkSinglePRWatch(ctx context.Context, watch *PRWatch) {
	_ = p.checkSinglePRWatchWithError(ctx, watch)
}

func (p *Poller) checkSinglePRWatchWithError(ctx context.Context, watch *PRWatch) error {
	incCanonicalPollRequests(1)
	// PRWatch with pr_number=0 means we're still searching for a PR on this branch.
	if watch.PRNumber == 0 {
		return p.detectPRForWatchWithError(ctx, watch)
	}

	status, _, err := p.service.CheckPRWatchForWorkspace(ctx, watch)
	p.circuits.recordError(watch.WorkspaceID, err, time.Now().UTC())
	if err != nil {
		p.logger.Debug("failed to check PR watch",
			zap.String("id", watch.ID), zap.Error(err))
		return err
	}
	if status == nil {
		return nil
	}
	return p.applyPRStatusForWatch(ctx, watch, status)
}

func (p *Poller) applyPRStatusForWatch(ctx context.Context, watch *PRWatch, status *PRStatus) error {
	if watch == nil || status == nil {
		return nil
	}
	if status.operationFence != nil {
		ctx = context.WithValue(ctx, githubOperationFenceKey{}, *status.operationFence)
	}
	var unlock func()
	var lockErr error
	ctx, unlock, lockErr = p.service.lockResourceMutation(ctx, watch.WorkspaceID)
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	if err := p.service.validatePRWatchHost(ctx, watch.WorkspaceID, watch); err != nil {
		return err
	}

	_, _, err := p.service.applyPRWatchStatusResult(ctx, watch, status, time.Now().UTC(), true)
	if errors.Is(err, ErrPRWatchRetired) || errors.Is(err, ErrPRWatchNotFound) {
		return nil
	}
	if err != nil {
		p.logger.Error("failed to apply PR watch result", zap.String("id", watch.ID), zap.Error(err))
	}
	return nil
}

func (p *Poller) checkPRWatchGroupWithError(ctx context.Context, watches []*PRWatch) error {
	if len(watches) == 0 || watches[0] == nil {
		return nil
	}
	representative := watches[0]
	incCanonicalPollRequests(1)
	if representative.PRNumber == 0 {
		pr, err := p.service.findPRByBranchForWatch(ctx, representative)
		p.circuits.recordError(representative.WorkspaceID, err, time.Now().UTC())
		if err != nil {
			p.logger.Debug("failed to search for shared PR watch target",
				zap.String("watch_id", representative.ID), zap.Error(err))
			return err
		}
		for _, watch := range watches {
			p.applyDetectedPR(ctx, watch, pr)
		}
		return nil
	}

	status, _, err := p.service.CheckPRWatchForWorkspace(ctx, representative)
	p.circuits.recordError(representative.WorkspaceID, err, time.Now().UTC())
	if err != nil {
		p.logger.Debug("failed to check shared PR watch target",
			zap.String("watch_id", representative.ID), zap.Error(err))
		return err
	}
	if status == nil {
		return nil
	}
	for _, watch := range watches {
		if err := p.applyPRStatusForWatch(ctx, watch, status); err != nil {
			return err
		}
	}
	return nil
}

// detectPRForWatch searches GitHub for a PR on the watch's branch.
// If found, updates the watch with the PR number and creates the TaskPR association.
func (p *Poller) detectPRForWatch(ctx context.Context, watch *PRWatch) {
	_ = p.detectPRForWatchWithError(ctx, watch)
}

func (p *Poller) detectPRForWatchWithError(ctx context.Context, watch *PRWatch) error {
	if watch == nil || watch.WorkspaceID == "" {
		p.logger.Warn("cannot detect PR for watch without workspace ownership",
			zap.String("watch_id", watchID(watch)))
		return ErrGitHubWorkspaceRequired
	}

	pr, err := p.service.findPRByBranchForWatch(ctx, watch)
	p.circuits.recordError(watch.WorkspaceID, err, time.Now().UTC())
	if err != nil {
		p.logger.Debug("failed to search for PR by branch",
			zap.String("watch_id", watch.ID),
			zap.String("branch", watch.Branch),
			zap.Error(err))
		return err
	}
	p.applyDetectedPR(ctx, watch, pr)
	return nil
}

func (p *Poller) applyDetectedPR(ctx context.Context, watch *PRWatch, pr *PR) {
	if pr != nil && pr.operationFence != nil {
		ctx = context.WithValue(ctx, githubOperationFenceKey{}, *pr.operationFence)
	}
	var unlock func()
	var lockErr error
	ctx, unlock, lockErr = p.service.lockResourceMutation(ctx, watch.WorkspaceID)
	if lockErr != nil {
		return
	}
	defer unlock()
	if err := p.service.validatePRWatchHost(ctx, watch.WorkspaceID, watch); err != nil {
		return
	}

	_, promoted, err := p.service.applyPRWatchDiscoveryResult(ctx, watch, pr, time.Now().UTC())
	if err != nil {
		if !errors.Is(err, ErrPRWatchRetired) && !errors.Is(err, ErrPRWatchNotFound) {
			p.logger.Error("failed to apply detected PR result", zap.String("watch_id", watch.ID), zap.Error(err))
		}
		return
	}
	if promoted {
		p.logger.Info("detected PR for session branch",
			zap.String("watch_id", watch.ID), zap.String("branch", watch.Branch), zap.Int("pr_number", pr.Number))
	}
}

func (p *Poller) publishPRStatusEvent(ctx context.Context, watch *PRWatch, taskID string, status *PRStatus) {
	if p.eventBus == nil {
		return
	}
	evt := &PRFeedbackEvent{
		SessionID:      watch.SessionID,
		TaskID:         taskID,
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

// SetTaskActivityProvider wires the persisted task activity projection used
// by adaptive searching-watch admission.
func (p *Poller) SetTaskActivityProvider(provider TaskActivityProvider) {
	p.taskActivityProvider = provider
}

// SetClock installs the scheduler clock. Production uses time.Now; tests can
// use a fixed clock to prove interval boundaries without sleeping.
func (p *Poller) SetClock(clock func() time.Time) {
	if clock == nil {
		p.clock = time.Now
		return
	}
	p.clock = clock
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
		if _, ensureErr := p.service.EnsurePRWatchForWorkspace(
			ctx, task.WorkspaceID, task.SessionID, task.TaskID, task.RepositoryID, task.Owner, task.Repo, task.Branch,
		); ensureErr != nil && !errors.Is(ensureErr, ErrPRWatchRetired) {
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
		if branchSetPreservesWatch(ctx, p.taskBranchProvider, watch) {
			continue
		}
		currentBranch := ""
		if repositoryProvider, ok := p.taskBranchProvider.(repositoryTaskBranchProvider); ok {
			currentBranch = repositoryProvider.ResolveBranchForRepository(ctx, watch.TaskID, watch.RepositoryID)
		} else {
			currentBranch = p.taskBranchProvider.ResolveBranchForWatch(ctx, watch)
		}
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

func branchSetPreservesWatch(ctx context.Context, provider TaskBranchProvider, watch *PRWatch) bool {
	multiBranchProvider, ok := provider.(multiBranchTaskBranchProvider)
	if !ok {
		return false
	}
	branches := multiBranchProvider.ResolveBranchesForRepository(ctx, watch.TaskID, watch.RepositoryID)
	if len(branches) == 0 {
		return false
	}
	return branchListContains(branches, watch.Branch)
}

func branchListContains(branches []string, branch string) bool {
	for _, candidate := range branches {
		if candidate == branch {
			return true
		}
	}
	return false
}

// logCleanupError logs a poller cleanup failure at WARN, except when the error
// is a context cancellation. That happens when the poll cycle is interrupted by
// backend shutdown, which is expected teardown rather than a fault, so it is
// downgraded to DEBUG to keep shutdown logs quiet.
func (p *Poller) logCleanupError(msg string, err error, fields ...zap.Field) {
	fields = append(fields, zap.Error(err))
	if errors.Is(err, context.Canceled) {
		p.logger.Debug(msg+" (context canceled during shutdown)", fields...)
		return
	}
	p.logger.Warn(msg, fields...)
}
