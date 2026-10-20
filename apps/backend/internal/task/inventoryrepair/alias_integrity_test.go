package inventoryrepair

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAliasWorkspaceCorrectionRequiresEverySession(t *testing.T) {
	for _, selection := range []string{"empty", "partial", "complete"} {
		t.Run(selection, func(t *testing.T) {
			f := sharedAliasFixture(t)
			execSQL(t, f.db, `INSERT INTO task_sessions(id,task_id,agent_profile_id,state,task_environment_id,workspace_path,started_at,updated_at) VALUES('child-second','child','profile','WAITING_FOR_INPUT','child-env','/old',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
			execSQL(t, f.db, `UPDATE task_sessions SET workspace_path='/old' WHERE id='child-session'`)
			f.plan.RedundantEnvironments[0].SessionIDs = []string{"child-session", "child-second"}
			root := filepath.Dir(f.plan.Repairs[0].Path)
			ids := []string{}
			if selection == "partial" {
				ids = []string{"child-session"}
			}
			if selection == "complete" {
				ids = []string{"child-session", "child-second"}
			}
			f.plan.Workspaces = []WorkspaceRepair{{EnvironmentID: "child-env", Path: root, SessionIDs: ids}}
			before := aliasFixtureRows(t, f)
			report, err := Preview(t.Context(), f.plan)
			if selection == "complete" {
				if err != nil {
					t.Fatal(err)
				}
				if err = repairWithIsolatedHost(report.Plan, "apply"); err != nil {
					t.Fatal(err)
				}
				var count int
				if err = f.db.Get(&count, `SELECT COUNT(*) FROM task_sessions WHERE id IN ('child-session','child-second') AND task_environment_id='env' AND workspace_path=?`, root); err != nil {
					t.Fatal(err)
				}
				if count != 2 {
					t.Fatalf("corrected %d sessions, want 2", count)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "explicitly include every bound session") {
				t.Fatalf("preview accepted incomplete correction: %v", err)
			}
			if err = repairWithIsolatedHost(f.plan, "apply"); err == nil {
				t.Fatal("apply accepted incomplete correction")
			}
			if aliasFixtureRows(t, f) != before {
				t.Fatal("refused correction changed rows")
			}
			if err = CheckPending(f.plan.Home, f.plan.Driver, f.plan.Database); err != nil {
				t.Fatalf("refused correction left a fence: %v", err)
			}
		})
	}
}

func TestAliasRepairPreservesPreexistingForeignKeyViolations(t *testing.T) {
	f := aliasFixture(t)
	execSQL(t, f.db, `CREATE TABLE repair_fk_parent(id TEXT PRIMARY KEY)`)
	execSQL(t, f.db, `CREATE TABLE unrelated_fk(id INTEGER PRIMARY KEY,parent_id TEXT REFERENCES repair_fk_parent(id))`)
	execSQL(t, f.db, `ALTER TABLE task_sessions ADD COLUMN legacy_parent_id TEXT REFERENCES repair_fk_parent(id)`)
	conn, err := f.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`PRAGMA foreign_keys=OFF`,
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1100) INSERT INTO unrelated_fk SELECT x,'missing' FROM n`,
		`UPDATE task_sessions SET legacy_parent_id='missing' WHERE id='session'`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err = conn.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	before := aliasFixtureRows(t, f)
	p := observedPlan(t, f)
	for _, mode := range []string{"apply", "verify", "rollback"} {
		if mode == "rollback" {
			j, loadErr := loadJournal(p)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			// Older journals have no foreign-key baseline. Recovery must still work.
			j.ForeignKeyBaseline = nil
			if saveErr := j.save(); saveErr != nil {
				t.Fatal(saveErr)
			}
		}
		if err = repairWithIsolatedHost(p, mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
	if aliasFixtureRows(t, f) != before {
		t.Fatal("rollback changed original rows")
	}
	if err = CheckPending(p.Home, p.Driver, p.Database); err != nil {
		t.Fatalf("rollback left a fence: %v", err)
	}
	var count int
	if err = f.db.Get(&count, `SELECT COUNT(*) FROM unrelated_fk`); err != nil {
		t.Fatal(err)
	}
	if count != 1100 {
		t.Fatalf("changed unrelated violations: %d", count)
	}
}

func TestAliasRepairRefusesInvalidSelectedRowBeforeFence(t *testing.T) {
	f := aliasFixture(t)
	execSQL(t, f.db, `CREATE TABLE repair_fk_parent(id TEXT PRIMARY KEY)`)
	execSQL(t, f.db, `ALTER TABLE task_environment_repos ADD COLUMN legacy_parent_id TEXT REFERENCES repair_fk_parent(id)`)
	conn, err := f.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`PRAGMA foreign_keys=OFF`, `UPDATE task_environment_repos SET legacy_parent_id='missing'`, `PRAGMA foreign_keys=ON`} {
		if _, err = conn.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	before := aliasFixtureRows(t, f)
	if _, err = Preview(t.Context(), f.plan); err == nil || !strings.Contains(err.Error(), "selected repair row") {
		t.Fatalf("invalid row accepted: %v", err)
	}
	if aliasFixtureRows(t, f) != before {
		t.Fatal("preview changed rows")
	}
	if err = CheckPending(f.plan.Home, f.plan.Driver, f.plan.Database); err != nil {
		t.Fatal(err)
	}
	j := &journal{}
	if err = j.checkNewAliasForeignKeyViolations(t.Context(), f.db); err == nil {
		t.Fatal("new violations accepted")
	}
}
