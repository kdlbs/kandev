package dataretention

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newOperationID() string { return uuid.NewString() }

type HTTPService interface {
	Get(context.Context) (Status, error)
	Save(context.Context, Update) (Status, error)
	Analyze(context.Context, Update) (string, error)
	Run(context.Context, int64) (string, error)
	Cancel(context.Context, string) (Status, error)
}

// Analyze starts a read-only estimate for a draft policy without saving it.
func (s *Service) Analyze(ctx context.Context, u Update) (string, error) {
	if err := u.ArchivedAge.Validate(); err != nil {
		return "", err
	}
	if err := u.CleanupAge.Validate(); err != nil {
		return "", err
	}
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Operation != nil && r.Operation.State == stateRunning {
			return errors.New("busy")
		}
		now := s.opts.Now()
		if err := s.startOperation(ctx, tx, r, kindAnalysis, u.ArchivedAge.Cutoff(now), u.CleanupAge.Cutoff(now), now); err != nil {
			return err
		}
		r.Operation.AnalysisOnly = true
		return nil
	})
	if err != nil {
		return "", err
	}
	s.notify()
	return r.Operation.ID, nil
}

// Run starts a cleanup pass against the saved, approved policy revision.
func (s *Service) Run(ctx context.Context, revision int64) (string, error) {
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Policy.Revision != revision {
			return errors.New("conflict")
		}
		if !approved(r) {
			return errors.New("preparation_required")
		}
		if operationBusy(r) {
			return errors.New("busy")
		}
		now := s.opts.Now()
		if err := s.startOperation(ctx, tx, r, kindCleanup, r.Policy.ArchivedAge.Cutoff(now), r.Policy.CleanupAge.Cutoff(now), now); err != nil {
			return err
		}
		// The pass reschedules on completion, so the pending slot is released
		// here rather than surviving a cancelled run.
		r.NextDueAt = nil
		return nil
	})
	if err != nil {
		return "", err
	}
	s.notify()
	return r.Operation.ID, nil
}

// Cancel stops the named operation at its next batch boundary. Cancelling a
// backup invalidates the policy revision so a late backup cannot arm cleanup.
func (s *Service) Cancel(ctx context.Context, id string) (Status, error) {
	if _, err := s.interruptValidated(ctx, func(r *record) error {
		if r.Operation == nil || r.Operation.ID != id {
			return errors.New("not_found")
		}
		if r.Operation.State != stateRunning {
			return errors.New("conflict")
		}
		return nil
	}); err != nil {
		return Status{}, err
	}
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if !operationMatches(r, id) || r.Operation.State != stateRunning {
			return nil
		}
		if r.Operation.Kind == choiceBackup {
			// Cancelling a backup invalidates the revision and clears the
			// preparation, so a late completion cannot arm a deletion.
			r.Policy.Enabled = false
			r.Preparation = Preparation{State: stateNone}
			r.PreparationDetail = ""
			r.ApprovedRevision = 0
			r.Receipt = ""
			r.NextDueAt = nil
			finishOperation(r, stateCancelled, "cancelled", s.opts.Now())
			return nil
		}
		finishOperation(r, stateCancelled, "cancelled", s.opts.Now())
		return nil
	})
	if err != nil {
		return Status{}, err
	}
	s.cancelOperation(id)
	s.notify()
	return r.Status, nil
}
