package github

import (
	"context"
	"sort"
	"time"

	"go.uber.org/zap"
)

const (
	// prMonitorPendingRetryFloor is the retry interval used when local admission
	// defers without supplying a usable boundary. It is a floor, never a
	// provider deadline and never a bypass of the next admission check.
	prMonitorPendingRetryFloor = time.Second
	// prMonitorPendingMaxConsumers bounds the retained unfinished inventory.
	prMonitorPendingMaxConsumers = 4096
)

// prMonitorPendingTarget is one deduplicated provider target with every live
// watch consumer that is waiting on it. Consumers are keyed by watch lifetime so
// a retire/resume replacement is never suppressed by the retired ID.
type prMonitorPendingTarget struct {
	watches map[string]*PRWatch
	retryAt time.Time
}

// prMonitorPendingWork is the monitor's bounded record of one workspace's
// unfinished provider phases. It never stores provider results or credentials;
// the fingerprint is a non-secret identity used only to drop stale suppression.
type prMonitorPendingWork struct {
	workspaceID string
	fingerprint string
	targets     map[string]*prMonitorPendingTarget
}

// prMonitorWatchConsumerKey identifies one consumer by watch lifetime; watches
// without a persisted ID fall back to their stable target identity.
func prMonitorWatchConsumerKey(watch *PRWatch) string {
	if watch == nil {
		return ""
	}
	if watch.ID != "" {
		return watch.ID
	}
	return prWatchDiscoveryTargetKey(watch) + "\x00" + watch.SessionID + "\x00" + watch.TaskID
}

// workspaceCredentialFingerprint returns "" when identity is unknown; an
// unknown fingerprint never invalidates retained work.
func (p *Poller) workspaceCredentialFingerprint(ctx context.Context, workspaceID string) string {
	if p == nil || p.service == nil || workspaceID == "" {
		return ""
	}
	fingerprint, err := p.service.WorkspaceConnectionFingerprint(ctx, workspaceID)
	if err != nil {
		return ""
	}
	return fingerprint
}

// prunePRMonitorPending drops retained work whose credential identity changed or
// whose consumer is no longer a live watch for the same provider target. A
// removed, retired, detached, re-branched, or replaced lifetime therefore
// cannot suppress or receive a late write.
func (p *Poller) prunePRMonitorPending(ctx context.Context, watches []*PRWatch) {
	live := prMonitorLiveWatchConsumers(watches)
	fingerprints := p.pendingCredentialFingerprints(ctx, p.pendingWorkspaceIDs())
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	for workspaceID, work := range p.pending {
		if work == nil {
			delete(p.pending, workspaceID)
			continue
		}
		if prMonitorCredentialChanged(work, fingerprints[workspaceID]) {
			delete(p.pending, workspaceID)
			continue
		}
		prunePRMonitorTargetConsumers(work, live)
		if len(work.targets) == 0 {
			delete(p.pending, workspaceID)
		}
	}
}

// pendingWorkspaceIDs snapshots the retained workspaces without holding the
// pending lock across credential reads.
func (p *Poller) pendingWorkspaceIDs() []string {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	workspaceIDs := make([]string, 0, len(p.pending))
	for workspaceID := range p.pending {
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	return workspaceIDs
}

// pendingCredentialFingerprints resolves each retained workspace's current
// non-secret credential identity. Unknown identities are omitted so a failed
// lookup never invalidates work.
func (p *Poller) pendingCredentialFingerprints(
	ctx context.Context, workspaceIDs []string,
) map[string]string {
	fingerprints := make(map[string]string, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		if fingerprint := p.workspaceCredentialFingerprint(ctx, workspaceID); fingerprint != "" {
			fingerprints[workspaceID] = fingerprint
		}
	}
	return fingerprints
}

func prMonitorCredentialChanged(work *prMonitorPendingWork, current string) bool {
	return work != nil && current != "" && work.fingerprint != "" && current != work.fingerprint
}

func prMonitorLiveWatchConsumers(watches []*PRWatch) map[string]*PRWatch {
	live := make(map[string]*PRWatch, len(watches))
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		if key := prMonitorWatchConsumerKey(watch); key != "" {
			live[key] = watch
		}
	}
	return live
}

