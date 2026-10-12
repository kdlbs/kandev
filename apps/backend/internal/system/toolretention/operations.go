package toolretention

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"time"
)

func (s *Service) Analyze(ctx context.Context, age Age) (string, error) {
	if err := age.Validate(); err != nil {
		return "", err
	}
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if operationBusy(r) {
			return errors.New("busy")
		}
		return startOperation(ctx, tx, r, kindAnalysis, age, s.opts.Now())
	})
	if err != nil {
		return "", err
	}
	s.notify()
	return r.Operation.ID, nil
}

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
		return startOperation(ctx, tx, r, kindCleanup, r.Policy.Age, s.opts.Now())
	})
	if err != nil {
		return "", err
	}
	s.notify()
	return r.Operation.ID, nil
}

func (s *Service) Cancel(ctx context.Context, id string) (Status, error) {
	observed, err := s.interruptValidated(ctx, func(r *record) error {
		return validateCancellation(r, id)
	})
	if err != nil {
		return Status{}, err
	}
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Policy.Revision != observed.Policy.Revision {
			return errors.New("conflict")
		}
		if err := validateCancellation(r, id); err != nil {
			return err
		}
		finishOperation(r, stateCancelled, "", s.opts.Now())
		if r.Preparation.State == stateRunning || r.Preparation.State == statePending {
			r.Preparation = Preparation{State: stateNone}
			r.ApprovedRevision = 0
			r.Policy.Enabled = false
		}
		next := s.opts.Now().Add(24 * time.Hour)
		if r.Policy.Enabled && r.Operation.Kind == kindCleanup {
			r.NextDueAt = &next
		}
		return nil
	})
	if err == nil {
		s.mu.Lock()
		if s.activeCancel != nil && s.activeID == id && s.activeRevision == observed.Policy.Revision {
			s.activeCancel()
		}
		s.mu.Unlock()
		s.notify()
	}
	return r.Status, err
}

func validateCancellation(r *record, id string) error {
	if r.Operation == nil || r.Operation.ID != id || r.Operation.State != stateRunning {
		return errors.New("conflict")
	}
	return nil
}

func approved(r *record) bool {
	return r.Policy.Enabled && r.ApprovedRevision == r.Policy.Revision && r.Preparation.State == stateReady && (r.Preparation.Choice == choiceSkip || r.Receipt != "")
}
func operationBusy(r *record) bool {
	return (r.Operation != nil && r.Operation.State == stateRunning) || r.Preparation.State == statePending || r.Preparation.State == stateRunning
}

func startOperation(ctx context.Context, q sqlx.QueryerContext, r *record, kind string, age Age, now time.Time) error {
	var schemaVersion int
	if err := scanGet(ctx, q, &schemaVersion, `PRAGMA schema_version`); err != nil {
		return err
	}
	var upper string
	if err := sqlx.GetContext(ctx, q, &upper, `SELECT COALESCE(MAX(id),'') FROM tasks`); err != nil {
		return err
	}
	r.Operation = &Operation{ID: uuid.NewString(), Kind: kind, State: stateRunning, Age: age, Cutoff: age.Cutoff(now), StartedAt: now, Skipped: map[string]int64{}}
	r.Progress = progress{UpperTask: upper, Revision: r.Policy.Revision, Started: true, SchemaVersion: schemaVersion}
	return nil
}
