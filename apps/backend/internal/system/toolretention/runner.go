package toolretention

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/system/maintenance"
	"maps"
	"time"
)

var errMaintenanceBusy = errors.New("maintenance_busy")

func (s *Service) stepCleanup(ctx context.Context, id string) error {
	release, ok := maintenance.ForPool(s.pool).TryAcquire()
	if !ok {
		return errMaintenanceBusy
	}
	defer release()
	before, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	if !operationMatches(&before, id) {
		return nil
	}
	if !approved(&before) {
		return errors.New("preparation_required")
	}
	handled, err := s.prepareFirstBatch(ctx, before)
	if err != nil || handled {
		return err
	}
	var changed []string
	_, err = s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if !operationMatches(r, id) {
			return nil
		}
		if !approved(r) || r.Policy.Revision != r.Progress.Revision {
			return errors.New("conflict")
		}
		var scanErr error
		changed, scanErr = s.scanBatch(ctx, tx, r, true)
		if scanErr != nil {
			return scanErr
		}
		if r.Operation.State != stateRunning {
			next := s.opts.Now().Add(24 * time.Hour)
			r.NextDueAt = &next
		}
		return nil
	})
	if err == nil && len(changed) > 0 && s.opts.Changed != nil {
		s.opts.Changed(ctx, changed)
	}
	return err
}

func (s *Service) prepareFirstBatch(ctx context.Context, r record) (bool, error) {
	if r.FirstMutation || r.Preparation.Choice != choiceBackup {
		return false, nil
	}
	// Empty batches must not repeatedly hash a multi-gigabyte backup.
	op := *r.Operation
	op.Skipped = maps.Clone(op.Skipped)
	r.Operation = &op
	ids, err := s.scanBatch(ctx, s.pool.Reader(), &r, false)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return true, s.storeScanProgress(ctx, r)
	}
	return false, s.checkReceipt(ctx, r)
}

func operationMatches(r *record, id string) bool {
	return r.Operation != nil && r.Operation.ID == id && r.Operation.State == stateRunning
}

func (s *Service) checkReceipt(ctx context.Context, r record) error {
	if r.FirstMutation || r.Preparation.Choice != choiceBackup {
		return nil
	}
	if s.opts.VerifyBackup == nil {
		return errors.New("backup_unavailable")
	}
	if err := s.opts.VerifyBackup(ctx, r.Receipt); err != nil {
		return errors.New("backup_verification_failed")
	}
	return nil
}
