package dataretention

import (
	"context"
	"errors"
	"maps"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/system/maintenance"
)

var errMaintenanceBusy = errors.New("maintenance_busy")

// stepCleanup runs one mutating batch under the shared maintenance admission, so
// a pass defers while backup, restore, reset or compaction owns the writer.
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
			scheduleNextPass(r, s.opts.Now())
		}
		return nil
	})
	if err == nil && len(changed) > 0 && s.opts.Changed != nil {
		s.opts.Changed(ctx, changed)
	}
	return err
}

// prepareFirstBatch verifies the approved backup receipt before the first
// mutation, and reports whether the batch found nothing to do. An empty batch
// must not repeatedly verify a multi-gigabyte backup, so the receipt is only
// consulted once a row is actually about to change.
func (s *Service) prepareFirstBatch(ctx context.Context, r record) (bool, error) {
	if r.FirstMutation || r.Preparation.Choice != choiceBackup {
		return false, nil
	}
	op := *r.Operation
	op.Skipped = maps.Clone(op.Skipped)
	r.Operation = &op
	if _, err := s.scanBatch(ctx, s.pool.Reader(), &r, false); err != nil {
		return false, err
	}
	if !hasEligibleWork(&r) {
		return true, s.storeScanProgress(ctx, r)
	}
	return false, s.checkReceipt(ctx, r)
}

// hasEligibleWork reports whether the dry run found a candidate in either
// target. The gate cannot depend on changed message identity, because a batch
// whose only work is a finished cleanup job still needs its receipt verified
// before that job's snapshot is rewritten.
func hasEligibleWork(r *record) bool {
	if r.Operation == nil {
		return false
	}
	return r.Operation.Messages.Eligible > 0 || r.Operation.CleanupJobs.Eligible > 0
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
