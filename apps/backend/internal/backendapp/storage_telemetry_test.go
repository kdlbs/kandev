package backendapp

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/telemetrycontract"
)

// TestProvideRepositoriesActivatesTelemetryContracts is the composition-level
// regression test for activateTelemetryContracts (storage.go:109 call site,
// :222 declaration): internal/telemetrycontract's own tests only exercise the
// underlying Store.Activate directly, so nothing proves the real boot path
// still calls it. A regression that dropped the call site, or moved it ahead
// of a repository's initSchema, would leave telemetry_activations silently
// missing rows on every real boot with the rest of the suite fully green —
// the same class of composition-level boot regression coverage this package
// uses elsewhere for provideRepositories' other post-initSchema side effects
// (see, for example, TestPostgresBootInitializesRepositories in
// postgres_boot_test.go).
//
// telemetry_activations itself is created unconditionally by
// telemetrycontract.NewWithDB, independent of ordering, so a row-count
// assertion alone cannot catch activateTelemetryContracts running ahead of a
// contract's backing repository's initSchema — Store.Activate writes the
// activation row regardless of whether those backing objects exist yet. The
// ordering-sensitive signal is LogHealth's per-contract "exists" field, so
// this test also captures the boot log via an observer core and asserts
// every registered contract reports exists=true.
func TestProvideRepositoriesActivatesTelemetryContracts(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	core, recorded := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}

	pool, _, cleanups, err := provideRepositories(ctx, cfg, log, "test")
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	})
	if err != nil {
		t.Fatalf("provideRepositories: %v", err)
	}

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

		exists, found := contractExistsFromHealthLog(recorded, c.Key, c.Version)
		if !found {
			t.Errorf("no telemetry.contract.health log entry for %s v%d (activateTelemetryContracts must call LogHealth during provideRepositories boot)",
				c.Key, c.Version)
			continue
		}
		if !exists {
			t.Errorf("telemetry.contract.health exists=%v for %s v%d, want true (activateTelemetryContracts must run after every repository's initSchema, so the contract's backing objects already exist)",
				exists, c.Key, c.Version)
		}
	}
}

// contractExistsFromHealthLog finds the "telemetry.contract.health" entry for
// (key, version) in the observed log and reports its "exists" field.
func contractExistsFromHealthLog(recorded *observer.ObservedLogs, key string, version int) (exists bool, found bool) {
	for _, entry := range recorded.All() {
		if entry.Message != "telemetry.contract.health" {
			continue
		}
		fields := entry.ContextMap()
		if fields["contract_key"] != key {
			continue
		}
		if v, ok := fields["contract_version"].(int64); !ok || int(v) != version {
			continue
		}
		existsVal, _ := fields["exists"].(bool)
		return existsVal, true
	}
	return false, false
}
