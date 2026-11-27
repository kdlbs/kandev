package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestExistingSQLiteTablesContextFiltersAndBindsNames(t *testing.T) {
	raw, err := OpenSQLite(filepath.Join(t.TempDir(), "tables.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	conn := sqlx.NewDb(raw, "sqlite3")
	for _, query := range []string{
		`CREATE TABLE first (id TEXT)`,
		`CREATE TABLE unrequested (id TEXT)`,
		`CREATE TABLE "quote'?,name" (id TEXT)`,
		`CREATE VIEW view_only AS SELECT id FROM first`,
		`CREATE INDEX index_only ON first(id)`,
	} {
		_, err := conn.Exec(query)
		require.NoError(t, err)
	}
	for _, test := range []struct {
		name   string
		tables []string
		want   map[string]bool
	}{
		{"filtered and deduplicated", []string{"first", "first", "missing", "quote'?,name", "view_only", "index_only"}, map[string]bool{"first": true, "quote'?,name": true}},
		{"all missing", []string{"missing"}, map[string]bool{}},
		{"empty", nil, map[string]bool{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ExistingSQLiteTablesContext(context.Background(), conn, test.tables)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
			require.Zero(t, conn.Stats().InUse)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := ExistingSQLiteTablesContext(ctx, conn, []string{"first"})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
	got, err = ExistingSQLiteTablesContext(context.Background(), sqlx.NewDb(raw, "pgx"), []string{"first"})
	require.ErrorContains(t, err, "requires the sqlite3 driver")
	require.Nil(t, got)
}
