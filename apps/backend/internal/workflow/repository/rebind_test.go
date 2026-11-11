package repository

import (
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestRepositoryRebindUsesDriverPlaceholders(t *testing.T) {
	sqlite := &Repository{db: sqlx.NewDb(&sql.DB{}, "sqlite3")}
	if got := sqlite.Rebind("SELECT * FROM workflow_steps WHERE workflow_id=? AND id=?"); got != "SELECT * FROM workflow_steps WHERE workflow_id=? AND id=?" {
		t.Fatalf("SQLite query = %q", got)
	}
	postgres := &Repository{db: sqlx.NewDb(&sql.DB{}, "pgx")}
	if got := postgres.Rebind("SELECT * FROM workflow_steps WHERE workflow_id=? AND id=?"); got != "SELECT * FROM workflow_steps WHERE workflow_id=$1 AND id=$2" {
		t.Fatalf("PostgreSQL query = %q", got)
	}
}
