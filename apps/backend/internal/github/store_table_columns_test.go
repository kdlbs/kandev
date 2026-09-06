package github

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestTableColumnsSQLite(t *testing.T) {
	store := newTestStore(t)

	columns, err := store.tableColumns("github_ci_run_grants")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if _, ok := columns["workspace_id"]; !ok {
		t.Fatal("tableColumns omitted github_ci_run_grants.workspace_id")
	}
}

func TestTableColumnsPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	columns, err := store.tableColumns("github_ci_run_grants")
	if err != nil {
		t.Fatalf("tableColumns: %v", err)
	}
	if _, ok := columns["workspace_id"]; !ok {
		t.Fatal("tableColumns omitted github_ci_run_grants.workspace_id")
	}
}
