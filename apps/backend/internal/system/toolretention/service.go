package toolretention

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"sync"
	"time"
)

type Options struct {
	CreateBackup func(context.Context) (string, error)
	VerifyBackup func(context.Context, string) error
	Changed      func(context.Context, []string)
	Report       func(context.Context, *Operation)
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

func (s *Service) Get(ctx context.Context) (Status, error) {
	if !s.supported() {
		r := defaultRecord()
		r.Supported = false
		return r.Status, nil
	}
	r, err := readRecord(ctx, s.pool.Reader())
	return r.Status, err
}

func (s *Service) Save(ctx context.Context, u Update) (Status, error) {
	if err := u.Age.Validate(); err != nil {
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
		r.Policy = Policy{Enabled: u.Enabled, Age: u.Age, Revision: u.Revision + 1}
		r.Progress = progress{}
		switch {
		case !u.Enabled:
			r.Preparation = Preparation{State: stateNone}
			r.ApprovedRevision = 0
			r.Receipt = ""
			r.NextDueAt = nil
		case needsReview:
			r.Policy.Enabled = false
			r.Preparation = Preparation{State: statePending, Choice: u.BackupChoice}
			r.ApprovedRevision = 0
			r.Receipt = ""
			r.FirstMutation = false
			r.NextDueAt = nil
		default:
			r.ApprovedRevision = r.Policy.Revision
			if r.NextDueAt == nil {
				next := s.opts.Now().Add(24 * time.Hour)
				r.NextDueAt = &next
			}
		}
		return nil
	})
	if err == nil {
		s.cancelOlder(r.Policy.Revision)
		s.notify()
	}
	return r.Status, err
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
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

// Validate through the reader before interrupting work that may hold the writer.
// Callers must repeat their durable-state checks inside the write transaction.
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

// The backup operation is committed before its ID is registered with the worker.
// Hold mu and recheck durable identity before cancelling across that handoff.
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

func validatePolicyUpdate(r *record, u Update, now time.Time) (bool, error) {
	if r.Policy.Revision != u.Revision {
		return false, errors.New("conflict")
	}
	review := u.Enabled && (!r.Policy.Enabled || u.Age.Cutoff(now).After(r.Policy.Age.Cutoff(now)))
	if review && u.BackupChoice == "" {
		return false, errors.New("backup_choice_required")
	}
	return review, nil
}
