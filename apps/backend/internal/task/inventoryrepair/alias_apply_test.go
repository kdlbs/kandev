package inventoryrepair

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aliasFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	p, err := Decode(strings.NewReader(aliasPlanInput(t, f)))
	if err != nil {
		t.Fatal(err)
	}
	f.plan = p
	return f
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.3, AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.5
func TestAliasApplyPreservesAndRollsBack(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "same-root"
		if shared {
			name = "shared"
		}
		t.Run(name, func(t *testing.T) {
			f := aliasFixture(t)
			if shared {
				f = sharedAliasFixture(t)
				addAliasSnapshot(t, &f)
			}
			path := f.plan.Repairs[0].Path
			if err := os.WriteFile(filepath.Join(path, "tracked"), []byte("staged change\n"), 0600); err != nil {
				t.Fatal(err)
			}
			git(t, path, "add", "tracked")
			if err := os.WriteFile(filepath.Join(path, "untracked"), []byte("preserve\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before := aliasFixtureRows(t, f)
			p := observedPlan(t, f)
			if err := repairWithIsolatedHost(p, "apply"); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := f.db.Get(&count, `SELECT count(*) FROM task_environment_repos WHERE worktree_id='wt'`); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("alias rows remain: %d", count)
			}
			if shared {
				var binding, state string
				if err := f.db.QueryRow(`SELECT task_environment_id,state FROM task_sessions WHERE id='child-session'`).Scan(&binding, &state); err != nil {
					t.Fatal(err)
				}
				if binding != "env" || state != "WAITING_FOR_INPUT" {
					t.Fatalf("session changed incorrectly: %s %s", binding, state)
				}
				if err := f.db.Get(&binding, `SELECT task_environment_id FROM task_session_git_snapshots WHERE id='snapshot'`); err != nil {
					t.Fatal(err)
				}
				if binding != "env" {
					t.Fatal("snapshot did not survive rebinding")
				}
			}
			for _, mode := range []string{"verify", "apply", "rollback", "rollback"} {
				if err := repairWithIsolatedHost(p, mode); err != nil {
					t.Fatalf("%s: %v", mode, err)
				}
			}
			if after := aliasFixtureRows(t, f); after != before {
				t.Fatal("rollback did not restore exact original rows")
			}
			if _, err := Preview(context.Background(), p); err != nil {
				t.Fatalf("original content and identities changed: %v", err)
			}
		})
	}
}

func addAliasSnapshot(t *testing.T, f *fixture) {
	t.Helper()
	execSQL(t, f.db, `INSERT INTO task_session_git_snapshots(id,task_environment_id,session_id,snapshot_type,branch,files,metadata,created_at) VALUES('snapshot','child-env','child-session','status_update','feature/actual','{"tracked":{"status":"modified"}}','{"repository_name":"Source"}','2026-08-01 01:02:03.120+00:00')`)
	f.plan.RedundantEnvironments[0].SnapshotIDs = []string{"snapshot"}
}

func aliasFixtureRows(t *testing.T, f fixture) string {
	t.Helper()
	var all []any
	for _, table := range []string{"task_environments", "task_environment_repos", "task_sessions", "task_session_git_snapshots"} {
		rows, err := queryRows(context.Background(), f.db, `SELECT *,CAST(created_at AS TEXT) AS raw_created FROM `+table+` ORDER BY id`)
		if table == "task_sessions" {
			rows, err = queryRows(context.Background(), f.db, `SELECT *,CAST(started_at AS TEXT) AS raw_started,CAST(updated_at AS TEXT) AS raw_updated FROM task_sessions ORDER BY id`)
		}
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, rows)
	}
	return digest(all)
}

func TestAliasApplyAuditFailureRollsBackEntireGraph(t *testing.T) {
	f := sharedAliasFixture(t)
	addAliasSnapshot(t, &f)
	before := aliasFixtureRows(t, f)
	p := observedPlan(t, f)
	calls := 0
	err := runRepair(context.Background(), p, modeApply, func(context.Context, Plan) error {
		calls++
		if calls == 3 {
			return errors.New("consumer appeared before commit")
		}
		return nil
	})
	if err == nil {
		t.Fatal("accepted consumer before commit")
	}
	if aliasFixtureRows(t, f) != before {
		t.Fatal("published a partial graph")
	}
	if err = CheckPending(p.Home, p.Driver, p.Database); !errors.Is(err, ErrRepairPending) {
		t.Fatalf("lost startup fence: %v", err)
	}
	if err = repairWithIsolatedHost(p, "rollback"); err != nil {
		t.Fatal(err)
	}
}
