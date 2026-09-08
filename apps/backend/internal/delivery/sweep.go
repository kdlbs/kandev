package delivery

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// Boundary values fixed by spec "Evaluation triggers and cadence §
// Boundary values" so they are not invented downstream.
const (
	// SweepInterval is completion-relative: the next pass starts this long
	// after the previous pass finished, never fixed-rate.
	SweepInterval = 5 * time.Minute
	// StaleRefreshInterval is the "Stale refresh" due condition's window.
	StaleRefreshInterval = 24 * time.Hour
	// StallThreshold is the writer-health stall threshold.
	StallThreshold = 15 * time.Minute
)

// Sweep owns the periodic evaluation loop: Start/Stop lifecycle,
// completion-relative cadence, and no overlapping passes — a single
// goroutine running one pass at a time (spec "Boundary values",
// "Concurrent sweep passes: never"). Mirrors the healthpoll.Poller
// Start/Stop shape.
type Sweep struct {
	repo     *Repository
	ancestry *AncestryChecker
	log      *logger.Logger

	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
}

// NewSweep constructs a Sweep. ancestry may be nil, in which case the
// ancestry check is never attempted and every pair's default-branch
// observation is limited to the provider/direct-commit bases.
func NewSweep(repo *Repository, ancestry *AncestryChecker, log *logger.Logger) *Sweep {
	return &Sweep{repo: repo, ancestry: ancestry, log: log}
}

// Start launches the background loop, running the first pass immediately
// (spec: "First sweep: runs at boot ... with no initial delay"). Calling
// Start more than once without Stop is a no-op.
func (s *Sweep) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go s.loop(ctx)
}

// Stop cancels the loop and waits for the in-flight pass, if any, to
// finish.
func (s *Sweep) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()
}

func (s *Sweep) loop(ctx context.Context) {
	defer s.wg.Done()
	s.RunPass(ctx)
	for {
		timer := time.NewTimer(SweepInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.RunPass(ctx)
		}
	}
}

// RunPass executes one complete sweep pass: candidacy, dueness (with the
// missing-ledger fallback), ordering, per-pair evaluation, and the
// writer-health publication at the end. Exported so tests and a manual
// trigger can drive a single deterministic pass.
func (s *Sweep) RunPass(ctx context.Context) {
	passStart := time.Now().UTC()

	candidates, err := s.repo.Candidates(ctx)
	if err != nil {
		s.warn("delivery_ledger.candidates_unavailable", zap.Error(err))
		return
	}
	setPairsMissingRepositoryGauge(candidates.MissingRepository)
	setPairsMissingTaskGauge(candidates.MissingTask)

	due, fallback, ledgerErr := s.repo.SelectDuePairs(ctx, candidates.Pairs, passStart)
	if fallback {
		s.warn("delivery_ledger.dueness_unavailable", zap.Error(ledgerErr), zap.Int("candidates", len(candidates.Pairs)))
	}

	ordered := s.repo.OrderPairs(ctx, due)

	writeErrors := 0
	var lastWriteErr error
	for _, pair := range ordered {
		if err := s.evaluatePair(ctx, pair); err != nil {
			writeErrors++
			lastWriteErr = err
		}
	}
	if writeErrors > 0 {
		s.warn("delivery_ledger.write_failed", zap.Error(lastWriteErr), zap.Int("count", writeErrors))
	}

	if n, err := s.repo.CountUnattributedSessions(ctx); err == nil {
		setSessionsUnattributedGauge(n)
	}

	s.publishWriterHealth(ctx)
}

func (s *Sweep) publishWriterHealth(ctx context.Context) {
	sig := s.repo.ComputeStallSignal(ctx)
	publishStallSignal(sig)
	switch {
	case sig.LedgerErr != nil:
		s.warn("delivery_ledger.unavailable", zap.Error(sig.LedgerErr))
	case sig.ComparandErr != nil:
		s.warn("delivery_ledger.comparand_unavailable", zap.Error(sig.ComparandErr))
	case sig.StallSeconds > int64(StallThreshold.Seconds()):
		s.warn("delivery_ledger.stalled",
			zap.Int64("last_evaluated_unix", sig.LastEvaluatedUnix),
			zap.Int64("stall_seconds", sig.StallSeconds))
	}
}

func (s *Sweep) warn(msg string, fields ...zap.Field) {
	if s.log != nil {
		s.log.Warn(msg, fields...)
	}
}

