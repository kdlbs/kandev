package store

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresFreshSchemaInitializes(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := newSQLiteRepositoryWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
}
