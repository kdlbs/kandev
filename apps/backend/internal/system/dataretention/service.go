package dataretention

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type Options struct {
	CreateBackup func(context.Context) (string, error)
	VerifyBackup func(context.Context, string) error
	Changed      func(context.Context, []string)
	Report       func(context.Context, *Operation)
	Log          func(context.Context, string, error)
	Now          func() time.Time
}

type Service struct {
	pool           *db.Pool
	opts           Options
	wake           chan struct{}
	mu             sync.Mutex
	lifecycleMu    sync.Mutex
	cancel         context.CancelFunc
	activeCancel   context.CancelFunc
	activeID       string
	activeRevision int64
	wg             sync.WaitGroup
}

func New(pool *db.Pool, options ...Options) *Service {
	o := Options{Now: time.Now}
	if len(options) > 0 {
		o = options[0]
		if o.Now == nil {
			o.Now = time.Now
		}
	}
	return &Service{pool: pool, opts: o, wake: make(chan struct{}, 1)}
}

func (s *Service) supported() bool {
	return s.pool != nil && s.pool.Writer() != nil && s.pool.Writer().DriverName() == "sqlite3"
}

func (s *Service) driverName() string {
	if s.pool == nil || s.pool.Writer() == nil {
		return ""
	}
	return s.pool.Writer().DriverName()
}

func (s *Service) Get(ctx context.Context) (Status, error) {
	if !s.supported() {
		r := defaultRecord()
		r.Supported = false
		return r.Status, nil
	}
	r, err := readRecord(ctx, s.pool.Reader())
	return r.Status, err
}

// Save validates a draft against the current revision. First enablement, and any
// change that brings a target's window forward, requires an explicit backup
// choice and arms no deletion until preparation succeeds.
func (s *Service) Save(ctx context.Context, u Update) (Status, error) {
	if err := u.ArchivedAge.Validate(); err != nil {
		return Status{}, err
	}
	if err := u.CleanupAge.Validate(); err != nil {
		return Status{}, err
	}
	if u.BackupChoice != "" && u.BackupChoice != choiceBackup && u.BackupChoice != choiceSkip {
		return Status{}, errors.New("invalid_backup_choice")
	}
	if _, err := s.interruptValidated(ctx, func(r *record) error {
		_, err := validatePolicyUpdate(r, u, s.opts.Now())
		return err
	}); err != nil {
		return Status{}, err
	}
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		needsReview, err := validatePolicyUpdate(r, u, s.opts.Now())
		if err != nil {
			return err
		}
		if r.Operation != nil && r.Operation.State == stateRunning {
			finishOperation(r, stateCancelled, "policy_changed", s.opts.Now())
		}
		r.Policy = Policy{Enabled: u.Enabled, ArchivedAge: u.ArchivedAge, CleanupAge: u.CleanupAge, Revision: u.Revision + 1}
		r.Progress = progress{}
		applyPolicyState(r, needsReview, u.BackupChoice, s.opts.Now())
		return nil
	})
	if err == nil {
		s.cancelOlder(r.Policy.Revision)
		s.notify()
	}
	return r.Status, err
}

func applyPolicyState(r *record, needsReview bool, choice string, now time.Time) {
	switch {
	case !r.Policy.Enabled:
		r.Preparation = Preparation{State: stateNone}
		r.PreparationDetail = ""
		r.ApprovedRevision = 0
		r.Receipt = ""
		r.NextDueAt = nil
	case needsReview:
		// A pending choice arms nothing: the first mutation still requires a
		// verified backup or an explicit skip receipt.
		r.Policy.Enabled = false
		r.Preparation = Preparation{State: statePending, Choice: choice}
		r.PreparationDetail = ""
		r.ApprovedRevision = 0
		r.Receipt = ""
		r.FirstMutation = false
		r.NextDueAt = nil
	default:
		r.ApprovedRevision = r.Policy.Revision
		if r.NextDueAt == nil {
			next := now.Add(24 * time.Hour)
			r.NextDueAt = &next
		}
	}
}

// validatePolicyUpdate reports whether the change needs a backup review. A first
// enablement always reviews; afterwards only shortening a window does, because
// that removes data sooner than the operator already approved.
func validatePolicyUpdate(r *record, u Update, now time.Time) (bool, error) {
	if r.Policy.Revision != u.Revision {
		return false, errors.New("conflict")
	}
	if !u.Enabled {
		return false, nil
	}
	review := !r.Policy.Enabled ||
		u.ArchivedAge.Cutoff(now).After(r.Policy.ArchivedAge.Cutoff(now)) ||
		u.CleanupAge.Cutoff(now).After(r.Policy.CleanupAge.Cutoff(now))
	if review && u.BackupChoice == "" {
		return false, errors.New("backup_choice_required")
	}
	return review, nil
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) logFailure(ctx context.Context, message string, err error) {
	if s.opts.Log != nil {
		s.opts.Log(ctx, message, err)
	}
}

