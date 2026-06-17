package store

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresFreshSchemaInitializes(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := newSQLiteRepositoryWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
}

func TestPostgresHasDeletedAgentProfilesHandlesMissingAgent(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}

	hasDeleted, err := repo.HasDeletedAgentProfiles(context.Background(), "missing-agent")
	if err != nil {
		t.Fatalf("HasDeletedAgentProfiles: %v", err)
	}
	if hasDeleted {
		t.Fatal("HasDeletedAgentProfiles = true, want false")
	}
}
