package github

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PRWatchSyncResult is the per-watch outcome from SyncWatchesBatched.
// Callers (poller, on-demand sync) can post-process — e.g. publish
// PRFeedback events — without re-fetching from GitHub.
type PRWatchSyncResult struct {
	Watch             *PRWatch
	Status            *PRStatus // nil when no PR was found for a searching watch, or alias missing
	Found             bool      // true when status applied (PR data exists)
	Changed           bool      // numbered watches only: true if checks/review state moved
	SyncFailed        bool      // true when SyncTaskPR returned an error — callers must NOT publish events
	FeedbackPublished bool      // true when feedback was published under the result fence
	Applied           bool      // false when the watch lifetime was retired or replaced
	// DiscoveryResolved indicates that the provider lookup completed, even if
	// a later database write failed. It keeps health admission focused on
	// provider evidence rather than task persistence.
	DiscoveryResolved  bool
	DiscoveryTrackable bool
}

const prSyncExplicitRefreshKeySuffix = "|explicit-refresh"

// prWatchSyncPending is one provider phase that did not complete during a
// batched sync pass. The watches keep their target identities and PR numbers,
// so a caller can resume exactly that phase without repeating finished work.
type prWatchSyncPending struct {
	Numbered bool
	Watches  []*PRWatch
}

// prWatchSyncOutcome is the internal result of one batched sync pass.
// Results holds writes that completed, Pending holds work that still needs the
// provider, Deferred marks local admission deferral, and Err the failure that
// left that work pending. Existing exported service methods adapt this outcome
// without changing their HTTP or WebSocket response shapes.
type prWatchSyncOutcome struct {
	Results  []PRWatchSyncResult
	Pending  []prWatchSyncPending
	Deferred *AdmissionDeferredError
	Err      error
}

// SyncWatchesBatched runs the batched GraphQL queries for the supplied
// watches and applies the resulting DB updates: timestamps, task PR sync,
// watch PR-number promotion on detection, watch reset on merge/close.
// Returns per-watch results so callers can post-process (event publishing).
//
// Returns an error when a batched phase fails — the caller should fall back to
// per-watch checks for the unfinished work rather than silently dropping a
// poll cycle. Completed phases are applied before this error is returned.
// Errors from the per-watch DB applies are logged but do not abort the
// loop, matching the per-watch path's best-effort semantics.
//
// This is the single seam both the 1-minute poller and the on-demand
// TriggerPRSyncAll / ListWorkspaceTaskPRs background refresh share, so a
// 40-watch workspace fans out to ~2 gh subprocess calls instead of 40.
func (s *Service) SyncWatchesBatched(ctx context.Context, watches []*PRWatch) ([]PRWatchSyncResult, error) {
	outcome := s.syncWatchesBatchedWithClient(ctx, s.client, "legacy", "", 0, watches, false, false)
	return outcome.Results, outcome.Err
}

// SyncWorkspaceWatchesBatched resolves one automation credential and rejects
// mixed or missing workspace ownership before making a provider call.
func (s *Service) SyncWorkspaceWatchesBatched(
	ctx context.Context, workspaceID string, watches []*PRWatch,
) ([]PRWatchSyncResult, error) {
	outcome := s.syncWorkspaceWatchesBatched(ctx, workspaceID, watches, false, false)
	return outcome.Results, outcome.Err
}

// syncWorkspaceWatchesBatchedForPoller keeps unfinished phases visible so the
// monitor loop can resume them at their admission boundary.
func (s *Service) syncWorkspaceWatchesBatchedForPoller(
	ctx context.Context, workspaceID string, watches []*PRWatch,
) prWatchSyncOutcome {
	return s.syncWorkspaceWatchesBatched(ctx, workspaceID, watches, false, true)
}

func (s *Service) syncWorkspaceWatchesBatched(
	ctx context.Context, workspaceID string, watches []*PRWatch, explicitRefresh, publishFeedback bool,
) prWatchSyncOutcome {
	if len(watches) == 0 {
		return prWatchSyncOutcome{}
	}
	if strings.TrimSpace(workspaceID) == "" {
		return prWatchSyncFailedOutcome(watches, ErrGitHubWorkspaceRequired)
	}
	for _, watch := range watches {
		if watch == nil || watch.WorkspaceID != workspaceID {
			return prWatchSyncFailedOutcome(watches, ErrGitHubWorkspaceRequired)
		}
	}
	host, err := s.selectedGitHubHost(ctx, workspaceID)
	if err != nil {
		return prWatchSyncFailedOutcome(watches, err)
	}
	currentWatches := make([]*PRWatch, 0, len(watches))
	for _, watch := range watches {
		if err := requireExpectedGitHubHost(withExpectedGitHubHost(ctx, defaultAuthFlowHost(watch.GitHubHost)), host); err != nil {
			if errors.Is(err, ErrGitHubHostMismatch) {
				continue
			}
			return prWatchSyncFailedOutcome(watches, err)
		}
		if err := s.validatePRWatchHost(ctx, workspaceID, watch); err != nil {
			return prWatchSyncFailedOutcome(watches, err)
		}
		currentWatches = append(currentWatches, watch)
	}
	if len(currentWatches) == 0 {
		return prWatchSyncOutcome{}
	}
	watches = currentWatches
	ctx = withExpectedGitHubHost(ctx, host.String())
	activeWatches, err := s.filterActivePRWatches(ctx, watches)
	if err != nil {
		return prWatchSyncFailedOutcome(watches, err)
	}
	watches = activeWatches
	if len(watches) == 0 {
		return prWatchSyncOutcome{}
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		return prWatchSyncFailedOutcome(watches, err)
	}
	credentialGeneration := int64(0)
	if resolved.credential != nil {
		credentialGeneration = resolved.credential.CredentialGeneration
	}
	if explicitRefresh {
		s.invalidateWorkflowAttentionForResolvedWatches(resolved, watches)
	}
	return s.syncWatchesBatchedWithClient(
		withGitHubOperationFence(ctx, s, workspaceID, resolved), resolved.Client, resolved.CacheScope,
		workspaceID, credentialGeneration, watches, explicitRefresh, publishFeedback,
	)
}

