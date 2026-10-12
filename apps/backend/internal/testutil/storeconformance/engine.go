package storeconformance

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
)

// EngineName identifies a database driver exercised by the conformance suite.
type EngineName string

const (
	EngineSQLite   EngineName = "sqlite3"
	EnginePostgres EngineName = "pgx"
)

// Engine is the database handle supplied to an adapter callback.
type Engine struct {
	Name EngineName
	DB   *sqlx.DB
}

// OpenEngine opens an isolated database for a conformance or upgrade test.
// PostgreSQL is skipped only after adapter completeness has been validated by
// the caller.
func OpenEngine(t *testing.T, name EngineName, postgresDSN string) Engine {
	return openEngine(t, name, postgresDSN)
}

func openEngine(t *testing.T, name EngineName, postgresDSN string) Engine {
	t.Helper()
	switch name {
	case EngineSQLite:
		path := filepath.Join(t.TempDir(), "store-conformance.db")
		raw, err := db.OpenSQLite(path)
		if err != nil {
			t.Fatalf("open SQLite conformance database: %v", err)
		}
		database := sqlx.NewDb(raw, string(EngineSQLite))
		t.Cleanup(func() { _ = database.Close() })
		return Engine{Name: name, DB: database}
	case EnginePostgres:
		if postgresDSN == "" {
			postgresDSN = os.Getenv("KANDEV_TEST_POSTGRES_DSN")
		}
		if postgresDSN == "" {
			t.Skip("set KANDEV_TEST_POSTGRES_DSN to run PostgreSQL conformance")
		}
		return Engine{Name: name, DB: testutil.OpenIsolatedPostgres(t, postgresDSN)}
	default:
		t.Fatalf("unknown conformance engine %q", name)
		return Engine{}
	}
}

func validateEngine(name EngineName) error {
	if name != EngineSQLite && name != EnginePostgres {
		return fmt.Errorf("unknown conformance engine %q", name)
	}
	return nil
}
