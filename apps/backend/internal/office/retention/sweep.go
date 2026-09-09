package retention

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/db"
)

// TableSweepResult is one reported table's outcome for one sweep
// (AC-OFFICE-RUN-HISTORY-RETENTION-004.6). Backlog is only ever true for a
// swept table's own SweptTableResult; a satellite table's Backlog stays
// false because its deletion is never independently batch-limited — it
// deletes exactly the run ids its owning runs batch selected.
type TableSweepResult struct {
	Deleted int64  `json:"deleted"`
	Backlog bool   `json:"backlog"`
	Err     string `json:"error"`
}

// SweptTableResult adds preview reporting to a swept table's outcome.
// Previewed is true only when THIS sweep was a preview pass for this
// table, never a running total.
type SweptTableResult struct {
	TableSweepResult
	Previewed   bool  `json:"previewed"`
	WouldDelete int64 `json:"would_delete"`
}

// LastSweep is the in-memory value replaced wholesale at the end of each
// sweep that actually ran (AC-OFFICE-RUN-HISTORY-RETENTION-004.6, -004.7).
// Nothing here is persisted.
type LastSweep struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`

	OfficeRoutineRuns SweptTableResult `json:"office_routine_runs"`
	Runs              SweptTableResult `json:"runs"`
	RunEvents         TableSweepResult `json:"run_events"`
	RouteAttempts     TableSweepResult `json:"route_attempts"`
	RunSkills         TableSweepResult `json:"run_skills"`
}

type satelliteResults struct {
	RunEvents     TableSweepResult
	RouteAttempts TableSweepResult
	RunSkills     TableSweepResult
}

// Sweeper runs one sweep or one census pass at a time; the scheduler
// (scheduler.go) owns when to call each. Findings resolved here, as
// documented Build decisions:
//
//   - F28 (lost-exclusivity outcome): a lock lost between tables abandons
//     the whole sweep attempt as a skip — the same outcome as a local
//     concurrent-sweep collision — rather than inventing a per-table
//     "skipped" state the design's eight-id health catalogue has nowhere
//     to report. Batches already committed on PostgreSQL stay committed
//     (AC-002.5); LastSweep simply is not replaced by this attempt, and
//     the next scheduled sweep reports the tables' true state either way.
//   - The skip counter increments for a collision, a failed settings
//     re-read, a failed lock acquisition, and a lock lost mid-sweep — every
//     case where a sweep was due and did not produce a result. It does
//     NOT increment when retention is disabled (AC-002.8's "run no sweep"
//     is a deliberate, steady-state condition, not a due sweep that could
//     not run; incrementing forever while off would make the counter
//     meaningless).
type Sweeper struct {
	pool          *db.Pool
	store         *Store
	settingsStore *SettingsStore
	previewMarker *PreviewMarkerStore
	census        *CensusTracker

	mu        sync.Mutex
	sweeping  bool
	lastSweep *LastSweep
	skipCount int64
	lastSkip  time.Time
}

// NewSweeper wires the sweep orchestration to its dependencies.
func NewSweeper(pool *db.Pool, store *Store, settingsStore *SettingsStore, previewMarker *PreviewMarkerStore) *Sweeper {
	return &Sweeper{
		pool:          pool,
		store:         store,
		settingsStore: settingsStore,
		previewMarker: previewMarker,
		census:        NewCensusTracker(),
	}
}

// LastSweepSnapshot returns the most recent completed sweep's result. ok is
// false before the first sweep ever completes (AC-OFFICE-RUN-HISTORY-RETENTION-004.7).
func (s *Sweeper) LastSweepSnapshot() (LastSweep, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastSweep == nil {
		return LastSweep{}, false
	}
	return *s.lastSweep, true
}

// SkipSnapshot returns the running skip count and the last skip's time.
func (s *Sweeper) SkipSnapshot() (count int64, lastAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.skipCount, s.lastSkip
}

// CensusSnapshot returns the current per-table retained counts.
func (s *Sweeper) CensusSnapshot() RetainedCounts {
	return s.census.Snapshot()
}

// RunSweep performs at most one sweep: office_routine_runs, then runs with
// its satellites, in that fixed order (AC-OFFICE-RUN-HISTORY-RETENTION-002.6).
func (s *Sweeper) RunSweep(ctx context.Context) {
	if !s.beginSweep() {
		s.recordSkip()
		return
	}
	defer s.endSweep()

	settings, err := s.settingsStore.GetSettingsForSweep(ctx)
	if err != nil {
		// Settings unreadable at sweep start fails closed: no sweep, no
		// fallback to the (possibly shorter) default window.
		s.recordSkip()
		return
	}
	if !settings.Enabled {
		// AC-002.8: disabled means no sweep at all, not a recorded skip.
		return
	}

	q, session, ok := s.acquireQueryer(ctx)
	if !ok {
		s.recordSkip()
		return
	}
	if session != nil {
		defer session.release()
	}

	now := time.Now().UTC()
	report := LastSweep{StartedAt: now}
	report.OfficeRoutineRuns = s.sweepRoutineRuns(ctx, q, settings.RoutineRuns, now, settings.BatchLimit)

	if testBetweenTablesSweep != nil {
		testBetweenTablesSweep(q)
	}
	if session != nil && !session.alive(ctx) {
		// F25/F28: exclusivity was lost after the first table's work. Do
		// not start the second table, and do not publish a partial
		// result — this whole attempt is a skip, exactly as if the lock
		// had never been acquired.
		s.recordSkip()
		return
	}

	runsResult, satellites := s.sweepRuns(ctx, q, settings.Runs, now, settings.BatchLimit)
	report.Runs = runsResult
	report.RunEvents = satellites.RunEvents
	report.RouteAttempts = satellites.RouteAttempts
	report.RunSkills = satellites.RunSkills

	report.FinishedAt = time.Now().UTC()
	s.mu.Lock()
	s.lastSweep = &report
	s.mu.Unlock()

	incSweepCompleted()
	incDeleted(TableOfficeRoutineRuns, report.OfficeRoutineRuns.Deleted)
	incDeleted(TableRuns, report.Runs.Deleted)
	incDeleted(TableRunEvents, report.RunEvents.Deleted)
	incDeleted("office_run_route_attempts", report.RouteAttempts.Deleted)
	incDeleted("office_run_skills", report.RunSkills.Deleted)
}

// testBetweenTablesSweep, when set, runs right after office_routine_runs'
// table work and right before the alive() liveness check and the runs
// table — a deterministic seam for exercising AC-OFFICE-RUN-HISTORY-RETENTION-002.12's
// "verifies the lock connection is still alive between tables" path (F25)
// without depending on real cross-process timing. It receives the sweep's
// own queryer so a test can run diagnostics (or a second sweep attempt)
// against the exact connection in use. Never set outside tests.
var testBetweenTablesSweep func(q queryer)

// RunCensus evaluates the retained-count census for every thresholded
// table. Read-only, so it needs no advisory lock: every backend computes
// and serves its own local view (F35).
func (s *Sweeper) RunCensus(ctx context.Context) {
	q := s.pool.Reader()
	now := time.Now().UTC()

	routineCensus, err := s.store.CensusRoutineRuns(ctx, q, now)
	s.census.RecordRoutineRuns(routineCensus, err)
	incCensus(TableOfficeRoutineRuns, err)

	runsCensus, err := s.store.CensusRuns(ctx, q, now)
	s.census.RecordRuns(runsCensus, err)
	incCensus(TableRuns, err)

	runEventsCensus, err := s.store.CensusRunEvents(ctx, q, now)
	s.census.RecordRunEvents(runEventsCensus, err)
	incCensus(TableRunEvents, err)
}

func (s *Sweeper) beginSweep() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sweeping {
		return false
	}
	s.sweeping = true
	return true
}

func (s *Sweeper) endSweep() {
	s.mu.Lock()
	s.sweeping = false
	s.mu.Unlock()
}

func (s *Sweeper) recordSkip() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipCount++
	s.lastSkip = time.Now().UTC()
	incSweepSkipped()
}

func (s *Sweeper) acquireQueryer(ctx context.Context) (queryer, *sweepSession, bool) {
	if !s.store.IsPostgres() {
		return s.pool.Writer(), nil, true
	}
	session, ok, err := acquireSweepSession(ctx, s.pool)
	if err != nil || !ok {
		return nil, nil, false
	}
	return session.queryer(), session, true
}

// isPreviewed reads the preview marker through q, the sweep's own
// connection, rather than the shared settings pool (F27 — see the
// PreviewMarkerStore.GetWith doc comment).
func (s *Sweeper) isPreviewed(ctx context.Context, q queryer, table TableName) bool {
	marker, _ := s.previewMarker.GetWith(ctx, q)
	_, ok := marker[table]
	return ok
}

func (s *Sweeper) sweepRoutineRuns(ctx context.Context, q queryer, cfg TableSettings, now time.Time, batchLimit int) SweptTableResult {
	if !s.isPreviewed(ctx, q, TableOfficeRoutineRuns) {
		return s.previewTable(ctx, q, TableOfficeRoutineRuns, func() (int64, error) {
			return s.store.CountEligibleRoutineRuns(ctx, q, now, cfg.FloorPerOwner)
		}, now)
	}

	eligible, err := s.store.CountEligibleRoutineRuns(ctx, q, now, cfg.FloorPerOwner)
	if err != nil {
		return SweptTableResult{TableSweepResult: TableSweepResult{Err: err.Error()}}
	}
	deleted, err := s.store.DeleteRoutineRunsBatch(ctx, q, now, cfg.FloorPerOwner, batchLimit)
	if err != nil {
		return SweptTableResult{TableSweepResult: TableSweepResult{Err: err.Error()}}
	}
	return SweptTableResult{TableSweepResult: TableSweepResult{
		Deleted: deleted,
		Backlog: eligible > int64(batchLimit),
	}}
}

func (s *Sweeper) sweepRuns(ctx context.Context, q queryer, cfg TableSettings, now time.Time, batchLimit int) (SweptTableResult, satelliteResults) {
	if !s.isPreviewed(ctx, q, TableRuns) {
		result := s.previewTable(ctx, q, TableRuns, func() (int64, error) {
			return s.store.CountEligibleRuns(ctx, q, now, cfg.FloorPerOwner)
		}, now)
		return result, satelliteResults{}
	}

	eligible, err := s.store.CountEligibleRuns(ctx, q, now, cfg.FloorPerOwner)
	if err != nil {
		return SweptTableResult{TableSweepResult: TableSweepResult{Err: err.Error()}}, satelliteResults{}
	}
	result, err := s.store.DeleteRunBatch(ctx, q, now, cfg.FloorPerOwner, batchLimit)
	if err != nil {
		return SweptTableResult{TableSweepResult: TableSweepResult{Err: err.Error()}}, satelliteResults{}
	}
	if result.Abandoned {
		// AC-002.7: an abandoned batch is that table's failure, not
		// backlog; the satellites report zero, matching what the
		// rollback actually left committed.
		return SweptTableResult{TableSweepResult: TableSweepResult{
			Err: "batch abandoned after two consecutive mismatches",
		}}, satelliteResults{}
	}
	return SweptTableResult{TableSweepResult: TableSweepResult{
		Deleted: result.RunsDeleted,
		Backlog: eligible > int64(batchLimit),
	}}, satelliteResults{
		RunEvents:     TableSweepResult{Deleted: result.RunEventsDeleted},
		RouteAttempts: TableSweepResult{Deleted: result.RouteAttemptsDeleted},
		RunSkills:     TableSweepResult{Deleted: result.RunSkillsDeleted},
	}
}

// previewTable runs a table's first-ever preview pass: count eligible rows
// uncapped, mark the table previewed, and delete nothing
// (AC-OFFICE-RUN-HISTORY-RETENTION-003.2, -003.9).
func (s *Sweeper) previewTable(ctx context.Context, q queryer, table TableName, countEligible func() (int64, error), now time.Time) SweptTableResult {
	count, err := countEligible()
	if err != nil {
		return SweptTableResult{TableSweepResult: TableSweepResult{Err: err.Error()}}
	}
	if err := s.previewMarker.MarkCompletedWith(ctx, q, table, now); err != nil {
		return SweptTableResult{TableSweepResult: TableSweepResult{Err: err.Error()}}
	}
	return SweptTableResult{Previewed: true, WouldDelete: count}
}