// prWatchSyncFailedOutcome reports work that never reached the provider.
func prWatchSyncFailedOutcome(watches []*PRWatch, err error) prWatchSyncOutcome {
	outcome := prWatchSyncOutcome{Err: err, Deferred: admissionDeferralOf(err)}
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		outcome.Pending = appendPRWatchSyncPending(
			outcome.Pending, watch.PRNumber > 0, []*PRWatch{watch},
		)
	}
	return outcome
}

func admissionDeferralOf(err error) *AdmissionDeferredError {
	var deferred *AdmissionDeferredError
	if errors.As(err, &deferred) {
		return deferred
	}
	return nil
}

// prWatchPendingWatches flattens pending phases back into their watches.
func prWatchPendingWatches(pending []prWatchSyncPending) []*PRWatch {
	watches := make([]*PRWatch, 0, len(pending))
	for _, entry := range pending {
		watches = append(watches, entry.Watches...)
	}
	return watches
}

// appendPRWatchSyncPending merges one phase's unfinished watches, so an
// outcome holds at most one entry per phase.
func appendPRWatchSyncPending(
	pending []prWatchSyncPending, numbered bool, watches []*PRWatch,
) []prWatchSyncPending {
	if len(watches) == 0 {
		return pending
	}
	for i := range pending {
		if pending[i].Numbered == numbered {
			pending[i].Watches = append(pending[i].Watches, watches...)
			return pending
		}
	}
	return append(pending, prWatchSyncPending{Numbered: numbered, Watches: watches})
}

func (s *Service) filterActivePRWatches(ctx context.Context, watches []*PRWatch) ([]*PRWatch, error) {
	active := make([]*PRWatch, 0, len(watches))
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		if err := s.checkPRWatchActive(ctx, watch); err != nil {
			if errors.Is(err, ErrPRWatchNotFound) || errors.Is(err, ErrPRWatchRetired) {
				continue
			}
			return nil, err
		}
		active = append(active, watch)
	}
	return active, nil
}

type prWatchBatchGroup struct {
	key            string
	target         prDiscoveryHealthTarget
	representative *PRWatch
	watches        []*PRWatch
	numbered       bool
	attempt        prDiscoveryWatchAttempt
}

func (s *Service) syncWatchesBatchedWithClient(
	ctx context.Context, client Client, cacheScope, workspaceID string, credentialGeneration int64,
	watches []*PRWatch, explicitRefresh, publishFeedback bool,
) prWatchSyncOutcome {
	if len(watches) == 0 {
		return prWatchSyncOutcome{}
	}
	activeWatches, err := s.filterActivePRWatches(ctx, watches)
	if err != nil {
		return prWatchSyncFailedOutcome(watches, err)
	}
	watches = activeWatches
	if len(watches) == 0 {
		return prWatchSyncOutcome{}
	}
	exec, err := graphQLExecutorFor(client)
	if err != nil {
		return prWatchSyncFailedOutcome(watches, err)
	}
	leaderGroups, joinedGroups := s.beginPRWatchBatchGroups(
		workspaceID, cacheScope, credentialGeneration, watches, explicitRefresh,
	)
	if len(leaderGroups) == 0 && len(joinedGroups) == 0 {
		return prWatchSyncOutcome{Results: noOpPRWatchSyncResults(watches)}
	}
	req := batchedPRWatchSync{
		ctx: ctx, client: client, exec: exec, cacheScope: cacheScope, workspaceID: workspaceID,
		credentialGeneration: credentialGeneration, explicitRefresh: explicitRefresh,
		publishFeedback: publishFeedback,
	}
	resultsByWatch := make(map[*PRWatch]PRWatchSyncResult, len(watches))
	outcome := s.runBatchedLeaderPhases(req, leaderGroups, resultsByWatch)
	pending, joinedErr := s.applyJoinedBatchGroups(req, joinedGroups, resultsByWatch)
	outcome.Pending = append(outcome.Pending, pending...)
	if joinedErr != nil {
		outcome.Err = firstPRWatchSyncError(outcome.Err, joinedErr)
		if outcome.Deferred == nil {
			outcome.Deferred = admissionDeferralOf(joinedErr)
		}
	}
	outcome.Results = collectPRWatchSyncResults(watches, resultsByWatch)
	return outcome
}

// batchedPRWatchSync bundles one batched pass's fixed inputs.
type batchedPRWatchSync struct {
	ctx                  context.Context
	client               Client
	exec                 GraphQLExecutor
	cacheScope           string
	workspaceID          string
	credentialGeneration int64
	explicitRefresh      bool
	publishFeedback      bool
}

// beginPRWatchBatchGroups coalesces equivalent targets and performs the atomic
// admission decision for each representative. Duplicate consumers are
// registered before their representative so another entry point cannot observe
// them as consumers before the representative joins an existing attempt.
func (s *Service) beginPRWatchBatchGroups(
	workspaceID, cacheScope string, credentialGeneration int64,
	watches []*PRWatch, explicitRefresh bool,
) (leaderGroups, joinedGroups []*prWatchBatchGroup) {
	groupsByKey := make(map[string]*prWatchBatchGroup, len(watches))
	groups := make([]*prWatchBatchGroup, 0, len(watches))
	for _, watch := range watches {
		if watch == nil {
			continue
		}
		target := prDiscoveryTargetForWatch(watch)
		key := target.key()
		group := groupsByKey[key]
		if group == nil {
			group = &prWatchBatchGroup{
				key:            key,
				target:         target,
				representative: watch,
				numbered:       target.PRNumber > 0,
			}
			groupsByKey[key] = group
			groups = append(groups, group)
		}
		group.watches = append(group.watches, watch)
	}
	leaderGroups = make([]*prWatchBatchGroup, 0, len(groups))
	joinedGroups = make([]*prWatchBatchGroup, 0, len(groups))
	for _, group := range groups {
		for _, watch := range group.watches[1:] {
			s.trackPRDiscoveryWatchConsumer(workspaceID, cacheScope, credentialGeneration, watch)
		}
		attempt, ok := s.beginPRDiscoveryWatchWithOptions(
			workspaceID, cacheScope, credentialGeneration, group.representative, explicitRefresh,
		)
		if !ok {
			continue
		}
		group.attempt = attempt
		if attempt.joined {
			joinedGroups = append(joinedGroups, group)
			continue
		}
		leaderGroups = append(leaderGroups, group)
	}
	return leaderGroups, joinedGroups
}

