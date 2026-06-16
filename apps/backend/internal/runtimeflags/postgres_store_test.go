package runtimeflags

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresStoreRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewSQLiteStore(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteStore postgres: %v", err)
	}

	ctx := context.Background()
	if err := store.SetOverride(ctx, "features.office", true); err != nil {
		t.Fatalf("SetOverride true: %v", err)
	}
	if err := store.SetOverride(ctx, "features.office", false); err != nil {
		t.Fatalf("SetOverride false: %v", err)
	}

	overrides, err := store.ListOverrides(ctx)
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	value, ok := overrides["features.office"]
	if !ok {
		t.Fatal("features.office override missing after upsert")
	}
	if value {
		t.Fatal("features.office override = true, want false")
	}

	if err := store.DeleteOverride(ctx, "features.office"); err != nil {
		t.Fatalf("DeleteOverride: %v", err)
	}
	overrides, err = store.ListOverrides(ctx)
	if err != nil {
		t.Fatalf("ListOverrides after delete: %v", err)
	}
	if _, ok := overrides["features.office"]; ok {
		t.Fatal("features.office override still present after delete")
	}
}
