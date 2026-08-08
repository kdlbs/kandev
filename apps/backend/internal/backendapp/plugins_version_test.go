package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/events/bus"
)

// TestProvideServicesWiresPluginsKandevVersion pins the production wiring of
// the running build version into the plugin service.
//
// internal/plugins has always been able to enforce a package's
// manifest.min_kandev_version, but checkMinKandevVersion short-circuits to
// nil while s.kandevVersion is empty — so for as long as SetKandevVersion had
// no production caller, the whole mechanism was a silent no-op in every
// shipped build while its unit tests stayed green. Only a test that goes
// through provideServices can catch that regressing again.
func TestProvideServicesWiresPluginsKandevVersion(t *testing.T) {
	const wantVersion = "9.8.7"

	services, _ := provideTestServices(t, wantVersion)
	if services.Plugins == nil {
		t.Fatal("provideServices returned a nil Plugins service")
	}
	if got := services.Plugins.KandevVersion(); got != wantVersion {
		t.Fatalf("Plugins.KandevVersion() = %q, want %q — min_kandev_version is unenforced without it", got, wantVersion)
	}
}

// provideTestServices boots the real service graph over a temp-dir SQLite
// database, the same way Run does, and returns the resulting services.
func provideTestServices(t *testing.T, version string) (*Services, *config.Config) {
	t.Helper()

	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	log := newTestLogger()

	pool, repos, cleanups, err := provideRepositories(context.Background(), cfg, log, version)
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

	agentRegistry, registryCleanup, err := registry.Provide(log)
	if err != nil {
		t.Fatalf("registry.Provide: %v", err)
	}
	t.Cleanup(func() {
		if registryCleanup != nil {
			_ = registryCleanup()
		}
	})

	services, _, err := provideServices(cfg, log, repos, pool, bus.NewMemoryEventBus(log), agentRegistry, version)
	if err != nil {
		t.Fatalf("provideServices: %v", err)
	}
	return services, cfg
}