// runBatchedLeaderPhases fetches and applies each phase in order. A completed
// numbered phase is applied as soon as its read finishes, before the searching
// read starts, so unrelated discovery latency or its context deadline cannot
// withhold a finished known-PR update. Unfinished phases are returned so the
// caller can resume them at their own boundary.
func (s *Service) runBatchedLeaderPhases(
	req batchedPRWatchSync, leaderGroups []*prWatchBatchGroup,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) prWatchSyncOutcome {
	if len(leaderGroups) == 0 {
		return prWatchSyncOutcome{}
	}
	numbered, searching := splitBatchGroups(leaderGroups)
	now := time.Now().UTC()
	numberedStatuses, err := s.fetchBatchedNumberedPhase(
		req.ctx, req.client, req.exec, req.cacheScope,
		batchGroupWatches(numbered), req.explicitRefresh,
	)
	if err != nil {
		// Complete every still-open leader attempt, including searching groups
		// whose attempts were admitted before this pass but never fetched, so a
		// joined consumer cannot wait forever on an abandoned attempt.
		pending, paused := s.failBatchedBatchGroups(req, leaderGroups, err)
		return batchedPhaseFailureOutcome(pending, paused, err)
	}
	s.applyNumberedBatchGroups(
		req, numbered, &batchedWatchStatuses{byKey: numberedStatuses}, now, resultsByWatch,
	)
	if len(searching) == 0 {
		return prWatchSyncOutcome{}
	}
	searchingStatuses := &batchedWatchStatuses{
		byKey:               make(map[string]*PRStatus, len(searching)),
		branchResolvedEmpty: make(map[string]struct{}, len(searching)),
	}
	if err := s.fetchBatchedSearchingPhase(
		req.ctx, req.client, req.exec, req.cacheScope,
		batchGroupWatches(searching), searchingStatuses, req.explicitRefresh,
	); err != nil {
		pending, paused := s.failBatchedBatchGroups(req, searching, err)
		return batchedPhaseFailureOutcome(pending, paused, err)
	}
	s.applySearchingBatchGroups(req, searching, searchingStatuses, now, resultsByWatch)
	return prWatchSyncOutcome{}
}

// batchedPhaseFailureOutcome reports unfinished work unless the shared health
// store already paused those targets.
func batchedPhaseFailureOutcome(
	pending []prWatchSyncPending, paused bool, err error,
) prWatchSyncOutcome {
	if paused {
		return prWatchSyncOutcome{}
	}
	return prWatchSyncOutcome{Pending: pending, Err: err, Deferred: admissionDeferralOf(err)}
}

func splitBatchGroups(groups []*prWatchBatchGroup) (numbered, searching []*prWatchBatchGroup) {
	for _, group := range groups {
		if group.numbered {
			numbered = append(numbered, group)
			continue
		}
		searching = append(searching, group)
	}
	return numbered, searching
}

func batchGroupWatches(groups []*prWatchBatchGroup) []*PRWatch {
	watches := make([]*PRWatch, 0, len(groups))
	for _, group := range groups {
		watches = append(watches, group.representative)
	}
	return watches
}

// failBatchedBatchGroups releases unfinished attempts without inventing a
// provider failure for local admission deferral, then reports their watches.
// paused reports a schema or throttling rejection the shared health store now
// owns, which must not repeat through per-watch fallback.
func (s *Service) failBatchedBatchGroups(
	req batchedPRWatchSync, groups []*prWatchBatchGroup, err error,
) (pending []prWatchSyncPending, paused bool) {
	category := classifyPRDiscoveryError(err)
	paused = category == PRDiscoveryHealthInvalidQuery || category == PRDiscoveryHealthRateLimited
	pending = make([]prWatchSyncPending, 0, 2)
	for _, group := range groups {
		if paused {
			s.finishPRDiscoveryWatchFailure(
				req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt, err,
			)
		} else {
			s.releasePRDiscoveryWatch(
				req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
			)
		}
		s.completePRDiscoveryWatchAttempt(group.attempt, nil, false, true, err, paused)
		s.forgetPRDiscoveryWatchAttempt(
			req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
		)
		pending = appendPRWatchSyncPending(pending, group.numbered, group.watches)
	}
	return pending, paused
}

func collectPRWatchSyncResults(
	watches []*PRWatch, resultsByWatch map[*PRWatch]PRWatchSyncResult,
) []PRWatchSyncResult {
	results := make([]PRWatchSyncResult, 0, len(watches))
	for _, watch := range watches {
		if result, ok := resultsByWatch[watch]; ok {
			results = append(results, result)
			continue
		}
		results = append(results, PRWatchSyncResult{Watch: watch})
	}
	return results
}

func firstPRWatchSyncError(current, next error) error {
	if current != nil {
		return current
	}
	return next
}

func noOpPRWatchSyncResults(watches []*PRWatch) []PRWatchSyncResult {
	results := make([]PRWatchSyncResult, 0, len(watches))
	for _, watch := range watches {
		results = append(results, PRWatchSyncResult{Watch: watch})
	}
	return results
}

// applyNumberedBatchGroups writes the completed known-PR phase. A successful
// known-PR query can legitimately return no alias when the PR is no longer
// open; that is still a provider success.
func (s *Service) applyNumberedBatchGroups(
	req batchedPRWatchSync, groups []*prWatchBatchGroup,
	statuses *batchedWatchStatuses, now time.Time,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) {
	for _, group := range groups {
		if group.attempt.result.invalidated() {
			continue
		}
		representative := s.applyNumberedBatchGroup(req, group, statuses, now, resultsByWatch)
		s.completePRDiscoveryWatchAttempt(
			group.attempt, representative.Status, true, representative.SyncFailed, nil, false,
		)
		if s.isRepoCachedAsMissingForScope(req.cacheScope, group.target.Owner, group.target.Repo) {
			s.finishPRDiscoveryWatchFailureCategory(
				req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
				PRDiscoveryHealthUnavailable,
			)
		} else {
			s.finishPRDiscoveryWatchSuccess(
				req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
			)
		}
		s.forgetPRDiscoveryWatchAttempt(
			req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
		)
	}
}

