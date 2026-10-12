package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestTableColumnsAndExistsSQLite(t *testing.T) {
	rawDB, err := OpenSQLite(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })

	if _, err := database.Exec(`CREATE TABLE sample_schema (
		id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := database.Exec(`CREATE INDEX sample_schema_id_idx ON sample_schema(id)`); err != nil {
		t.Fatalf("create index: %v", err)
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

	for _, column := range []struct {
		name string
		want bool
	}{
		{name: "id", want: true},
		{name: "missing", want: false},
	} {
		got, err := ColumnExists(database, "sample_schema", column.name)
		if err != nil {
			t.Fatalf("column %s exists: %v", column.name, err)
		}
		if got != column.want {
			t.Errorf("ColumnExists(%q) = %v, want %v", column.name, got, column.want)
		}
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
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "sample_schema_id_idx", want: true},
		{name: "missing_schema_idx", want: false},
	} {
		got, err := IndexExistsContext(context.Background(), database, test.name)
		if err != nil {
			t.Fatalf("index %s exists: %v", test.name, err)
		}
		if got != test.want {
			t.Errorf("IndexExistsContext(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}
