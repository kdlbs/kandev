package github

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
)

const workflowPhaseSingleflightPrefix = "batched-workflow:"

var errWorkflowSuiteCountUnavailable = errors.New("workflow suite job evidence unavailable")

// workflowPhaseProgress retains step-1 suites and step-2 job counts across a
// deferred wake, so a one-second admission boundary cannot force the whole
// phase to start over.
type workflowPhaseProgress struct {
	evidence    map[string]workflowSuiteObservation
	counts      map[string]int
	unavailable map[string]struct{}
	rest        map[string]*WorkflowAttention
	restPRs     map[string]*PR
	// forceREST keeps a workspace on the serial fallback once GraphQL is
	// unsupported or rejected, including across a deferred wake.
	forceREST bool
}

func newWorkflowPhaseProgress() *workflowPhaseProgress {
	return &workflowPhaseProgress{
		evidence:    make(map[string]workflowSuiteObservation),
		counts:      make(map[string]int),
		unavailable: make(map[string]struct{}),
		rest:        make(map[string]*WorkflowAttention),
		restPRs:     make(map[string]*PR),
	}
}

type workflowPhaseRow struct {
	TaskID       string
	RepositoryID string
	// Previous is the observation read with the row, used only to suppress an
	// event for a first unavailable/empty observation that no surface renders.
	Previous *WorkflowAttention
}

type workflowPhaseTarget struct {
	owner      string
	repo       string
	number     int
	headSHA    string
	headBranch string
	rows       []workflowPhaseRow
	watches    []*PRWatch
}

func (t *workflowPhaseTarget) key() string {
	return fmt.Sprintf("%s/%s#%d@%s", strings.ToLower(t.owner), strings.ToLower(t.repo), t.number, t.headSHA)
}

// evidenceKey is the provider identity shared by every target that reads the
// same repository PR. The head is validated separately from the observation.
func (t *workflowPhaseTarget) evidenceKey() string {
	return workflowSuiteTargetKey(t.owner, t.repo, t.number)
}

type workflowPhaseResult struct {
	Unfinished []*PRWatch
	Deferred   *AdmissionDeferredError
	Err        error
	// useREST asks the attempt to fall back to serial REST evidence.
	useREST bool
}

// workflowPhaseSharedResult is the singleflight-owned evidence fetch. Every
// caller applies it to its own TaskPR rows and its own unfinished watches.
type workflowPhaseSharedResult struct {
	progress *workflowPhaseProgress
	deferred *AdmissionDeferredError
	err      error
}

// runWorkflowPhase refreshes workflow attention for the open known PRs behind
// the supplied watches. It never re-fetches status, so a continuation wake and
// an inventory tick follow one path.
func (s *Service) runWorkflowPhase(
	ctx context.Context, client Client, exec GraphQLExecutor, cacheScope string,
	watches []*PRWatch, explicitRefresh bool,
) workflowPhaseResult {
	targets := s.loadWorkflowPhaseTargets(ctx, watches)
	if len(targets) == 0 {
		return workflowPhaseResult{}
	}
	refsKey := workflowPhaseRefsKey(targets)
	key := batchedPhaseKey(cacheScope, workflowPhaseSingleflightPrefix+refsKey, explicitRefresh)
	v, err, _ := s.syncGroup.Do(key, func() (interface{}, error) {
		return s.fetchWorkflowPhaseAttempt(ctx, client, exec, cacheScope, key, refsKey, targets), nil
	})
	if err != nil {
		return workflowPhaseResult{Unfinished: workflowPhaseWatches(targets), Err: err}
	}
	shared, ok := v.(workflowPhaseSharedResult)
	if !ok || shared.progress == nil {
		return workflowPhaseResult{Unfinished: workflowPhaseWatches(targets)}
	}
	if shared.deferred != nil {
		return workflowPhaseResult{
			Unfinished: workflowPhaseWatches(targets), Deferred: shared.deferred, Err: shared.err,
		}
	}
	s.classifyAndStoreWorkflowPhase(ctx, targets, shared.progress)
	return workflowPhaseResult{Err: shared.err}
}