func (s *Service) applyNumberedBatchGroup(
	req batchedPRWatchSync, group *prWatchBatchGroup,
	statuses *batchedWatchStatuses, now time.Time,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) PRWatchSyncResult {
	var representative PRWatchSyncResult
	for index, watch := range group.watches {
		result := s.applyBatchedNumberedWatch(
			req.ctx, req.cacheScope, watch, statuses.byKey, now, req.publishFeedback,
		)
		resultsByWatch[watch] = result
		if index == 0 {
			representative = result
		}
	}
	return representative
}

func (s *Service) applySearchingBatchGroups(
	req batchedPRWatchSync, groups []*prWatchBatchGroup,
	statuses *batchedWatchStatuses, now time.Time,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) {
	for _, group := range groups {
		if group.attempt.result.invalidated() {
			continue
		}
		representative := s.applySearchingBatchGroup(req, group, statuses, now, resultsByWatch)
		s.completePRDiscoveryWatchAttempt(
			group.attempt, representative.Status, representative.DiscoveryResolved,
			representative.SyncFailed, nil, false,
		)
		s.finishSearchingBatchGroup(req, group, representative, resultsByWatch)
	}
}

func (s *Service) applySearchingBatchGroup(
	req batchedPRWatchSync, group *prWatchBatchGroup,
	statuses *batchedWatchStatuses, now time.Time,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) PRWatchSyncResult {
	var representative PRWatchSyncResult
	for index, watch := range group.watches {
		result := s.applyBatchedSearchingWatch(
			req.ctx, req.client, req.cacheScope, watch, statuses, now, false, false, false,
		)
		resultsByWatch[watch] = result
		if index == 0 {
			representative = result
		}
	}
	return representative
}

func (s *Service) finishSearchingBatchGroup(
	req batchedPRWatchSync, group *prWatchBatchGroup,
	representative PRWatchSyncResult, resultsByWatch map[*PRWatch]PRWatchSyncResult,
) {
	if representative.DiscoveryResolved {
		s.finishPRDiscoveryWatchSuccess(
			req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
		)
		s.trackResolvedSearchingBatchTargets(req, group, resultsByWatch)
		s.forgetPRDiscoveryWatchAttempt(
			req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
		)
		return
	}
	if representative.SyncFailed {
		// An unresolved alias whose fallback failed is an unavailable discovery
		// result. If the fallback was not due yet, release the attempt and
		// preserve any existing failure without inventing a new one.
		s.finishPRDiscoveryWatchFailureCategory(
			req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
			PRDiscoveryHealthUnavailable,
		)
		s.forgetPRDiscoveryWatchAttempt(
			req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt,
		)
		return
	}
	s.releasePRDiscoveryWatch(req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt)
	s.forgetPRDiscoveryWatchAttempt(req.workspaceID, req.cacheScope, req.credentialGeneration, group.attempt)
}

func (s *Service) trackResolvedSearchingBatchTargets(
	req batchedPRWatchSync, group *prWatchBatchGroup,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) {
	for _, watch := range group.watches {
		result := resultsByWatch[watch]
		if !result.DiscoveryTrackable {
			continue
		}
		target := prDiscoveryTargetForWatch(watch)
		if result.Status != nil && result.Status.PR != nil && !result.SyncFailed {
			target = prDiscoveryHealthTarget{
				Owner:    result.Status.PR.RepoOwner,
				Repo:     result.Status.PR.RepoName,
				Branch:   watch.Branch,
				PRNumber: result.Status.PR.Number,
			}
		}
		s.trackPRDiscoveryWatchTarget(
			req.workspaceID, req.cacheScope, req.credentialGeneration, watch, target,
		)
	}
}

// applyJoinedBatchGroups shares one in-flight attempt's outcome with the
// consumers that joined it. A shared failure reports the joined watches as
// unfinished work for the caller to resume.
func (s *Service) applyJoinedBatchGroups(
	req batchedPRWatchSync, groups []*prWatchBatchGroup,
	resultsByWatch map[*PRWatch]PRWatchSyncResult,
) ([]prWatchSyncPending, error) {
	pending := make([]prWatchSyncPending, 0, 2)
	var sharedFailure error
	now := time.Now().UTC()
	for _, group := range groups {
		status, resolved, syncFailed, sharedErr, suppressFallback := group.attempt.result.wait(req.ctx)
		if sharedErr != nil {
			if suppressFallback {
				continue
			}
			sharedFailure = firstPRWatchSyncError(sharedFailure, sharedErr)
			pending = appendPRWatchSyncPending(pending, group.numbered, group.watches)
			continue
		}
		if status == nil && !resolved && syncFailed {
			continue
		}
		s.applyJoinedBatchGroup(req, group, status, resolved, syncFailed, now, resultsByWatch)
	}
	return pending, sharedFailure
}

func (s *Service) applyJoinedBatchGroup(
	req batchedPRWatchSync, group *prWatchBatchGroup, status *PRStatus,
	resolved, syncFailed bool, now time.Time, resultsByWatch map[*PRWatch]PRWatchSyncResult,
) {
	if group.numbered {
		byKey := make(map[string]*PRStatus, 1)
		if status != nil {
			byKey[prStatusCacheKey(group.target.Owner, group.target.Repo, group.target.PRNumber)] = status
		}
		for _, watch := range group.watches {
			resultsByWatch[watch] = s.applyBatchedNumberedWatch(
				req.ctx, req.cacheScope, watch, byKey, now, req.publishFeedback,
			)
		}
		return
	}
	byKey := make(map[string]*PRStatus, 1)
	if status != nil {
		byKey[graphqlBranchKey(group.target.Owner, group.target.Repo, group.target.Branch)] = status
	}
	sharedStatuses := &batchedWatchStatuses{byKey: byKey}
	for _, watch := range group.watches {
		resultsByWatch[watch] = s.applyBatchedSearchingWatch(
			req.ctx, req.client, req.cacheScope, watch, sharedStatuses, now, true, resolved, syncFailed,
		)
	}
}

const (
	workflowAttentionBatchConcurrency = 4
	workflowAttentionBatchBudget      = 5 * time.Second
)

type workflowAttentionStatusGroup struct {
	owner, repo, headSHA string
	statuses             []*PRStatus
}

