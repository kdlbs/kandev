package models

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

func TestStepSQLMatchesGoPredicates(t *testing.T) {
	engines := []struct {
		name string
		open func(*testing.T) *sqlx.DB
	}{
		{
			name: "sqlite",
			open: func(t *testing.T) *sqlx.DB {
				t.Helper()
				database, err := internaldb.OpenSQLite(filepath.Join(t.TempDir(), "predicates.db"))
				if err != nil {
					t.Fatalf("open sqlite: %v", err)
				}
				db := sqlx.NewDb(database, "sqlite3")
				t.Cleanup(func() { _ = db.Close() })
				return db
			},
		},
		{
			name: "postgres",
			open: func(t *testing.T) *sqlx.DB {
				t.Helper()
				return testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			},
		},
	}

	for _, engine := range engines {
		t.Run(engine.name, func(t *testing.T) {
			db := engine.open(t)
			ctx := context.Background()
			if _, err := db.ExecContext(ctx, `CREATE TABLE workflow_steps (
				id TEXT PRIMARY KEY,
				workflow_id TEXT NOT NULL,
				position INTEGER NOT NULL,
				complete_task_on_enter INTEGER NOT NULL DEFAULT 0
			)`); err != nil {
				t.Fatalf("create workflow_steps: %v", err)
			}

			steps := completionPredicateFixtures()
			for _, step := range steps {
				completeTaskOnEnter := 0
				if step.CompleteTaskOnEnter {
					completeTaskOnEnter = 1
				}
				if _, err := db.ExecContext(ctx, db.Rebind(`
					INSERT INTO workflow_steps (id, workflow_id, position, complete_task_on_enter)
					VALUES (?, ?, ?, ?)
				`), step.ID, step.WorkflowID, step.Position, completeTaskOnEnter); err != nil {
					t.Fatalf("insert step %q: %v", step.ID, err)
				}
			}

			type result struct {
				ID         string `db:"id"`
				Final      bool   `db:"final"`
				Completing bool   `db:"completing"`
			}
			var results []result
			query := "SELECT s.id, " + FinalStepSQL("s") + " AS final, " + CompletingStepSQL("s") + " AS completing FROM workflow_steps s"
			if err := db.SelectContext(ctx, &results, query); err != nil {
				t.Fatalf("evaluate SQL predicates: %v", err)
			}
			byID := make(map[string]result, len(results))
			for _, row := range results {
				byID[row.ID] = row
			}

			for i := range steps {
				step := &steps[i]
				next := nextStepWithHigherPosition(steps, step)
				got := byID[step.ID]
				if want := IsFinalStep(step, next); got.Final != want {
					t.Errorf("%s final: SQL = %v, Go = %v", step.ID, got.Final, want)
				}
				if want := CompletesTaskOnEnter(step, next); got.Completing != want {
					t.Errorf("%s completing: SQL = %v, Go = %v", step.ID, got.Completing, want)
				}
			}
		})
	}
}
