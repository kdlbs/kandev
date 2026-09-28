package inventoryrepair

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

const (
	modeApply    = "apply"
	modeRollback = "rollback"
	modeVerify   = "verify"
)

func Apply(ctx context.Context, p Plan) error { return runRepair(ctx, p, modeApply, checkProcesses) }
func Rollback(ctx context.Context, p Plan) error {
	return runRepair(ctx, p, modeRollback, checkProcesses)
}
func Verify(ctx context.Context, p Plan) error {
	if err := p.validate(); err != nil {
		return err
	}
	db, err := openDatabase(p, false)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	j, err := loadJournal(p)
	if err != nil {
		return err
	}
	if err = checkJournalBackup(j); err != nil {
		return err
	}
	return j.verify(ctx, db)
}

func repairOwnership(p Plan) (*ownershiplock.Owner, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if len(p.ExpectedRows) == 0 || len(p.ExpectedGit) != len(p.Repairs) {
		return nil, errors.New("application requires a reviewed preview with row and Git observations")
	}
	for _, path := range []string{p.Home, p.TasksRoot, filepath.Dir(p.Database)} {
		h, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(path), path)
		if err != nil {
			return nil, err
		}
		_ = h.Close()
	}
	info, err := os.Lstat(p.Database)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("repair database must be a regular file")
	}
	targets, err := ownershiplock.Targets(p.Home, p.Driver, p.Database)
	if err != nil {
		return nil, err
	}
	return ownershiplock.Acquire(targets)
}

func runRepair(ctx context.Context, p Plan, mode string, audit func(context.Context, Plan) error) error {
	owner, err := repairOwnership(p)
	if err != nil {
		return err
	}
	defer func() { _ = owner.Close() }()
	if err = audit(ctx, p); err != nil {
		return err
	}
	db, err := openDatabase(p, true)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	j, err := loadOrPrepare(ctx, p, db, mode)
	if err != nil {
		return err
	}
	return runJournal(ctx, p, db, j, mode, audit)
}

func loadOrPrepare(ctx context.Context, p Plan, db *sql.DB, mode string) (*journal, error) {
	j, err := loadJournal(p)
	if errors.Is(err, os.ErrNotExist) && mode == modeApply {
		if err = CheckPending(p.Home, p.Driver, p.Database); err != nil {
			return nil, err
		}
		var in *inspection
		in, err = inspect(ctx, p, db)
		if err != nil {
			return nil, err
		}
		if len(in.report.Blockers) > 0 {
			return nil, fmt.Errorf("repair blocked: %s", strings.Join(in.report.Blockers, ", "))
		}
		j, err = prepareJournal(ctx, in, db)
	}
	if err != nil {
		return nil, err
	}
	return j, nil
}

func runJournal(ctx context.Context, p Plan, db *sql.DB, j *journal, mode string, audit func(context.Context, Plan) error) error {
	var err error
	if err = checkJournalBackup(j); err != nil {
		return err
	}
	if mode == modeVerify {
		return j.verify(ctx, db)
	}
	if j.Phase == "rolled_back" {
		if mode == modeRollback {
			return j.finish("rolled_back")
		}
		return errors.New("operation was rolled back; preview a new operation ID")
	}
	if j.Phase == phaseComplete && mode == modeApply {
		if err = j.verify(ctx, db); err != nil {
			return err
		}
		return j.finish(phaseComplete)
	}
	if j.Phase == phaseComplete {
		if err = j.verify(ctx, db); err != nil {
			return err
		}
	}
	if err = j.fence(); err != nil {
		return err
	}
	if err = j.reconcile(ctx, db, mode == modeApply, audit); err != nil {
		return fmt.Errorf("repair %s remains fenced; rerun --apply or --rollback: %w", p.OperationID, err)
	}
	phase := phaseComplete
	if mode == modeRollback {
		phase = "rolled_back"
	}
	return j.finish(phase)
}

func (j *journal) verify(ctx context.Context, db queryer) error {
	if j.Phase != phaseComplete {
		return errors.New("repair has not completed")
	}
	if err := rowsMatch(ctx, db, j.Changes, true); err != nil {
		return err
	}
	if err := j.checkObservations(ctx, db, true); err != nil {
		return err
	}
	return j.verifyLocations(ctx, true)
}

func (j *journal) reconcile(ctx context.Context, db *sql.DB, forward bool, audit func(context.Context, Plan) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	before := rowsMatch(ctx, tx, j.Changes, false) == nil
	after := rowsMatch(ctx, tx, j.Changes, true) == nil
	if !before && !after {
		return errors.New("database matches neither original nor repaired rows")
	}
	if err = j.checkObservations(ctx, tx, after && !before); err != nil {
		return err
	}
	if err = audit(ctx, j.Plan); err != nil {
		return err
	}
	if err = j.moveLocations(ctx, forward); err != nil {
		return err
	}
	if err = j.verifyLocations(ctx, forward); err != nil {
		return err
	}
	j.Phase = "moved"
	if err = j.save(); err != nil {
		return err
	}
	if err = j.publishDirection(ctx, tx, forward, before, after, audit); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	j.Phase = "published"
	return j.save()
}

func (j *journal) publishDirection(ctx context.Context, tx *sql.Tx, forward, before, after bool, audit func(context.Context, Plan) error) error {
	var err error
	if (forward && !after) || (!forward && !before) {
		if err = publish(ctx, tx, j.Changes, forward); err != nil {
			return err
		}
	}
	if err = audit(ctx, j.Plan); err != nil {
		return err
	}
	if forward {
		if err = j.capturePublishedObservations(ctx, tx); err != nil {
			return err
		}
	} else if err = j.checkObservations(ctx, tx, false); err != nil {
		return err
	}
	return nil
}