func (s *Service) fetchWorkflowPhaseAttempt(
	ctx context.Context, client Client, exec GraphQLExecutor, cacheScope, key, refsKey string,
	targets []*workflowPhaseTarget,
) workflowPhaseSharedResult {
	fetchCtx, cancel := derivedFetchContext(ctx)
	defer cancel()
	progress := s.workflowPhaseProgress(key, refsKey)
	result := s.fetchWorkflowPhaseEvidence(fetchCtx, client, exec, cacheScope, targets, progress)
	s.finishWorkflowPhaseProgress(key, progress, result.deferred != nil)
	return result
}

func (s *Service) fetchWorkflowPhaseEvidence(
	ctx context.Context, client Client, exec GraphQLExecutor, cacheScope string,
	targets []*workflowPhaseTarget, progress *workflowPhaseProgress,
) workflowPhaseSharedResult {
	shared := workflowPhaseSharedResult{progress: progress}
	if exec == nil || progress.forceREST {
		return s.fetchWorkflowPhaseREST(ctx, client, cacheScope, targets, progress)
	}
	if result := s.ensureWorkflowPhaseEvidence(ctx, exec, targets, progress); result != nil {
		if result.useREST {
			progress.forceREST = true
			return s.fetchWorkflowPhaseREST(ctx, client, cacheScope, targets, progress)
		}
		return workflowPhaseSharedResult{progress: progress, deferred: result.Deferred, err: result.Err}
	}
	if result := s.ensureWorkflowPhaseSuiteCounts(ctx, exec, targets, progress); result != nil {
		if result.useREST {
			progress.forceREST = true
			return s.fetchWorkflowPhaseREST(ctx, client, cacheScope, targets, progress)
		}
		return workflowPhaseSharedResult{progress: progress, deferred: result.Deferred, err: result.Err}
	}
	return shared
}

func (s *Service) ensureWorkflowPhaseEvidence(
	ctx context.Context, exec GraphQLExecutor, targets []*workflowPhaseTarget, progress *workflowPhaseProgress,
) *workflowPhaseResult {
	refs := workflowPhaseMissingRefs(targets, progress)
	for _, chunk := range chunkRefs(refs, graphQLWorkflowSuitesChunkSize) {
		observations, err := s.fetchWorkflowSuiteChunk(ctx, exec, chunk)
		if err == nil {
			for key, observation := range observations {
				progress.evidence[key] = observation
			}
			continue
		}
		if deferred := admissionDeferralOf(err); deferred != nil {
			return &workflowPhaseResult{Unfinished: workflowPhaseWatches(targets), Deferred: deferred}
		}
		if errors.Is(err, errGraphQLUnsupported) ||
			classifyPRDiscoveryError(err) == PRDiscoveryHealthInvalidQuery {
			return &workflowPhaseResult{useREST: true}
		}
		s.logWorkflowPhaseFailure("workflow suites read unavailable", err)
		for _, ref := range chunk {
			progress.evidence[workflowSuiteTargetKey(ref.Owner, ref.Repo, ref.Number)] =
				workflowSuiteObservation{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number, Missing: true}
		}
	}
	return nil
}

func (s *Service) fetchWorkflowSuiteChunk(
	ctx context.Context, exec GraphQLExecutor, refs []graphQLPRRef,
) (map[string]workflowSuiteObservation, error) {
	query, variables := buildWorkflowSuitesQuery(refs)
	var response workflowSuitesResponse
	if err := exec.ExecuteGraphQL(ctx, query, variables, &response); err != nil {
		return nil, err
	}
	observations, retryAt, err := decodeWorkflowSuitesResponse(response, refs)
	if err != nil {
		return nil, withPRDiscoveryRetryAt(err, retryAt)
	}
	return observations, nil
}

func workflowPhaseMissingRefs(
	targets []*workflowPhaseTarget, progress *workflowPhaseProgress,
) []graphQLPRRef {
	refs := make([]graphQLPRRef, 0, len(targets))
	for _, target := range targets {
		if target.headSHA == "" {
			continue
		}
		if _, ok := progress.evidence[target.evidenceKey()]; ok {
			continue
		}
		refs = append(refs, graphQLPRRef{Owner: target.owner, Repo: target.repo, Number: target.number})
	}
	sort.Slice(refs, func(i, j int) bool {
		left, right := workflowSuiteTargetKey(refs[i].Owner, refs[i].Repo, refs[i].Number),
			workflowSuiteTargetKey(refs[j].Owner, refs[j].Repo, refs[j].Number)
		return left < right
	})
	return refs
}

