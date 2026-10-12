package toolretention

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func (s *Service) stepPreparation(ctx context.Context) error {
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Preparation.State != statePending {
			return nil
		}
		r.Preparation.State = stateRunning
		r.Operation = &Operation{ID: uuid.NewString(), Kind: choiceBackup, State: stateRunning, Age: r.Policy.Age, StartedAt: s.opts.Now(), Cutoff: r.Policy.Age.Cutoff(s.opts.Now()), Skipped: map[string]int64{}}
		return nil
	})
	if err != nil {
		return err
	}
	if r.Preparation.State != stateRunning {
		return nil
	}
	s.mu.Lock()
	if s.activeCancel != nil {
		s.activeID = r.Operation.ID
		s.activeRevision = r.Policy.Revision
	}
	s.mu.Unlock()
	if s.opts.Report != nil {
		s.opts.Report(ctx, r.Operation)
	}
	var receipt string
	if r.Preparation.Choice == choiceBackup {
		if s.opts.CreateBackup == nil {
			err = errors.New("backup_unavailable")
		} else {
			receipt, err = s.opts.CreateBackup(ctx)
		}
		if err == nil && receipt == "" {
			err = errors.New("backup_unavailable")
		}
	}
	return s.finishPreparation(ctx, r, receipt, err)
}

func (s *Service) finishPreparation(ctx context.Context, prepared record, receipt string, backupErr error) error {
	code := ""
	if backupErr != nil {
		code = "backup_failed"
	}
	_, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Policy.Revision != prepared.Policy.Revision || r.Preparation.State != stateRunning || r.Operation == nil || r.Operation.ID != prepared.Operation.ID {
			return nil
		}
		if backupErr != nil {
			r.Preparation.State = stateFailed
			r.Preparation.Error = code
			finishOperation(r, stateFailed, code, s.opts.Now())
			return nil
		}
		r.Preparation.State = stateReady
		r.Preparation.Error = ""
		r.Receipt = receipt
		r.ApprovedRevision = r.Policy.Revision
		r.Policy.Enabled = true
		return startOperation(ctx, tx, r, kindCleanup, r.Policy.Age, s.opts.Now())
	})
	if err != nil {
		return err
	}
	if backupErr != nil {
		return errors.New(code)
	}
	return nil
}

func (s *Service) recoverPreparation(ctx context.Context, observed record) error {
	_, err := s.change(ctx, func(r *record, _ *sqlx.Tx) error {
		if r.Policy.Revision != observed.Policy.Revision || r.Preparation.State != stateRunning {
			return nil
		}
		observedID, currentID := "", ""
		if observed.Operation != nil {
			observedID = observed.Operation.ID
		}
		if r.Operation != nil {
			currentID = r.Operation.ID
		}
		if currentID == observedID {
			s.failPreparation(r)
		}
		return nil
	})
	return err
}

func (s *Service) failPreparation(r *record) {
	r.Policy.Enabled = false
	r.ApprovedRevision = 0
	r.Receipt = ""
	r.NextDueAt = nil
	r.Preparation.State = stateFailed
	r.Preparation.Error = "backup_interrupted"
	finishOperation(r, stateFailed, "backup_interrupted", s.opts.Now())
}