func prunePRMonitorTargetConsumers(work *prMonitorPendingWork, live map[string]*PRWatch) {
	for key, target := range work.targets {
		if target == nil {
			delete(work.targets, key)
			continue
		}
		for consumerKey := range target.watches {
			watch, ok := live[consumerKey]
			if !ok || prWatchDiscoveryTargetKey(watch) != key {
				delete(target.watches, consumerKey)
			}
		}
		if len(target.watches) == 0 {
			delete(work.targets, key)
		}
	}
}

// selectMonitorTickPRWatches removes watches that already have a future
// admission boundary. The normal tick must not create a second attempt for
// work the continuation timer will resume.
func (p *Poller) selectMonitorTickPRWatches(watches []*PRWatch) []*PRWatch {
	now := p.now()
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if len(p.pending) == 0 {
		return watches
	}
	selected := make([]*PRWatch, 0, len(watches))
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		if work := p.pending[watch.WorkspaceID]; work != nil {
			target := work.targets[prWatchDiscoveryTargetKey(watch)]
			if target != nil && target.retryAt.After(now) {
				if _, waiting := target.watches[prMonitorWatchConsumerKey(watch)]; waiting {
					continue
				}
			}
		}
		selected = append(selected, watch)
	}
	return selected
}

// recordPRMonitorPending replaces the attempted targets of each workspace with
// the unfinished work the pass reported. A new deferral always replaces the
// deadline of the target it retried, so an expired boundary cannot survive as
// the deadline of newly deferred work. Untouched targets keep their own
// deadlines.
func (p *Poller) recordPRMonitorPending(
	ctx context.Context, outcomes []prWatchWorkspaceBatchOutcome, now time.Time,
) {
	if len(outcomes) == 0 {
		return
	}
	fingerprints := make(map[string]string, len(outcomes))
	for _, outcome := range outcomes {
		if outcome.state != prWatchBatchDeferred {
			continue
		}
		if fingerprint := p.workspaceCredentialFingerprint(ctx, outcome.workspaceID); fingerprint != "" {
			fingerprints[outcome.workspaceID] = fingerprint
		}
	}
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if p.pending == nil {
		p.pending = make(map[string]*prMonitorPendingWork)
	}
	for _, outcome := range outcomes {
		work := p.pending[outcome.workspaceID]
		if work == nil {
			work = &prMonitorPendingWork{
				workspaceID: outcome.workspaceID,
				targets:     make(map[string]*prMonitorPendingTarget),
			}
			p.pending[outcome.workspaceID] = work
		}
		for _, watch := range outcome.watches {
			if watch == nil {
				continue
			}
			removePRMonitorPendingConsumer(
				work, prWatchDiscoveryTargetKey(watch), prMonitorWatchConsumerKey(watch),
			)
		}
		if next := prMonitorPendingFromOutcome(outcome, now); next != nil {
			if fingerprint := fingerprints[outcome.workspaceID]; fingerprint != "" {
				work.fingerprint = fingerprint
			}
			for key, target := range next.targets {
				mergePRMonitorPendingTarget(work, key, target)
			}
		}
		if len(work.targets) == 0 {
			delete(p.pending, outcome.workspaceID)
			continue
		}
		p.evictPRMonitorPendingLocked()
	}
}

func prMonitorPendingFromOutcome(
	outcome prWatchWorkspaceBatchOutcome, now time.Time,
) *prMonitorPendingWork {
	if outcome.state != prWatchBatchDeferred || len(outcome.unfinished) == 0 {
		return nil
	}
	retryAt, ok := prMonitorPendingBoundary(outcome, now)
	if !ok {
		return nil
	}
	targets := make(map[string]*prMonitorPendingTarget, len(outcome.unfinished))
	for _, watch := range outcome.unfinished {
		if watch == nil {
			continue
		}
		key := prWatchDiscoveryTargetKey(watch)
		target := targets[key]
		if target == nil {
			target = &prMonitorPendingTarget{watches: make(map[string]*PRWatch), retryAt: retryAt}
			targets[key] = target
		}
		target.watches[prMonitorWatchConsumerKey(watch)] = watch
	}
	if len(targets) == 0 {
		return nil
	}
	return &prMonitorPendingWork{workspaceID: outcome.workspaceID, targets: targets}
}

// removePRMonitorPendingConsumer drops only the consumer a pass attempted.
// Sibling consumers of the same provider target keep their queued boundary.
func removePRMonitorPendingConsumer(work *prMonitorPendingWork, targetKey, consumerKey string) {
	target := work.targets[targetKey]
	if target == nil {
		return
	}
	delete(target.watches, consumerKey)
	if len(target.watches) == 0 {
		delete(work.targets, targetKey)
	}
}