// enrichBatchedWorkflowAttention adds Actions evidence to GraphQL statuses.
// Workflow runs are fetched once per credential scope, target repository, and
// head SHA; job reads are shared within that group. The enrichment is best
// effort so missing Actions permission never discards review or check data.
func (s *Service) enrichBatchedWorkflowAttention(
	ctx context.Context, client Client, cacheScope string, statuses map[string]*PRStatus,
) {
	groups := groupBatchedWorkflowAttentionStatuses(cacheScope, statuses)
	if len(groups) == 0 {
		return
	}

	baseCtx, cancelBase := derivedFetchContext(ctx)
	defer cancelBase()
	fetchCtx, cancel := context.WithTimeout(baseCtx, workflowAttentionBatchBudget)
	defer cancel()
	sem := make(chan struct{}, workflowAttentionBatchConcurrency)
	var wg sync.WaitGroup
	for _, group := range groups {
		group := group
		wg.Add(1)
		go s.enrichBatchedWorkflowAttentionGroup(fetchCtx, client, cacheScope, group, sem, &wg)
	}
	wg.Wait()
}

func groupBatchedWorkflowAttentionStatuses(
	cacheScope string, statuses map[string]*PRStatus,
) map[string]*workflowAttentionStatusGroup {
	groups := make(map[string]*workflowAttentionStatusGroup)
	for _, status := range statuses {
		if status == nil || status.PR == nil {
			continue
		}
		if isTerminalPR(status.PR) {
			status.WorkflowAttention = workflowAttentionNone(status.PR.HeadSHA)
			status.WorkflowAttentionPopulated = true
			continue
		}
		if status.PR.HeadSHA == "" {
			status.WorkflowAttention = workflowAttentionUnknown("")
			status.WorkflowAttentionPopulated = true
			continue
		}
		key := workflowAttentionBatchKey(cacheScope, status.PR.RepoOwner, status.PR.RepoName, status.PR.HeadSHA)
		group := groups[key]
		if group == nil {
			group = &workflowAttentionStatusGroup{
				owner: status.PR.RepoOwner, repo: status.PR.RepoName, headSHA: status.PR.HeadSHA,
			}
			groups[key] = group
		}
		group.statuses = append(group.statuses, status)
	}
	return groups
}

func acquireWorkflowAttentionBatchSlot(ctx context.Context, sem chan struct{}) bool {
	select {
	case sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func markBatchedWorkflowAttentionUnknown(group *workflowAttentionStatusGroup) {
	for _, status := range group.statuses {
		status.WorkflowAttention = workflowAttentionUnknown(group.headSHA)
		status.WorkflowAttentionPopulated = true
	}
}

func (s *Service) enrichBatchedWorkflowAttentionGroup(
	ctx context.Context,
	client Client,
	cacheScope string,
	group *workflowAttentionStatusGroup,
	sem chan struct{},
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	if !acquireWorkflowAttentionBatchSlot(ctx, sem) {
		markBatchedWorkflowAttentionUnknown(group)
		return
	}
	defer func() { <-sem }()

	runs, err := s.cachedWorkflowRuns(ctx, client, cacheScope, group.owner, group.repo, group.headSHA)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("workflow attention read unavailable",
				zap.String("owner", group.owner), zap.String("repo", group.repo), zap.Error(err))
		}
		markBatchedWorkflowAttentionUnknown(group)
		return
	}
	s.applyBatchedWorkflowAttention(ctx, client, cacheScope, group, runs)
}

func (s *Service) applyBatchedWorkflowAttention(
	ctx context.Context, client Client, cacheScope string, group *workflowAttentionStatusGroup, runs []WorkflowRun,
) {
	jobs := make(map[workflowJobKey]struct {
		value []WorkflowJob
		err   error
	})
	readJobs := func(jobCtx context.Context, runID int64, attempt int) ([]WorkflowJob, error) {
		jobKey := workflowJobKey{Owner: group.owner, Repo: group.repo, RunID: runID, Attempt: attempt}
		if cached, found := jobs[jobKey]; found {
			return cached.value, cached.err
		}
		value, readErr := s.cachedWorkflowJobs(
			jobCtx, client, cacheScope, group.owner, group.repo, runID, attempt, true,
		)
		jobs[jobKey] = struct {
			value []WorkflowJob
			err   error
		}{value: value, err: readErr}
		return value, readErr
	}
	for _, status := range group.statuses {
		if status == nil || status.PR == nil {
			continue
		}
		status.WorkflowAttention = classifyWorkflowAttentionWithJobs(
			ctx, group.owner, group.repo, status.PR, runs, readJobs,
		)
		status.WorkflowAttentionPopulated = true
	}
}

func workflowAttentionBatchKey(cacheScope, owner, repo, headSHA string) string {
	return workflowRunsCacheKey(cacheScope, owner, repo, headSHA)
}

// batchedWatchStatuses is the shared, read-only result of one batched phase.
//
// byKey holds every PR the phase resolved, keyed by prStatusCacheKey for
// numbered watches and graphqlBranchKey for searching ones. branchResolvedEmpty
// holds the branch keys the phase answered definitively as "no open PR" — see
// branchBatchResult for why that distinction matters.
type batchedWatchStatuses struct {
	byKey               map[string]*PRStatus
	branchResolvedEmpty map[string]struct{}
}

func (s *Service) fetchBatchedNumberedPhase(
	ctx context.Context, client Client, exec GraphQLExecutor, cacheScope string,
	numbered []*PRWatch, explicitRefresh bool,
) (map[string]*PRStatus, error) {
	if len(numbered) == 0 {
		return nil, nil
	}
	progressKey := batchedPhaseKey(cacheScope, batchedNumberedSingleflightKey(numbered), explicitRefresh)
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	v, err, _ := s.syncGroup.Do(progressKey, func() (interface{}, error) {
		// Snapshot the negative-cache generation BEFORE the fetch so an
		// eviction (relink / clear) firing while the GraphQL call is in flight
		// wins over the post-fetch markRepoAsMissing writes — see
		// Service.markRepoAsMissing.
		repoErrGen := s.repoErrorGenSnapshot()
		statuses := make(map[string]*PRStatus, len(numbered))
		progress, fetchErr := s.fetchBatchedPRStatusesWithProgress(
			fetchCtx, exec, cacheScope, progressKey, numbered, statuses, repoErrGen,
		)
		if fetchErr != nil {
			s.finishBatchedPRQueryProgress(progressKey, progress, keepBatchedPRProgress(fetchErr))
			return nil, fetchErr
		}
		s.enrichBatchedWorkflowAttention(fetchCtx, client, cacheScope, statuses)
		s.finishBatchedPRQueryProgress(progressKey, progress, false)
		return statuses, nil
	})
	if err != nil {
		return nil, err
	}
	statuses, _ := v.(map[string]*PRStatus)
	return statuses, nil
}

