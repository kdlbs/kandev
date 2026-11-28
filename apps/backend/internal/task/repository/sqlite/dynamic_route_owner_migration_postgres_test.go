package sqlite

import (
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/testutil"
)

// nativeRouteOwnerPostgresCutoverSteps lists the PostgreSQL failpoints. The
// PostgreSQL path alters the existing tables in place instead of copying them.
var nativeRouteOwnerPostgresCutoverSteps = []string{"alter_owner_columns", "owner_indexes"}

func openLegacyNativeRouteOwnerPostgres(t *testing.T) *sqlx.DB {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("seed baseline postgres schema: %v", err)
	}
	rewindNativeRouteOwnersToLegacy(t, db)
	return db
}

func TestNativeRouteOwnerMigrationPostgres_FreshSchemaIsFinalAndReplays(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
	seedLegacyTask(t, db, legacySeed{taskID: "task-route-owner-a", sessionID: "session-route-owner-a"}, nativeRouteOwnerSeedTime)
	assertNativeRouteOwnerFinalShape(t, db)
	schema := nativeRouteOwnerSchema(t, db)
	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay fresh postgres schema: %v", err)
	}
	assertNativeRouteOwnerDump(t, "fresh postgres route schema after replay", nativeRouteOwnerSchema(t, db), schema)
}

func TestNativeRouteOwnerMigrationPostgres_PreservesLegacyValuesAndReplays(t *testing.T) {
	runNativeRouteOwnerUpgradeAndReplay(t, openLegacyNativeRouteOwnerPostgres(t))
}

func TestNativeRouteOwnerMigrationPostgres_RollbackAtEveryFailpoint(t *testing.T) {
	for _, step := range nativeRouteOwnerPostgresCutoverSteps {
		t.Run(step, func(t *testing.T) {
			runNativeRouteOwnerRollbackAt(t, openLegacyNativeRouteOwnerPostgres(t), step)
		})
	}
}