// mergePRMonitorPendingTarget keeps consumers that were not part of the retried
// pass and retains the earliest boundary, because one provider read serves every
// consumer of the same target.
func mergePRMonitorPendingTarget(
	work *prMonitorPendingWork, key string, next *prMonitorPendingTarget,
) {
	existing := work.targets[key]
	if existing == nil {
		work.targets[key] = next
		return
	}
	for consumerKey, watch := range next.watches {
		existing.watches[consumerKey] = watch
	}
	if existing.retryAt.IsZero() || (!next.retryAt.IsZero() && next.retryAt.Before(existing.retryAt)) {
		existing.retryAt = next.retryAt
	}
}

// prMonitorPendingBoundary prefers the deferral's next authorization boundary,
// then its positive delay, then the retry floor. A provider deadline is kept as
// a lower bound and is never replaced by the floor.
func prMonitorPendingBoundary(outcome prWatchWorkspaceBatchOutcome, now time.Time) (time.Time, bool) {
	if deferred := outcome.deferred; deferred != nil {
		if deferred.RetryAt.After(now) {
			return deferred.RetryAt.UTC(), true
		}
		if deferred.Delay > 0 {
			return now.Add(deferred.Delay).UTC(), true
		}
		return now.Add(prMonitorPendingRetryFloor), true
	}
	if retryAt := prDiscoveryRetryAtFromError(outcome.err); retryAt != nil && retryAt.After(now) {
		return retryAt.UTC(), true
	}
	return time.Time{}, false
}

func (p *Poller) evictPRMonitorPendingLocked() {
	total := 0
	for _, work := range p.pending {
		total += prMonitorPendingConsumerCount(work)
	}
	for total > prMonitorPendingMaxConsumers {
		oldestID := ""
		var oldest time.Time
		for workspaceID, work := range p.pending {
			at := prMonitorPendingWorkDeadline(work)
			if oldestID == "" || at.Before(oldest) {
				oldestID, oldest = workspaceID, at
			}
		}
		if oldestID == "" {
			return
		}
		total -= prMonitorPendingConsumerCount(p.pending[oldestID])
		delete(p.pending, oldestID)
	}
}

func prMonitorPendingConsumerCount(work *prMonitorPendingWork) int {
	if work == nil {
		return 0
	}
	total := 0
	for _, target := range work.targets {
		if target != nil {
			total += len(target.watches)
		}
	}
	return total
}

func prMonitorPendingWorkDeadline(work *prMonitorPendingWork) time.Time {
	if work == nil {
		return time.Time{}
	}
	var earliest time.Time
	for _, target := range work.targets {
		if target == nil || target.retryAt.IsZero() {
			continue
		}
		if earliest.IsZero() || target.retryAt.Before(earliest) {
			earliest = target.retryAt
		}
	}
	return earliest
}

// prMonitorPendingDeadline reports the earliest retained resumption boundary so
// the monitor loop can arm one timer.
func (p *Poller) prMonitorPendingDeadline() (time.Time, bool) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	var earliest time.Time
	for _, work := range p.pending {
		if deadline := prMonitorPendingWorkDeadline(work); !deadline.IsZero() {
			if earliest.IsZero() || deadline.Before(earliest) {
				earliest = deadline
			}
		}
	}
	return earliest, !earliest.IsZero()
}

// takeDuePRMonitorPending returns due targets in rotating workspace order.
// Targets whose own boundary is still in the future stay queued. Entries stay in
// place until the resumed pass records their new state.
func (p *Poller) takeDuePRMonitorPending(now time.Time) map[string][]*PRWatch {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	if len(p.pending) == 0 {
		return nil
	}
	workspaceIDs := make([]string, 0, len(p.pending))
	for workspaceID, work := range p.pending {
		if prMonitorPendingDueWatches(work, now) != nil {
			workspaceIDs = append(workspaceIDs, workspaceID)
		}
	}
	if len(workspaceIDs) == 0 {
		return nil
	}
	sort.Strings(workspaceIDs)
	start := p.pendingCursor % len(workspaceIDs)
	p.pendingCursor = (start + 1) % len(workspaceIDs)
	due := make(map[string][]*PRWatch, len(workspaceIDs))
	for offset := range workspaceIDs {
		work := p.pending[workspaceIDs[(start+offset)%len(workspaceIDs)]]
		if watches := prMonitorPendingDueWatches(work, now); len(watches) > 0 {
			due[work.workspaceID] = watches
		}
	}
	return due
}