func (s *Service) fetchBatchedSearchingPhase(
	ctx context.Context, client Client, exec GraphQLExecutor, cacheScope string,
	searching []*PRWatch, combined *batchedWatchStatuses, explicitRefresh bool,
) error {
	if len(searching) == 0 {
		return nil
	}
	progressKey := batchedPhaseKey(cacheScope, batchedSearchingSingleflightKey(searching), explicitRefresh)
	fetchCtx, cancelFetch := derivedFetchContext(ctx)
	defer cancelFetch()
	v, err, _ := s.syncGroup.Do(progressKey, func() (interface{}, error) {
		repoErrGen := s.repoErrorGenSnapshot()
		phase := &batchedWatchStatuses{
			byKey:               make(map[string]*PRStatus, len(searching)),
			branchResolvedEmpty: make(map[string]struct{}, len(searching)),
		}
		progress, fetchErr := s.fetchBatchedBranchStatusesWithProgress(
			fetchCtx, exec, cacheScope, progressKey, searching, phase, repoErrGen,
		)
		if fetchErr != nil {
			s.finishBatchedBranchQueryProgress(progressKey, progress, keepBatchedPRProgress(fetchErr))
			return nil, fetchErr
		}
		s.enrichBatchedWorkflowAttention(fetchCtx, client, cacheScope, phase.byKey)
		s.finishBatchedBranchQueryProgress(progressKey, progress, false)
		return phase, nil
	})
	if err != nil {
		return err
	}
	phase, ok := v.(*batchedWatchStatuses)
	if !ok || phase == nil {
		return nil
	}
	for key, status := range phase.byKey {
		combined.byKey[key] = status
	}
	for key := range phase.branchResolvedEmpty {
		combined.branchResolvedEmpty[key] = struct{}{}
	}
	return nil
}

func batchedPhaseKey(cacheScope, phaseKey string, explicitRefresh bool) string {
	key := scopedCacheKey(cacheScope, phaseKey)
	if explicitRefresh {
		key += prSyncExplicitRefreshKeySuffix
	}
	return key
}

// batchedNumberedSingleflightKey builds a deterministic key from the sorted
// numbered ref tuples. Sorting ensures ordering doesn't split equivalent calls
// into separate slots, and lowercasing matches repoErrorCacheKey so the
// in-flight key is stable regardless of caller casing (watches from the DB
// always have consistent casing in practice, but it costs nothing to make the
// key insensitive).
func batchedNumberedSingleflightKey(numbered []*PRWatch) string {
	parts := make([]string, 0, len(numbered))
	for _, w := range numbered {
		parts = append(parts, fmt.Sprintf("n:%s/%s#%d",
			strings.ToLower(w.Owner), strings.ToLower(w.Repo), w.PRNumber))
	}
	sort.Strings(parts)
	return "batched-pr:" + strings.Join(parts, "|")
}

// batchedSearchingSingleflightKey is the branch-phase sibling. The distinct
// "b:" prefix prevents a branch ref colliding with a numbered ref that happens
// to render the same string.
func batchedSearchingSingleflightKey(searching []*PRWatch) string {
	parts := make([]string, 0, len(searching))
	for _, w := range searching {
		parts = append(parts, fmt.Sprintf("b:%s/%s@%s",
			strings.ToLower(w.Owner), strings.ToLower(w.Repo), w.Branch))
	}
	sort.Strings(parts)
	return "batched-branch:" + strings.Join(parts, "|")
}

// fetchBatchedPRStatuses runs the numbered-watch (PR-number-keyed) batch.
// Watches whose repo is in the negative cache are dropped before building
// the GraphQL refs so the dead-repo storm doesn't burn gh throttle slots.
// Missing repos surfaced by the response are negative-cached for the next
// 10 minutes; partial results for the repos that did resolve are merged
// into `combined`.
func (s *Service) fetchBatchedPRStatuses(
	ctx context.Context, exec GraphQLExecutor, cacheScope, progressKey string,
	numbered []*PRWatch, combined map[string]*PRStatus,
	repoErrGen uint64,
) error {
	progress, err := s.fetchBatchedPRStatusesWithProgress(ctx, exec, cacheScope, progressKey, numbered, combined, repoErrGen)
	s.finishBatchedPRQueryProgress(progressKey, progress, keepBatchedPRProgress(err))
	return err
}

// fetchBatchedPRStatusesWithProgress leaves cleanup to the whole-batch caller.
// A mixed batch needs a completed numbered phase to survive a deferred branch phase.
func (s *Service) fetchBatchedPRStatusesWithProgress(
	ctx context.Context, exec GraphQLExecutor, cacheScope, progressKey string,
	numbered []*PRWatch, combined map[string]*PRStatus,
	repoErrGen uint64,
) (*batchedPRQueryProgress, error) {
	refs := make([]graphQLPRRef, 0, len(numbered))
	for _, w := range numbered {
		if s.isRepoCachedAsMissingForScope(cacheScope, w.Owner, w.Repo) {
			continue
		}
		refs = append(refs, graphQLPRRef{Owner: w.Owner, Repo: w.Repo, Number: w.PRNumber})
	}
	if len(refs) == 0 {
		s.discardBatchedPRQueryProgress(progressKey)
		return nil, nil
	}
	progress := s.batchedPRQueryProgress(progressKey, batchedPRProgressRefsKey(refs))
	out, err := runBatchedPRQueryWithProgress(ctx, exec, refs, progress)
	out, err = s.absorbMissingReposErr(out, err, cacheScope, repoErrGen)
	if err != nil {
		return progress, fmt.Errorf("batched PR query: %w", err)
	}
	for k, v := range out {
		combined[k] = v
	}
	return progress, nil
}

