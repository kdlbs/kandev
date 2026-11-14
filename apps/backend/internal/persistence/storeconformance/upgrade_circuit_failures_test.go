package storeconformance

import (
	"context"
	"testing"

	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

// The previous stable circuit table has no failure count. Upgrading adds the
// column with a zero default on both dialects and keeps existing circuit rows.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21
func TestPreviousStableUpgrade_AddsCircuitFailureCount(t *testing.T) {
	manifest := loadUpgradeManifest(t)
	for _, engine := range []testconformance.EngineName{testconformance.EngineSQLite, testconformance.EnginePostgres} {
		engine := engine
		t.Run(string(engine)+"/"+previousStableTag, func(t *testing.T) {
			fixture := fixtureForEngine(t, manifest, engine)
			database := testconformance.OpenEngine(t, engine, "")
			if err := applyFixture(t, database, fixture); err != nil {
				t.Fatalf("apply %s fixture: %v", engine, err)
			}
			ctx := context.Background()
			if _, err := database.DB.ExecContext(ctx, database.DB.Rebind(`
				INSERT INTO dynamic_resource_circuits (resource_key, state, until_at, code, updated_at)
				VALUES (?, 'open', ?, 'rate_limited', ?)
			`), "fixture-v0930-circuit", "2026-01-01 00:00:00", "2026-01-01 00:00:00"); err != nil {
				t.Fatalf("seed legacy circuit row: %v", err)
			}
			if err := runCurrentInitialization(database); err != nil {
				t.Fatalf("current initialization: %v", err)
			}
			var failures int
			if err := database.DB.QueryRowxContext(ctx, database.DB.Rebind(
				`SELECT failure_count FROM dynamic_resource_circuits WHERE resource_key = ?`,
			), "fixture-v0930-circuit").Scan(&failures); err != nil {
				t.Fatalf("read dynamic_resource_circuits.failure_count: %v", err)
			}
			if failures != 0 {
				t.Fatalf("failure_count = %d, want 0", failures)
			}
		})
	}
}