func (s *Service) ensureWorkflowPhaseSuiteCounts(
	ctx context.Context, exec GraphQLExecutor, targets []*workflowPhaseTarget, progress *workflowPhaseProgress,
) *workflowPhaseResult {
	suiteIDs := workflowPhaseSuiteIDs(targets, progress)
	for _, chunk := range chunkRefs(suiteIDs, graphQLWorkflowSuiteJobsChunkSize) {
		counts, missing, err := s.fetchWorkflowSuiteJobCounts(ctx, exec, chunk)
		if err == nil {
			for id, count := range counts {
				progress.counts[id] = count
			}
			for id := range missing {
				progress.unavailable[id] = struct{}{}
			}
			continue
		}
		if deferred := admissionDeferralOf(err); deferred != nil {
			return &workflowPhaseResult{Unfinished: workflowPhaseWatches(targets), Deferred: deferred}
		}
		if errors.Is(err, errGraphQLUnsupported) ||
			classifyPRDiscoveryError(err) == PRDiscoveryHealthInvalidQuery {
			return &workflowPhaseResult{useREST: true}
		}
		s.logWorkflowPhaseFailure("workflow suite job read unavailable", err)
		for _, id := range chunk {
			progress.unavailable[id] = struct{}{}
		}
	}
	return nil
}

func (s *Service) fetchWorkflowSuiteJobCounts(
	ctx context.Context, exec GraphQLExecutor, suiteIDs []string,
) (map[string]int, map[string]struct{}, error) {
	query, variables := buildWorkflowSuiteJobsQuery(suiteIDs)
	var response workflowSuiteJobsResponse
	if err := exec.ExecuteGraphQL(ctx, query, variables, &response); err != nil {
		return nil, nil, err
	}
	counts, missing, retryAt, err := decodeWorkflowSuiteJobCounts(response, suiteIDs)
	if err != nil {
		return nil, nil, withPRDiscoveryRetryAt(err, retryAt)
	}
	return counts, missing, nil
}

