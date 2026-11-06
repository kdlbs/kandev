package dataretention

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
)

// stepPreparation takes the administrator's recorded backup-or-skip choice and
// only reaches stateReady, which approves the saved revision, once the choice is
// satisfied. A pending choice arms no deletion.
func (s *Service) stepPreparation(ctx context.Context) error {
	r, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Preparation.State != statePending {
			return nil
		}
		r.Preparation.State = stateRunning
		now := s.opts.Now()
		return s.startOperation(ctx, tx, r, choiceBackup, r.Policy.ArchivedAge.Cutoff(now), r.Policy.CleanupAge.Cutoff(now), now)
	})
	if err != nil {
		return err
	}
	if r.Preparation.State != stateRunning {
		return nil
	}
	s.claimActive(r.Operation.ID, r.Policy.Revision)
	if s.opts.Report != nil {
		s.opts.Report(ctx, r.Operation)
	}
	var receipt string
	if r.Preparation.Choice == choiceBackup {
		receipt, err = s.createBackup(ctx)
	}
	return s.finishPreparation(ctx, r, receipt, err)
}

// claimActive registers the just-committed backup identity with the worker. The
// operation is durable before this runs, so cancellation of the same revision
// must find it through the handoff recheck.
func (s *Service) claimActive(id string, revision int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeCancel != nil {
		s.activeID = id
		s.activeRevision = revision
	}
}

func (s *Service) createBackup(ctx context.Context) (string, error) {
	if s.opts.CreateBackup == nil {
		return "", errors.New("backup_unavailable")
	}
	receipt, err := s.opts.CreateBackup(ctx)
	if err != nil {
		return "", err
	}
	if receipt == "" {
		return "", errors.New("backup_unavailable")
	}
	return receipt, nil
}

func (s *Service) finishPreparation(ctx context.Context, prepared record, receipt string, backupErr error) error {
	code, detail := "", ""
	if backupErr != nil {
		code = "backup_failed"
		detail = limitPreparationDetail(backupErr.Error())
		s.logFailure(ctx, "archived data retention preparation failed", backupErr)
	}
	_, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if r.Policy.Revision != prepared.Policy.Revision || r.Preparation.State != stateRunning ||
			r.Operation == nil || r.Operation.ID != prepared.Operation.ID {
			return nil
		}
		if backupErr != nil {
			r.Preparation.State = stateFailed
			r.Preparation.Error = code
			r.PreparationDetail = detail
			finishOperation(r, stateFailed, code, s.opts.Now())
			return nil
		}
		r.Preparation.State = stateReady
		r.Preparation.Error = ""
		r.PreparationDetail = ""
		r.Receipt = receipt
		r.ApprovedRevision = r.Policy.Revision
		r.Policy.Enabled = true
		now := s.opts.Now()
		return s.startOperation(ctx, tx, r, kindCleanup, r.Policy.ArchivedAge.Cutoff(now), r.Policy.CleanupAge.Cutoff(now), now)
	})
	if err != nil {
		return err
	}
	if backupErr != nil {
		return errors.New(code)
	}
	return nil
}

// preparationDetailLimit bounds the stored failure cause so a runaway error
// message cannot bloat the settings row.
const preparationDetailLimit = 1024

func limitPreparationDetail(message string) string {
	if len(message) <= preparationDetailLimit {
		return message
	}
	return message[:preparationDetailLimit]
}

// recoverPreparation fails a preparation whose worker is gone. Ticks are
// serialized, so a record still marked running when reconcile observes it is one
// whose backup worker did not survive; the same operation identity on both sides
// means nothing else took the preparation over. An unbacked approval is never
// left standing, so a restart cannot arm a deletion silently.
func (s *Service) recoverPreparation(ctx context.Context, observed record) error {
	_, err := s.change(ctx, func(r *record, _ *sqlx.Tx) error {
		if r.Policy.Revision != observed.Policy.Revision || r.Preparation.State != stateRunning {
			return nil
		}
		observedID := ""
		if observed.Operation != nil {
			observedID = observed.Operation.ID
		}
		if observedID == currentID(r) {
			s.failPreparation(r)
		}
		return nil
	})
	return err
}

// currentID identifies the operation the durable record still carries, so a
// missing one compares as empty rather than being read as a mismatch.
func currentID(r *record) string {
	if r.Operation == nil {
		return ""
	}
	return r.Operation.ID
}

func (s *Service) failPreparation(r *record) {
	r.Policy.Enabled = false
	r.ApprovedRevision = 0
	r.Receipt = ""
	r.NextDueAt = nil
	r.Preparation.State = stateFailed
	r.Preparation.Error = "backup_interrupted"
	r.PreparationDetail = ""
	finishOperation(r, stateFailed, "backup_interrupted", s.opts.Now())
}