func (s *Service) cancelOlder(revision int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeCancel != nil && s.activeRevision < revision {
		s.activeCancel()
	}
}

func (s *Service) cancelOperation(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeCancel != nil && s.activeID == id {
		s.activeCancel()
	}
}

// interruptValidated validates through the reader before interrupting work that
// may hold the writer. Callers must repeat their durable-state checks inside the
// write transaction.
func (s *Service) interruptValidated(ctx context.Context, validate func(*record) error) (record, error) {
	if !s.supported() {
		return record{}, errors.New("unsupported")
	}
	r, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return r, err
	}
	if err := validate(&r); err != nil {
		return r, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if s.activeCancel == nil || s.activeRevision != r.Policy.Revision {
		return r, nil
	}
	if r.Preparation.State == statePending {
		return r, s.interruptPreparationHandoff(ctx, r)
	}
	if r.Operation == nil || r.Operation.State != stateRunning {
		return r, nil
	}
	if s.activeID == r.Operation.ID {
		s.activeCancel()
		return r, nil
	}
	return r, s.interruptPreparationHandoff(ctx, r)
}

// The backup operation is committed before its id reaches the worker, so the
// handoff re-reads durable identity while holding mu before cancelling.
func (s *Service) interruptPreparationHandoff(ctx context.Context, observed record) error {
	pending := observed.Preparation.State == statePending
	if !pending && (observed.Preparation.State != stateRunning || observed.Operation.Kind != choiceBackup) {
		return nil
	}
	current, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	if current.Policy.Revision != observed.Policy.Revision {
		return errors.New("conflict")
	}
	if pending && current.Preparation.State == statePending {
		s.activeCancel()
		return nil
	}
	if current.Operation == nil || current.Operation.State != stateRunning ||
		current.Preparation.State != stateRunning || current.Operation.Kind != choiceBackup ||
		(!pending && current.Operation.ID != observed.Operation.ID) {
		return errors.New("conflict")
	}
	s.activeCancel()
	return nil
}

func approved(r *record) bool {
	return r.Policy.Enabled && r.Preparation.State == stateReady && r.ApprovedRevision == r.Policy.Revision &&
		(r.Preparation.Choice == choiceSkip || r.Receipt != "")
}

func operationBusy(r *record) bool {
	return (r.Operation != nil && r.Operation.State == stateRunning) ||
		r.Preparation.State == statePending || r.Preparation.State == stateRunning
}

func operationMatches(r *record, id string) bool {
	return r.Operation != nil && r.Operation.ID == id
}

func finishOperation(r *record, state, code string, now time.Time) {
	if r.Operation == nil {
		return
	}
	r.Operation.State = state
	r.Operation.Error = code
	r.Operation.FinishedAt = &now
	if r.Operation.Kind == kindAnalysis {
		r.LastAnalysis = r.Operation
	}
	if r.Operation.Kind == kindCleanup {
		r.LastRun = r.Operation
	}
}

// startOperation captures the schema version and both upper cursors before any
// row is touched, so a migration or a concurrent insert mid-pass ends the pass
// instead of widening or skipping its window.
func (s *Service) startOperation(ctx context.Context, q sqlx.QueryerContext, r *record, kind string, archivedCutoff, cleanupCutoff, now time.Time) error {
	var schemaVersion int
	if err := scanGet(ctx, q, &schemaVersion, `PRAGMA schema_version`); err != nil {
		return err
	}
	upperTask, err := tasksqlite.HistoryUpperTaskID(ctx, q)
	if err != nil {
		return err
	}
	upperJob, err := tasksqlite.SucceededCleanupUpperJobID(ctx, q)
	if err != nil {
		return err
	}
	r.Operation = &Operation{
		ID:             newOperationID(),
		Kind:           kind,
		State:          stateRunning,
		ArchivedCutoff: archivedCutoff,
		CleanupCutoff:  cleanupCutoff,
		StartedAt:      now,
		Skipped:        map[string]int64{},
		PolicyRevision: r.Policy.Revision,
	}
	r.Progress = progress{
		SchemaVersion: schemaVersion,
		UpperTask:     upperTask,
		UpperJob:      upperJob,
		Phase:         phaseMessages,
		Revision:      r.Policy.Revision,
		Started:       true,
	}
	return nil
}