func workflowPhaseSuiteIDs(
	targets []*workflowPhaseTarget, progress *workflowPhaseProgress,
) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		observation, ok := progress.evidence[target.evidenceKey()]
		if !ok || !workflowPhaseObservationUsable(target, observation) || len(observation.SuiteIDs) == 0 {
			continue
		}
		for _, id := range observation.SuiteIDs {
			if _, counted := progress.counts[id]; counted {
				continue
			}
			if _, unavailable := progress.unavailable[id]; unavailable {
				continue
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func workflowPhaseObservationUsable(target *workflowPhaseTarget, observation workflowSuiteObservation) bool {
	return !observation.Missing && !observation.HasNextPage &&
		observation.HeadSHA == target.headSHA && observation.CommitSHA == target.headSHA
}

func (s *Service) classifyAndStoreWorkflowPhase(
	ctx context.Context, targets []*workflowPhaseTarget, progress *workflowPhaseProgress,
) {
	for _, target := range targets {
		attention := s.workflowPhaseAttentionForTarget(ctx, target, progress)
		s.storeWorkflowPhaseAttention(ctx, target, attention)
	}
}

func (s *Service) workflowPhaseAttentionForTarget(
	ctx context.Context, target *workflowPhaseTarget, progress *workflowPhaseProgress,
) *WorkflowAttention {
	if attention, ok := progress.rest[target.key()]; ok {
		return cloneWorkflowAttention(attention)
	}
	observation, ok := progress.evidence[target.evidenceKey()]
	if !ok || !workflowPhaseObservationUsable(target, observation) {
		return workflowAttentionUnknown(target.headSHA)
	}
	pr := workflowPhasePullRequest(target, observation)
	return classifyWorkflowAttentionWithJobs(ctx, target.owner, target.repo, pr, observation.Runs,
		workflowPhaseJobReader(progress, observation),
	)
}

func workflowPhasePullRequest(target *workflowPhaseTarget, observation workflowSuiteObservation) *PR {
	return &PR{
		Number: target.number, State: "open", HeadSHA: target.headSHA,
		HeadBranch: observation.HeadBranch,
		RepoOwner:  target.owner, RepoName: target.repo,
		BaseRepoOwner: target.owner, BaseRepoName: target.repo,
		HeadRepoOwner: observation.HeadRepoOwner, HeadRepoName: observation.HeadRepoName,
	}
}

func workflowPhaseJobReader(
	progress *workflowPhaseProgress, observation workflowSuiteObservation,
) workflowAttentionJobReader {
	return func(_ context.Context, runID int64, _ int) ([]WorkflowJob, error) {
		suiteID, ok := observation.RunSuiteIDs[runID]
		if !ok {
			return nil, errWorkflowSuiteCountUnavailable
		}
		if _, unavailable := progress.unavailable[suiteID]; unavailable {
			return nil, errWorkflowSuiteCountUnavailable
		}
		count, ok := progress.counts[suiteID]
		if !ok {
			return nil, errWorkflowSuiteCountUnavailable
		}
		if count <= 0 {
			return nil, nil
		}
		// The classifier only distinguishes zero from non-zero jobs.
		return []WorkflowJob{{ID: -1}}, nil
	}
}

func (s *Service) storeWorkflowPhaseAttention(
	ctx context.Context, target *workflowPhaseTarget, attention *WorkflowAttention,
) {
	if attention == nil || s.store == nil {
		return
	}
	// A credential or host rotation after the phase started invalidates the
	// evidence it read under the previous principal.
	if err := requireGitHubOperationCurrent(ctx); err != nil {
		return
	}
	for _, row := range target.rows {
		stored, changed, err := s.store.UpdateTaskPRWorkflowAttention(
			ctx, row.TaskID, row.RepositoryID, target.number, target.headSHA, attention,
		)
		if err != nil {
			s.logWorkflowPhaseFailure("failed to store workflow attention", err)
			continue
		}
		if changed && stored != nil && workflowAttentionPublicationNeeded(row.Previous, attention) && s.eventBus != nil {
			if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, newTaskPRUpdatedEvent(stored)); err != nil {
				s.logWorkflowPhaseFailure("failed to publish workflow attention update", err)
			}
		}
	}
}

// workflowAttentionPublicationNeeded suppresses an event for a first
// unavailable or empty observation. Neither state changes a rendered badge, so
// no summary projection needs to re-run; a positive observation always
// publishes, as does any change from a prior observation.
func workflowAttentionPublicationNeeded(previous, next *WorkflowAttention) bool {
	if previous != nil {
		return true
	}
	if next == nil {
		return false
	}
	return next.State == WorkflowAttentionApprovalRequired || next.State == WorkflowAttentionActionRequired
}

func (s *Service) logWorkflowPhaseFailure(message string, err error) {
	if s != nil && s.logger != nil {
		s.logger.Debug(message, zap.Error(err))
	}
}

func (s *Service) fetchWorkflowPhaseREST(
	ctx context.Context, client Client, cacheScope string, targets []*workflowPhaseTarget,
	progress *workflowPhaseProgress,
) workflowPhaseSharedResult {
	shared := workflowPhaseSharedResult{progress: progress}
	if client == nil {
		return shared
	}
	for _, target := range targets {
		if _, ok := progress.rest[target.key()]; ok {
			continue
		}
		attention, err := s.collectRESTWorkflowAttention(ctx, client, cacheScope, target, progress)
		if err != nil {
			if deferred := admissionDeferralOf(err); deferred != nil {
				shared.deferred = deferred
				return shared
			}
			s.logWorkflowPhaseFailure("workflow attention read unavailable", err)
			attention = workflowAttentionUnknown(target.headSHA)
		}
		progress.rest[target.key()] = attention
	}
	return shared
}

func (s *Service) collectRESTWorkflowAttention(
	ctx context.Context, client Client, cacheScope string, target *workflowPhaseTarget,
	progress *workflowPhaseProgress,
) (*WorkflowAttention, error) {
	pr := &PR{
		Number: target.number, State: "open", HeadSHA: target.headSHA, HeadBranch: target.headBranch,
		RepoOwner: target.owner, RepoName: target.repo,
		BaseRepoOwner: target.owner, BaseRepoName: target.repo,
	}
	fetched := progress.restPRs[target.key()]
	if fetched == nil {
		var err error
		fetched, err = client.GetPR(ctx, target.owner, target.repo, target.number)
		if err != nil {
			return workflowAttentionUnknown(target.headSHA), err
		}
		if fetched != nil {
			progress.restPRs[target.key()] = fetched
		}
	}
	if fetched == nil || (fetched.HeadSHA != "" && fetched.HeadSHA != target.headSHA) {
		return workflowAttentionUnknown(target.headSHA), nil
	}
	pr.HeadBranch = fetched.HeadBranch
	pr.HeadRepoID = fetched.HeadRepoID
	pr.HeadRepoOwner = fetched.HeadRepoOwner
	pr.HeadRepoName = fetched.HeadRepoName
	return s.collectRESTWorkflowRunAttention(ctx, client, cacheScope, pr)
}

// collectRESTWorkflowRunAttention keeps a job-read deferral on the returned
// error. The classifier represents missing job evidence as unknown, but the
// fallback must distinguish a local deferral from a completed unavailable
// observation so it can resume instead of storing stale evidence.
func (s *Service) collectRESTWorkflowRunAttention(
	ctx context.Context, client Client, cacheScope string, pr *PR,
) (*WorkflowAttention, error) {
	if pr == nil || pr.HeadSHA == "" {
		return workflowAttentionUnknown(""), nil
	}
	owner, repo := pr.RepoOwner, pr.RepoName
	runs, err := s.cachedWorkflowRuns(ctx, client, cacheScope, owner, repo, pr.HeadSHA)
	if err != nil {
		return workflowAttentionUnknown(pr.HeadSHA), err
	}
	var jobErr error
	attention := classifyWorkflowAttentionWithJobs(ctx, owner, repo, pr, runs,
		func(jobCtx context.Context, runID int64, attempt int) ([]WorkflowJob, error) {
			jobs, readErr := s.cachedWorkflowJobs(jobCtx, client, cacheScope, owner, repo, runID, attempt, true)
			if readErr != nil && jobErr == nil {
				jobErr = readErr
			}
			return jobs, readErr
		},
	)
	if jobErr != nil {
		return attention, jobErr
	}
	return attention, nil
}

func (s *Service) loadWorkflowPhaseTargets(
	ctx context.Context, watches []*PRWatch,
) []*workflowPhaseTarget {
	byKey := make(map[string]*workflowPhaseTarget)
	rows := make(map[string]map[string]struct{})
	for _, watch := range watches {
		if watch == nil || watch.PRNumber <= 0 || s.store == nil {
			continue
		}
		taskID := s.resolveEffectiveAssociationTaskIDForSession(
			ctx, watch.SessionID, watch.TaskID, watch.RepositoryID,
		)
		row, err := s.store.GetTaskPRByRepoAndNumber(ctx, taskID, watch.RepositoryID, watch.PRNumber)
		if err != nil || row == nil || row.DetachedAt != nil || strings.TrimSpace(row.HeadSHA) == "" ||
			isTerminalPR(&PR{State: row.State, MergedAt: row.MergedAt}) {
			continue
		}
		key := fmt.Sprintf("%s/%s#%d@%s", strings.ToLower(row.Owner), strings.ToLower(row.Repo), row.PRNumber, row.HeadSHA)
		target := byKey[key]
		if target == nil {
			target = &workflowPhaseTarget{
				owner: row.Owner, repo: row.Repo, number: row.PRNumber,
				headSHA: row.HeadSHA, headBranch: row.HeadBranch,
			}
			byKey[key] = target
			rows[key] = make(map[string]struct{})
		}
		rowKey := taskID + "\x00" + watch.RepositoryID
		if _, exists := rows[key][rowKey]; !exists {
			rows[key][rowKey] = struct{}{}
			target.rows = append(target.rows, workflowPhaseRow{
				TaskID: taskID, RepositoryID: watch.RepositoryID, Previous: cloneWorkflowAttention(row.WorkflowAttention),
			})
		}
		target.watches = append(target.watches, watch)
	}
	targets := make([]*workflowPhaseTarget, 0, len(byKey))
	for _, target := range byKey {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].key() < targets[j].key() })
	return targets
}

func workflowPhaseRefsKey(targets []*workflowPhaseTarget) string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, target.key())
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}

func workflowPhaseWatches(targets []*workflowPhaseTarget) []*PRWatch {
	seen := make(map[*PRWatch]struct{})
	watches := make([]*PRWatch, 0, len(targets))
	for _, target := range targets {
		for _, watch := range target.watches {
			if _, exists := seen[watch]; exists {
				continue
			}
			seen[watch] = struct{}{}
			watches = append(watches, watch)
		}
	}
	return watches
}
