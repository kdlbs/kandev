package db

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestTableColumnsAndExistsPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := database.Exec(`CREATE TABLE sample_schema (
		id TEXT PRIMARY KEY,
		created_at TIMESTAMPTZ NOT NULL
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	exists, err := TableExists(database, "sample_schema")
	if err != nil {
		t.Fatalf("table exists: %v", err)
	}
	if !exists {
		t.Fatal("TableExists = false, want true")
	}

	columns, err := TableColumns(database, "sample_schema")
	if err != nil {
		t.Fatalf("table columns: %v", err)
	}
	if !columns["created_at"] || columns["missing"] {
		t.Fatalf("columns = %#v, want created_at and not missing", columns)
	}

	got, err := ColumnExists(database, "sample_schema", "missing")
	if err != nil {
		t.Fatalf("missing column exists: %v", err)
	}
	if got {
		t.Fatal("ColumnExists for missing column = true, want false")
	}

	missingTable, err := TableExists(database, "missing_schema")
	if err != nil {
		t.Fatalf("missing table exists: %v", err)
	}
	if missingTable {
		t.Fatal("TableExists for missing table = true, want false")
	}

	missingColumns, err := TableColumns(database, "missing_schema")
	if err != nil {
		t.Fatalf("missing table columns: %v", err)
	}
	if len(missingColumns) != 0 {
		t.Fatalf("missing table columns = %#v, want empty", missingColumns)
	}
}