// evaluatePair gathers every input for one pair, computes the ancestry
// precondition/check, classifies, and upserts. Returns an error only for
// the upsert-failure mode (spec "Failure modes"); an input-query failure
// abandons the evaluation (no column written, pair stays due) and is
// counted by delivery_ledger_evaluation_errors_total, not returned as a
// pass-level write error.
func (s *Sweep) evaluatePair(ctx context.Context, pair CandidatePair) error {
	evaluatedAt := time.Now().UTC() // T0: the instant this evaluation began reading its inputs

	snapshots, err := s.repo.SnapshotsForPair(ctx, pair.TaskID, pair.RepositoryID)
	if err != nil {
		recordEvaluationError()
		return nil
	}
	providers, err := s.repo.ProvidersForPair(ctx, pair.TaskID, pair.RepositoryID)
	if err != nil {
		recordEvaluationError()
		return nil
	}
	repoInfo, err := s.repo.RepositoryInfo(ctx, pair.RepositoryID)
	if err != nil {
		recordEvaluationError()
		return nil
	}
	// Re-check the freeze: SelectDuePairs enforced it at due-selection
	// time, but the two reads are not in the same transaction, so a
	// repository soft-deleted in the window between due-selection and
	// this evaluation must still be honored here (Review round 1,
	// finding #5) — otherwise the freeze promised under "Persistence
	// guarantees" can be silently voided by an ordinary race.
	if repoInfo.DeletedAt != nil {
		return nil
	}
	taskInfo, err := s.repo.TaskInfo(ctx, pair.TaskID)
	if err != nil {
		recordEvaluationError()
		return nil
	}

	ancestry := s.runAncestryIfDue(ctx, pair.RepositoryID, repoInfo.DefaultBranch, snapshots)

	classification := Classify(PairInput{
		DefaultBranch: repoInfo.DefaultBranch,
		Snapshots:     snapshots,
		Providers:     providers,
		Ancestry:      ancestry,
	})

	result, err := s.repo.Upsert(ctx, UpsertInput{
		TaskID: pair.TaskID, RepositoryID: pair.RepositoryID, WorkspaceID: taskInfo.WorkspaceID,
		Classification: classification, EvaluatedAt: evaluatedAt,
	})
	if err != nil {
		if errors.Is(err, errRepositoryNotLive) {
			return nil
		}
		recordWriteError()
		return err
	}
	// recordEvaluation counts persisted evaluations only, per spec's
	// "once per persisted evaluation" definition — it must run after a
	// successful Upsert, not before (Review round 1, finding #2).
	recordEvaluation(classification.Outcome)
	if result.RowChanged {
		recordRowWritten()
	}
	if result.Demoted {
		recordDemotionSuppressed()
	}
	if result.DegradedOutcomeChanged {
		recordDegradedOutcomeChanged()
	}
	return nil
}

// runAncestryIfDue applies the ancestry precondition (spec "Default-
// branch observation § Ancestry precondition") and, when met, runs the
// check. Suspended entirely while degraded (empty default branch), which
// is neither a skip nor an error and is not counted by either counter.
func (s *Sweep) runAncestryIfDue(ctx context.Context, repositoryID, defaultBranch string, snapshots []Snapshot) AncestryOutcome {
	if defaultBranch == "" || s.ancestry == nil {
		return AncestryOutcome{}
	}
	if !AncestryPrecondition(snapshots) {
		recordAncestrySkipped()
		return AncestryOutcome{}
	}
	commit := SelectAncestryHead(snapshots)
	if commit == "" {
		return AncestryOutcome{}
	}
	out := s.ancestry.Check(ctx, repositoryID, defaultBranch, commit)
	if out.Errored {
		recordAncestryError()
	}
	return out
}