// fetchBatchedBranchStatuses runs the branch-keyed batch with the same
// negative-cache filter and missing-repo absorption as the numbered path.
func (s *Service) fetchBatchedBranchStatuses(
	ctx context.Context, exec GraphQLExecutor, cacheScope, progressKey string,
	searching []*PRWatch, combined *batchedWatchStatuses,
	repoErrGen uint64,
) error {
	progress, err := s.fetchBatchedBranchStatusesWithProgress(ctx, exec, cacheScope, progressKey, searching, combined, repoErrGen)
	s.finishBatchedBranchQueryProgress(progressKey, progress, keepBatchedPRProgress(err))
	return err
}

// fetchBatchedBranchStatusesWithProgress leaves cleanup to the whole-batch caller.
func (s *Service) fetchBatchedBranchStatusesWithProgress(
	ctx context.Context, exec GraphQLExecutor, cacheScope, progressKey string,
	searching []*PRWatch, combined *batchedWatchStatuses,
	repoErrGen uint64,
) (*batchedBranchQueryProgress, error) {
	refs := make([]graphQLBranchRef, 0, len(searching))
	for _, w := range searching {
		if s.isRepoCachedAsMissingForScope(cacheScope, w.Owner, w.Repo) {
			continue
		}
		refs = append(refs, graphQLBranchRef{Owner: w.Owner, Repo: w.Repo, Branch: w.Branch})
	}
	if len(refs) == 0 {
		s.discardBatchedBranchQueryProgress(progressKey)
		return nil, nil
	}
	progress := s.batchedBranchQueryProgress(progressKey, batchedBranchProgressRefsKey(refs))
	out, err := runBatchedBranchQueryWithProgress(ctx, exec, refs, progress)
	statuses, err := s.absorbMissingReposErr(out.Statuses, err, cacheScope, repoErrGen)
	if err != nil {
		return progress, fmt.Errorf("batched branch query: %w", err)
	}
	for k, v := range statuses {
		combined.byKey[k] = v
	}
	// Only carry the definitive negatives that survived error absorption. A
	// purely-missing-repos error keeps its partial decode; anything else made
	// absorbMissingReposErr return early above.
	if statuses != nil {
		for k := range out.ResolvedEmpty {
			combined.branchResolvedEmpty[k] = struct{}{}
		}
	}
	return progress, nil
}

// absorbMissingReposErr extracts the negative-cacheable repos from a
// runBatched* result and seeds the cache, then returns (partial,
// remaining-err). When the error was PURELY missing-repo errors, the
// partial decode is preserved and a nil error is returned so the caller
// can use the resolved repos' data. When other errors were also
// present, the inner error is returned unchanged so the caller falls
// back to per-watch checks. `repoErrGen` is the negative-cache generation
// snapshot taken BEFORE the fetch so a concurrent eviction wins.
func (s *Service) absorbMissingReposErr(
	out map[string]*PRStatus, err error, cacheScope string, repoErrGen uint64,
) (map[string]*PRStatus, error) {
	if err == nil {
		return out, nil
	}
	var missingErr *batchedMissingReposErr
	if !errors.As(err, &missingErr) {
		return nil, err
	}
	for _, r := range missingErr.Repos {
		s.markRepoAsMissingForScope(cacheScope, r.Owner, r.Repo, repoErrGen)
		s.logger.Debug("repo not resolvable; negative-cached",
			zap.String("owner", r.Owner), zap.String("repo", r.Repo))
	}
	if missingErr.Inner != nil {
		return nil, missingErr.Inner
	}
	return out, nil
}

// applyBatchedNumberedWatch mirrors Poller.applyPRStatus on the service
// side so on-demand callers reuse the same DB-write sequence.
func (s *Service) applyBatchedNumberedWatch(
	ctx context.Context, cacheScope string, w *PRWatch, statusByKey map[string]*PRStatus, now time.Time,
	publishFeedback bool,
) PRWatchSyncResult {
	var unlock func()
	var lockErr error
	ctx, unlock, lockErr = s.lockResourceMutation(ctx, w.WorkspaceID)
	if lockErr != nil {
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}
	defer unlock()
	if err := s.validatePRWatchHost(ctx, w.WorkspaceID, w); err != nil {
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}

	// Repo is in the 10-min negative cache — the fetch path didn't include
	// it in the GraphQL batch, so the apply path can't probe upstream either.
	// Bump last_checked_at to suppress retry storms and surface SyncFailed
	// so the WS handler can flag the result as permanent for the frontend.
	if s.isRepoCachedAsMissingForScope(cacheScope, w.Owner, w.Repo) {
		if _, _, err := s.applyPRWatchDiscoveryResult(ctx, w, nil, now); err != nil {
			s.logSyncError("failed to stamp missing-repository PR watch", err, zap.String("id", w.ID))
		}
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}
	status, ok := statusByKey[prStatusCacheKey(w.Owner, w.Repo, w.PRNumber)]
	if !ok || status == nil || status.PR == nil {
		// Alias missing — best-effort liveness bump so we don't immediately re-probe.
		applied, _, err := s.applyPRWatchDiscoveryResult(ctx, w, nil, now)
		if err != nil {
			s.logSyncError("failed to stamp missing PR watch status", err, zap.String("id", w.ID))
			return PRWatchSyncResult{Watch: w, SyncFailed: true}
		}
		return PRWatchSyncResult{Watch: w, Applied: applied, SyncFailed: status != nil && status.PR == nil}
	}
	applied, changed, err := s.applyPRWatchStatusResult(ctx, w, status, now, publishFeedback)
	if err != nil {
		s.logSyncError("failed to apply batched PR watch result", err, zap.String("id", w.ID))
		return PRWatchSyncResult{Watch: w, Status: status, Found: true, SyncFailed: true}
	}
	return PRWatchSyncResult{
		Watch: w, Status: status, Found: applied, Changed: changed, Applied: applied,
		FeedbackPublished: publishFeedback && applied,
	}
}

