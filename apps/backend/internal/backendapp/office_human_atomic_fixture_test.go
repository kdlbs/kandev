package backendapp

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/stretchr/testify/require"
)

func newOfficeAtomicHarness(t *testing.T) *pluginMoveErrorBindingHarness {
	t.Helper()
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		return newPluginMoveErrorBindingHarness(t)
	}
	database := testutil.OpenIsolatedPostgres(t, dsn)
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)
	return newPluginMoveErrorBindingHarness(t, database)
}

// officeAtomicSecondWriter uses an independent pool and physical connection to
// the same disposable database. Canonical Tx reads still use admission's Tx.
func officeAtomicSecondWriter(t *testing.T, h *pluginMoveErrorBindingHarness) *workflowrepo.Repository {
	repo, _ := officeAtomicSecondWriterWithDB(t, h)
	return repo
}

func officeAtomicSecondWriterWithDB(t *testing.T, h *pluginMoveErrorBindingHarness) (*workflowrepo.Repository, *sqlx.DB) {
	t.Helper()
	var rawDB *sqlx.DB
	if h.db.DriverName() == "pgx" {
		var schema string
		require.NoError(t, h.db.QueryRow(`SELECT current_schema()`).Scan(&schema))
		dsn := testutil.PostgresDSNWithSearchPath(t, os.Getenv("KANDEV_TEST_POSTGRES_DSN"), schema)
		raw, err := internaldb.OpenPostgres(dsn, 1, 1)
		require.NoError(t, err)
		rawDB = sqlx.NewDb(raw, "pgx")
	} else {
		var seq int
		var name, path string
		require.NoError(t, h.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path))
		raw, err := internaldb.OpenSQLite(path)
		require.NoError(t, err)
		rawDB = sqlx.NewDb(raw, "sqlite3")
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	repo, err := workflowrepo.NewWithDB(rawDB, rawDB, nil)
	require.NoError(t, err)
	return repo, rawDB
}