// SelectDuePairs applies the freeze and the three due conditions (spec
// "Sweep selection predicate"), including the two-stage dueness fallback
// under "When the ledger itself cannot be read": if the bulk ledger read
// fails, every non-frozen candidate is treated as due. The returned error
// is the underlying ledger-read failure that triggered the fallback (nil
// when fallback is false), so a caller logging the fallback can carry the
// actual error rather than just the fact that one occurred (spec "Sweep
// selection predicate": the dueness_unavailable warning "carries the error
// and the candidate count").
func (r *Repository) SelectDuePairs(ctx context.Context, candidates []CandidatePair, passStart time.Time) ([]CandidatePair, bool, error) {
	ledgerRows, ledgerErr := r.allLedgerRows(ctx)
	fallback := ledgerErr != nil
	staleRefreshBoundary := passStart.Add(-StaleRefreshInterval)

	var due []CandidatePair
	for _, pair := range candidates {
		repoInfo, err := r.RepositoryInfo(ctx, pair.RepositoryID)
		if err != nil {
			// Not a spec-defined evaluation_errors_total case: this pair
			// never reached evaluatePair, so it is not an "abandoned
			// evaluation" under that counter's definition. Log only —
			// self-healing, since Candidates()/SelectDuePairs reruns every
			// pass and will pick the pair back up.
			r.logWarn("delivery_ledger.due_selection_repository_info_failed", err)
			continue
		}
		if repoInfo.DeletedAt != nil {
			continue // freeze evaluated first, overrides every due condition
		}
		if fallback {
			due = append(due, pair)
			continue
		}
		if r.isDue(ctx, pair, ledgerRows, repoInfo, staleRefreshBoundary) {
			due = append(due, pair)
		}
	}
	return due, fallback, ledgerErr
}

func (r *Repository) isDue(
	ctx context.Context, pair CandidatePair, ledgerRows map[pairKey]dueLedgerInfo,
	repoInfo RepositoryInfo, staleRefreshBoundary time.Time,
) bool {
	existing, hasRow := ledgerRows[pairKey{pair.TaskID, pair.RepositoryID}]
	if !hasRow {
		return true
	}
	// Fail OPEN on a per-pair query error here, consistent with
	// SelectDuePairs's own bulk-ledger-read fallback one level up
	// (Review round 1, finding #3): these queries read the same input
	// tables spec "Failure modes" names under "an input query fails",
	// so a transient failure here must not silently exclude a pair with
	// zero observability. Selecting it as due lets evaluatePair's own
	// read of the same tables discover and count the failure via
	// delivery_ledger_evaluation_errors_total.
	taskInfo, err := r.TaskInfo(ctx, pair.TaskID)
	if err != nil {
		return true
	}
	mostRecent, err := r.mostRecentInputObservation(ctx, pair.TaskID, pair.RepositoryID, repoInfo.UpdatedAt, taskInfo.UpdatedAt)
	if err != nil {
		return true
	}
	if existing.LastEvaluatedAt.Before(mostRecent) {
		return true
	}
	if existing.ReachedDefaultAt == nil && existing.LastEvaluatedAt.Before(staleRefreshBoundary) {
		return true
	}
	return false
}

type dueLedgerInfo struct {
	EvidenceRank     int
	LastEvaluatedAt  time.Time
	ReachedDefaultAt *time.Time
}

// allLedgerRows bulk-reads the ledger in one query, so a missing table
// fails once for the whole pass rather than per-candidate (spec "When the
// ledger itself cannot be read").
func (r *Repository) allLedgerRows(ctx context.Context) (map[pairKey]dueLedgerInfo, error) {
	var rows []struct {
		TaskID           string       `db:"task_id"`
		RepositoryID     string       `db:"repository_id"`
		EvidenceRank     int          `db:"evidence_rank"`
		LastEvaluatedAt  time.Time    `db:"last_evaluated_at"`
		ReachedDefaultAt sql.NullTime `db:"reached_default_at"`
	}
	err := r.ro.SelectContext(ctx, &rows, `
		SELECT task_id, repository_id, evidence_rank, last_evaluated_at, reached_default_at
		FROM task_delivery_ledger
	`)
	if err != nil {
		return nil, err
	}
	out := make(map[pairKey]dueLedgerInfo, len(rows))
	for _, row := range rows {
		info := dueLedgerInfo{EvidenceRank: row.EvidenceRank, LastEvaluatedAt: row.LastEvaluatedAt}
		if row.ReachedDefaultAt.Valid {
			t := row.ReachedDefaultAt.Time
			info.ReachedDefaultAt = &t
		}
		out[pairKey{row.TaskID, row.RepositoryID}] = info
	}
	return out, nil
}