// lookupSearchingWatchPR resolves the PR for a searching watch the batched
// query returned nothing for. Returns (nil, nil) when there is no PR to
// promote — including when the probe was skipped.
//
// The batched query already asked GitHub `pullRequests(states: OPEN,
// headRefName: <branch>)` for this exact repository and branch. When it came
// back resolved-and-empty, re-asking through client.FindPRByBranch spends one
// more API call per watch per cycle to receive the same answer — and since a
// branch without a PR keeps that shape indefinitely, that call repeats forever.
// That redundancy was the dominant GitHub GraphQL consumer in production: 42
// searching watches burned ~150 points/minute against a 5000/hour budget, which
// parked the poller on `github rate-limited` roughly 40 minutes into every
// reset window. So on a definitive negative only the fork PARENT — a different
// repository the batch never asked about — is still worth a look, and that
// lookup is itself cached per (scope, owner, repo).
//
// An alias that did not resolve, or a branch carrying several open PRs that
// selectBatchedBranchPRNode refused to guess between, is not a definitive
// negative and keeps the full fork-network fallback.
//
// Either way the probe is throttled on the watch's own last_checked_at exactly
// as triggerPRDetection's is, so a watch costs at most one upstream probe per
// freshness window no matter how many callers sync it — the frontend retries
// every 5s, and that throttle previously guarded only the legacy per-watch
// path this one replaced. The 60s poller always clears the window, so
// detection latency is unchanged.
func (s *Service) lookupSearchingWatchPR(
	ctx context.Context, client Client, cacheScope string, w *PRWatch,
	branchKey string, statuses *batchedWatchStatuses, lastChecked *time.Time,
) (*PR, bool, error) {
	if lastChecked != nil && time.Since(*lastChecked) < PRSyncFreshnessWindow {
		return nil, false, nil
	}
	if _, definitive := statuses.branchResolvedEmpty[branchKey]; definitive {
		pr, err := s.findPRInForkParent(ctx, client, cacheScope, w.Owner, w.Repo, w.Branch)
		return pr, true, err
	}
	pr, err := s.findPRByBranchInForkNetwork(ctx, client, cacheScope, w.Owner, w.Repo, w.Branch)
	return pr, true, err
}

// applyBatchedSearchingWatch mirrors Poller.applyDetectedPR on the service
// side. A searching watch (pr_number=0) is promoted to a known PR when the
// branch lookup returns one; otherwise we just bump last_checked_at.
func (s *Service) applyBatchedSearchingWatch(
	ctx context.Context, client Client, cacheScope string, w *PRWatch, statuses *batchedWatchStatuses, now time.Time,
	skipFallback, sharedResolved, sharedSyncFailed bool,
) PRWatchSyncResult {
	var unlock func()
	var lockErr error
	ctx, unlock, lockErr = s.lockResourceMutation(ctx, w.WorkspaceID)
	if lockErr != nil {
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}
	defer unlock()
	if err := s.validatePRWatchHost(ctx, w.WorkspaceID, w); err != nil {
		return PRWatchSyncResult{Watch: w, SyncFailed: true}
	}

	// Read the previous probe time before the stamp below overwrites it —
	// lookupSearchingWatchPR throttles on it.
	lastChecked := w.LastCheckedAt
	if s.isRepoCachedAsMissingForScope(cacheScope, w.Owner, w.Repo) {
		applied, _, err := s.applyPRWatchDiscoveryResult(ctx, w, nil, now)
		if err != nil {
			s.logSyncError("failed to stamp missing-repository PR watch", err, zap.String("id", w.ID))
		}
		return PRWatchSyncResult{Watch: w, SyncFailed: true, Applied: applied}
	}
	branchKey := graphqlBranchKey(w.Owner, w.Repo, w.Branch)
	status, ok := statuses.byKey[branchKey]
	discoveryResolved := ok && status != nil && status.PR != nil
	if !ok || status == nil || status.PR == nil {
		if skipFallback {
			applied, _, err := s.applyPRWatchDiscoveryResult(ctx, w, nil, now)
			if err != nil {
				s.logSyncError("failed to stamp unresolved PR watch", err, zap.String("id", w.ID))
				return PRWatchSyncResult{Watch: w, SyncFailed: true}
			}
			return PRWatchSyncResult{
				Watch: w, DiscoveryResolved: sharedResolved, DiscoveryTrackable: applied,
				SyncFailed: sharedSyncFailed, Applied: applied,
			}
		}
		pr, attempted, err := s.lookupSearchingWatchPR(ctx, client, cacheScope, w, branchKey, statuses, lastChecked)
		if err != nil {
			applied, _, stampErr := s.applyPRWatchDiscoveryResult(ctx, w, nil, now)
			if stampErr != nil {
				s.logSyncError("failed to stamp failed PR discovery", stampErr, zap.String("id", w.ID))
			}
			s.logger.Debug("failed to search parent repository for PR",
				zap.String("watch_id", w.ID), zap.String("branch", w.Branch), zap.Error(err))
			return PRWatchSyncResult{Watch: w, SyncFailed: true, Applied: applied}
		}
		if !attempted {
			applied, _, err := s.applyPRWatchDiscoveryResult(ctx, w, nil, now)
			if err != nil {
				s.logSyncError("failed to stamp throttled PR discovery", err, zap.String("id", w.ID))
				return PRWatchSyncResult{Watch: w, SyncFailed: true}
			}
			return PRWatchSyncResult{Watch: w, Applied: applied}
		}
		discoveryResolved = true
		if pr == nil {
			applied, _, err := s.applyPRWatchDiscoveryResult(ctx, w, nil, now)
			if err != nil {
				s.logSyncError("failed to stamp empty PR discovery", err, zap.String("id", w.ID))
				return PRWatchSyncResult{Watch: w, SyncFailed: true, DiscoveryResolved: true}
			}
			return PRWatchSyncResult{Watch: w, DiscoveryResolved: true, DiscoveryTrackable: applied, Applied: applied}
		}
		status = &PRStatus{PR: pr}
	}
	applied, promoted, err := s.applyPRWatchDiscoveryResult(ctx, w, status.PR, now)
	if err != nil {
		s.logSyncError("failed to apply detected PR result", err,
			zap.String("watch_id", w.ID), zap.Int("pr_number", status.PR.Number))
		return PRWatchSyncResult{Watch: w, Status: status, Found: true, SyncFailed: true, DiscoveryResolved: discoveryResolved}
	}
	if promoted {
		s.logger.Info("detected PR for session branch (batched)",
			zap.String("watch_id", w.ID), zap.String("branch", w.Branch), zap.Int("pr_number", status.PR.Number))
	}
	return PRWatchSyncResult{
		Watch: w, Status: status, Found: applied, DiscoveryResolved: discoveryResolved,
		DiscoveryTrackable: promoted, Applied: applied,
	}
}