// prMonitorPendingDueWatches lists every consumer of due targets, sorted for a
// stable wake order. Each consumer appears once.
func prMonitorPendingDueWatches(work *prMonitorPendingWork, now time.Time) []*PRWatch {
	if work == nil {
		return nil
	}
	targetKeys := make([]string, 0, len(work.targets))
	for key, target := range work.targets {
		if target != nil && !target.retryAt.After(now) {
			targetKeys = append(targetKeys, key)
		}
	}
	if len(targetKeys) == 0 {
		return nil
	}
	sort.Strings(targetKeys)
	consumerKeys := make([]string, 0, len(targetKeys))
	byConsumer := make(map[string]*PRWatch)
	for _, key := range targetKeys {
		for consumerKey, watch := range work.targets[key].watches {
			if watch == nil {
				continue
			}
			if _, exists := byConsumer[consumerKey]; !exists {
				byConsumer[consumerKey] = watch
				consumerKeys = append(consumerKeys, consumerKey)
			}
		}
	}
	sort.Strings(consumerKeys)
	watches := make([]*PRWatch, 0, len(consumerKeys))
	for _, consumerKey := range consumerKeys {
		watches = append(watches, byConsumer[consumerKey])
	}
	return watches
}

// resumePRMonitorPending retries due unfinished work through the normal batched
// seam. It never runs the transport fallback, so a continuation wake cannot
// replenish a poll cycle's fallback budget.
func (p *Poller) resumePRMonitorPending(ctx context.Context) {
	p.refreshPRMonitorPendingTargets(ctx)
	now := p.now()
	due := p.takeDuePRMonitorPending(now)
	if len(due) == 0 {
		return
	}
	eligible := make(map[string][]*PRWatch, len(due))
	for workspaceID, watches := range due {
		remaining := p.filterOpenCircuitWatches(ctx, watches)
		if len(remaining) == 0 {
			p.deferPRMonitorPendingWorkspace(workspaceID, now.Add(defaultPRPollInterval), now)
			continue
		}
		eligible[workspaceID] = remaining
	}
	if len(eligible) == 0 {
		return
	}
	outcomes := p.runBatchedPRWatchCheck(ctx, eligible)
	p.recordPRMonitorPending(ctx, outcomes, p.now())
	p.logger.Debug("resumed deferred PR monitoring", zap.Int("workspaces", len(outcomes)))
}

// refreshPRMonitorPendingTargets revalidates retained consumers against the
// active inventory before a wake, so a re-branched or replaced watch is not
// queried for its old target. Credential identity is checked on the same pass;
// a failed inventory read falls back to that check alone.
func (p *Poller) refreshPRMonitorPendingTargets(ctx context.Context) {
	watches, err := p.service.ListActivePRWatches(ctx)
	if err != nil {
		p.logger.Debug("failed to list PR watches before continuation", zap.Error(err))
		p.pruneStalePendingCredentials(ctx)
		return
	}
	p.prunePRMonitorPending(ctx, watches)
}

// pruneStalePendingCredentials drops entries whose workspace credential identity
// changed since the deferral. The new principal's work is admitted by the next
// regular attempt instead of waiting for the previous principal's boundary.
func (p *Poller) pruneStalePendingCredentials(ctx context.Context) {
	workspaceIDs := p.pendingWorkspaceIDs()
	if len(workspaceIDs) == 0 {
		return
	}
	fingerprints := p.pendingCredentialFingerprints(ctx, workspaceIDs)
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	for workspaceID, current := range fingerprints {
		if prMonitorCredentialChanged(p.pending[workspaceID], current) {
			delete(p.pending, workspaceID)
		}
	}
}

func (p *Poller) deferPRMonitorPendingWorkspace(workspaceID string, retryAt, now time.Time) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	work := p.pending[workspaceID]
	if work == nil {
		return
	}
	for _, target := range work.targets {
		if target != nil && !target.retryAt.After(now) {
			target.retryAt = retryAt
		}
	}
}

func (p *Poller) clearPRMonitorPending() {
	p.pendingMu.Lock()
	p.pending = nil
	p.pendingCursor = 0
	p.pendingMu.Unlock()
}
