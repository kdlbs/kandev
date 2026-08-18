package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/telemetrycontract"
)

// TestProvideRepositoriesActivatesTelemetryContracts is the composition-level
// regression test for activateTelemetryContracts (storage.go:109 call site,
// :222 declaration): internal/telemetrycontract's own tests only exercise the
// underlying Store.Activate directly, so nothing proves the real boot path
// still calls it. A regression that dropped the call site, or moved it ahead
// of a repository's initSchema, would leave telemetry_activations silently
// missing rows on every real boot with the rest of the suite fully green —
// mirrors TestProvideRepositoriesBackfillsSessionCachedTokens's rationale for
// the neighboring backfillSessionCachedTokens call.
func TestProvideRepositoriesActivatesTelemetryContracts(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	log := newTestLogger()

	pool, _, cleanups, err := provideRepositories(ctx, cfg, log, "test")
	if err != nil {
		t.Fatalf("provideRepositories: %v", err)
	}
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	})

	registry := telemetrycontract.Registry()
	if len(registry) == 0 {
		t.Fatal("telemetrycontract.Registry() is empty; nothing for this test to assert")
	}
	writer := pool.Writer()
	for _, c := range registry {
		var count int
		if err := writer.QueryRowx(writer.Rebind(
			`SELECT COUNT(*) FROM telemetry_activations WHERE contract_key = ? AND contract_version = ?`),
			c.Key, c.Version,
		).Scan(&count); err != nil {
			t.Fatalf("query telemetry_activations for %s v%d: %v", c.Key, c.Version, err)
		}
		if count != 1 {
			t.Errorf("telemetry_activations row count for %s v%d = %d, want 1 (activateTelemetryContracts must run during provideRepositories boot)",
				c.Key, c.Version, count)
		}
	}
}
