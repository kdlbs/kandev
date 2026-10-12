package db_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/db"
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
	if _, err := database.Exec(`CREATE INDEX sample_schema_id_idx ON sample_schema(id)`); err != nil {
		t.Fatalf("create index: %v", err)
	}

	exists, err := db.TableExists(database, "sample_schema")
	if err != nil {
		t.Fatalf("table exists: %v", err)
	}
	if !exists {
		t.Fatal("TableExists = false, want true")
	}

	columns, err := db.TableColumns(database, "sample_schema")
	if err != nil {
		t.Fatalf("table columns: %v", err)
	}
	if !columns["created_at"] || columns["missing"] {
		t.Fatalf("columns = %#v, want created_at and not missing", columns)
	}

	got, err := db.ColumnExists(database, "sample_schema", "missing")
	if err != nil {
		t.Fatalf("missing column exists: %v", err)
	}
	if got {
		t.Fatal("ColumnExists for missing column = true, want false")
	}

	missingTable, err := db.TableExists(database, "missing_schema")
	if err != nil {
		t.Fatalf("missing table exists: %v", err)
	}
	if missingTable {
		t.Fatal("TableExists for missing table = true, want false")
	}

	missingColumns, err := db.TableColumns(database, "missing_schema")
	if err != nil {
		t.Fatalf("missing table columns: %v", err)
	}
	if len(missingColumns) != 0 {
		t.Fatalf("missing table columns = %#v, want empty", missingColumns)
	}
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "sample_schema_id_idx", want: true},
		{name: "missing_schema_idx", want: false},
	} {
		got, err := db.IndexExistsContext(context.Background(), database, test.name)
		if err != nil {
			t.Fatalf("index %s exists: %v", test.name, err)
		}
		if got != test.want {
			t.Errorf("IndexExistsContext(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}