// mostRecentInputObservationQueries enumerates every per-pair source in
// spec "Sweep selection predicate"'s due-source table other than the task
// and repository rows themselves (passed in directly, since the caller
// already has them). Tolerant of a missing provider table.
var mostRecentInputObservationQueries = []string{
	`SELECT MAX(updated_at) FROM task_sessions WHERE task_id = ? AND repository_id = ?`,
	`SELECT MAX(updated_at) FROM task_repositories WHERE task_id = ? AND repository_id = ?`,
	`SELECT MAX(updated_at) FROM github_task_prs WHERE task_id = ? AND repository_id = ?`,
	`SELECT MAX(updated_at) FROM gitlab_task_mrs WHERE task_id = ? AND repository_id = ?`,
	`SELECT MAX(updated_at) FROM azure_devops_task_prs WHERE task_id = ? AND repository_id = ?`,
}

func mostRecentGitSnapshotObservationQuery(driver string) string {
	repositoryNameExpr := "COALESCE(" + dialect.JSONExtract(driver, "g.metadata", "repository_name") + ", '')"
	return `SELECT MAX(g.created_at) FROM task_session_git_snapshots g ` +
		`JOIN task_environments e ON e.id = g.task_environment_id ` +
		`JOIN task_environment_repos er ON er.task_environment_id = g.task_environment_id ` +
		`JOIN repositories repository ON repository.id = er.repository_id ` +
		`WHERE (e.task_id = ? OR EXISTS (SELECT 1 FROM task_sessions binding ` +
		`WHERE binding.task_environment_id = g.task_environment_id AND binding.task_id = ?) ` +
		`) AND er.repository_id = ? AND er.deleted_at IS NULL ` +
		`AND (` + repositoryNameExpr + ` = repository.name ` +
		`OR EXISTS (SELECT 1 FROM task_sessions provenance ` +
		`WHERE provenance.id = g.session_id AND provenance.repository_id = er.repository_id) ` +
		`OR (` + repositoryNameExpr + ` = '' AND NOT EXISTS (` +
		`SELECT 1 FROM task_environment_repos other ` +
		`WHERE other.task_environment_id = g.task_environment_id ` +
		`AND other.repository_id <> er.repository_id AND other.deleted_at IS NULL)))`
}

func (r *Repository) mostRecentInputObservation(
	ctx context.Context, taskID, repositoryID string, repoUpdatedAt, taskUpdatedAt time.Time,
) (time.Time, error) {
	max := repoUpdatedAt
	if taskUpdatedAt.After(max) {
		max = taskUpdatedAt
	}
	queries := make([]string, 0, len(mostRecentInputObservationQueries)+1)
	queries = append(queries, mostRecentGitSnapshotObservationQuery(r.ro.DriverName()))
	queries = append(queries, mostRecentInputObservationQueries...)
	for index, q := range queries {
		var raw interface{}
		args := []interface{}{taskID, repositoryID}
		if index == 0 {
			args = []interface{}{taskID, taskID, repositoryID}
		}
		err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(q), args...).Scan(&raw)
		if err != nil {
			if dbutil.IsMissingTableError(err) {
				continue
			}
			return time.Time{}, err
		}
		t, ok, err := scanAggregateTime(raw)
		if err != nil {
			return time.Time{}, err
		}
		if ok && t.After(max) {
			max = t
		}
	}
	return max, nil
}

// OrderPairs sorts due pairs by tasks.created_at ascending, then
// tasks.id ascending, then repositories.id ascending (spec "Ordering"). A
// pair whose task has since vanished is dropped rather than sorted on a
// missing created_at.
func (r *Repository) OrderPairs(ctx context.Context, pairs []CandidatePair) []CandidatePair {
	type item struct {
		pair      CandidatePair
		createdAt time.Time
	}
	items := make([]item, 0, len(pairs))
	for _, p := range pairs {
		info, err := r.TaskInfo(ctx, p.TaskID)
		if err != nil {
			// Same rationale as SelectDuePairs' RepositoryInfo read above:
			// not an "abandoned evaluation" (the pair never reached
			// evaluatePair), so log rather than count. Self-healing next
			// pass.
			r.logWarn("delivery_ledger.ordering_task_info_failed", err)
			continue
		}
		items = append(items, item{pair: p, createdAt: info.CreatedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].createdAt.Equal(items[j].createdAt) {
			return items[i].createdAt.Before(items[j].createdAt)
		}
		if items[i].pair.TaskID != items[j].pair.TaskID {
			return items[i].pair.TaskID < items[j].pair.TaskID
		}
		return items[i].pair.RepositoryID < items[j].pair.RepositoryID
	})
	out := make([]CandidatePair, len(items))
	for i, it := range items {
		out[i] = it.pair
	}
	return out
}
