package sqlite_test

import (
	"context"
	"testing"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-OFFICE-COSTS-001.2: real PostgreSQL query parity, DSN-gated.
func TestPostgresCostsWindowQueries(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatal(err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	windowSQL(t, repo, `INSERT INTO agents (id,name,created_at,updated_at) VALUES (?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, "claude-acp", "claude-acp")
	for _, row := range []struct{ id, ws string }{{"window-task", "window-ws"}, {"foreign-task", "foreign-ws"}} {
		_, err = db.ExecContext(context.Background(), db.Rebind(`INSERT INTO workspaces (id,name,created_at,updated_at) VALUES (?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`), row.ws, row.ws)
		if err != nil {
			t.Fatal(err)
		}
		windowSQL(t, repo, `INSERT INTO tasks (id,workspace_id,title,created_at,updated_at) VALUES (?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, row.id, row.ws, row.id)
	}
	assertCostsWindowQueries(t, repo)
}
