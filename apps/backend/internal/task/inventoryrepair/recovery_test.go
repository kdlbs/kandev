package inventoryrepair

import (
	"context"
	"errors"
	"os"
	"testing"
)

func stagedRepair(t *testing.T, f fixture) *journal {
	t.Helper()
	ctx := context.Background()
	p := observedPlan(t, f)
	in, err := inspect(ctx, p, f.db)
	if err != nil {
		t.Fatal(err)
	}
	j, err := prepareJournal(ctx, in, f.db.DB)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.fence(); err != nil {
		t.Fatal(err)
	}
	return j
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.5
func TestInterruptedRepairResumesOrReverses(t *testing.T) {
	for _, phase := range []string{"prepared", "moved", "committed"} {
		for _, mode := range []string{"apply", "rollback"} {
			t.Run(phase+"/"+mode, func(t *testing.T) {
				f := relocationFixture(t)
				j := stagedRepair(t, f)
				ctx := context.Background()
				if phase != "prepared" {
					if err := j.moveLocations(ctx, true); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "committed" {
					tx, err := f.db.BeginTx(ctx, nil)
					if err != nil {
						t.Fatal(err)
					}
					if err = publish(ctx, tx, j.Changes, true); err != nil {
						_ = tx.Rollback()
						t.Fatal(err)
					}
					if err = j.capturePublishedObservations(ctx, tx); err != nil {
						_ = tx.Rollback()
						t.Fatal(err)
					}
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
				}
				if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); err == nil {
					t.Fatal("interrupted repair did not fence startup")
				}
				if err := repairWithIsolatedHost(j.Plan, mode); err != nil {
					t.Fatal(err)
				}
				if mode == "apply" {
					if err := repairWithIsolatedHost(j.Plan, "verify"); err != nil {
						t.Fatal(err)
					}
				} else if _, err := Preview(ctx, j.Plan); err != nil {
					t.Fatal(err)
				}
				if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestRepairRefusesChangedOwnershipAfterJournal(t *testing.T) {
	f := relocationFixture(t)
	j := stagedRepair(t, f)
	execSQL(t, f.db, `UPDATE tasks SET archived_at=CURRENT_TIMESTAMP WHERE id='owner'`)
	if err := repairWithIsolatedHost(j.Plan, "apply"); err == nil {
		t.Fatal("published after ownership generation changed")
	}
	if _, err := os.Stat(j.Plan.Repairs[0].SourcePath); err != nil {
		t.Fatal("moved before validating task")
	}
	if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); err == nil {
		t.Fatal("unresolved repair lost startup fence")
	}
}

func TestFailureAfterMoveLeavesRecoverableJournal(t *testing.T) {
	f := relocationFixture(t)
	p := observedPlan(t, f)
	calls := 0
	err := runRepair(context.Background(), p, "apply", func(context.Context, Plan) error {
		calls++
		if calls == 3 {
			return errors.New("process appeared before commit")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected precommit failure")
	}
	if _, err = os.Stat(p.Repairs[0].Path); err != nil {
		t.Fatal("fixture did not fail after moving")
	}
	if err = repairWithIsolatedHost(p, "rollback"); err != nil {
		t.Fatal(err)
	}
	if _, err = Preview(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}
