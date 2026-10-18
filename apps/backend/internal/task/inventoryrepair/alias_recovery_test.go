package inventoryrepair

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.5
func TestAliasRecoveryInterruptedGraph(t *testing.T) {
	for _, phase := range []string{phasePrepared, "committed", "complete-fenced"} {
		for _, mode := range []string{"apply", "rollback"} {
			t.Run(phase+"/"+mode, func(t *testing.T) {
				f := sharedAliasFixture(t)
				addAliasSnapshot(t, &f)
				before := aliasFixtureRows(t, f)
				j := stagedRepair(t, f)
				if phase != phasePrepared {
					tx, err := f.db.BeginTx(context.Background(), nil)
					if err != nil {
						t.Fatal(err)
					}
					if err = publish(context.Background(), tx, j.Changes, true); err != nil {
						_ = tx.Rollback()
						t.Fatal(err)
					}
					if err = j.capturePublishedObservations(context.Background(), tx); err != nil {
						_ = tx.Rollback()
						t.Fatal(err)
					}
					if err = tx.Commit(); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "complete-fenced" {
					j.Phase = phaseComplete
					if err := j.save(); err != nil {
						t.Fatal(err)
					}
				}
				if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); !errors.Is(err, ErrRepairPending) {
					t.Fatalf("startup not fenced: %v", err)
				}
				if err := repairWithIsolatedHost(j.Plan, mode); err != nil {
					t.Fatal(err)
				}
				if mode == "rollback" && aliasFixtureRows(t, f) != before {
					t.Fatal("rollback changed historical rows")
				}
				if mode == "apply" {
					if err := Verify(context.Background(), j.Plan); err != nil {
						t.Fatal(err)
					}
				}
				if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestAliasRecoveryRejectsNewRootGitMetadata(t *testing.T) {
	f := aliasFixture(t)
	j := stagedRepair(t, f)
	if err := os.Mkdir(filepath.Join(filepath.Dir(j.Plan.Repairs[0].Path), ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := repairWithIsolatedHost(j.Plan, "apply"); err == nil {
		t.Fatal("accepted newly competing root Git metadata")
	}
	if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); !errors.Is(err, ErrRepairPending) {
		t.Fatalf("lost startup fence: %v", err)
	}
}

func TestAliasRecoveryRejectsNewDependentReference(t *testing.T) {
	f := sharedAliasFixture(t)
	j := stagedRepair(t, f)
	execSQL(t, f.db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,started_at,updated_at) VALUES('new-session','child','profile','FAILED','child-env',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	if err := repairWithIsolatedHost(j.Plan, "apply"); err == nil {
		t.Fatal("accepted unobserved dependent row")
	}
	if err := CheckPending(j.Plan.Home, j.Plan.Driver, j.Plan.Database); !errors.Is(err, ErrRepairPending) {
		t.Fatalf("lost startup fence: %v", err)
	}
}
