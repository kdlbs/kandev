package backendapp

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
)

// TestRecordRequiredStoreAdvancesItsDescriptorsSweep is the unit-level half
// of AC-PLATFORM-STARTUP-PROGRESS-005.7 ("recordRequiredStore advances the
// sweep its descriptor names, by one"): a successful admission advances the
// active sweep step by exactly one, and a failed admission - fatal to the
// caller - advances nothing.
func TestRecordRequiredStoreAdvancesItsDescriptorsSweep(t *testing.T) {
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{
		{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories},
		{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}, Sweep: startup.StepStoresRepositories},
	})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	reporter := startup.New(nil)
	ctx := startup.WithReporter(context.Background(), reporter)
	reporter.BeginStep(startup.StepStoresRepositories)
	reporter.SetTotal(startup.StepStoresRepositories, 2)

	if err := recordRequiredStore(ctx, tracker, "first", nil); err != nil {
		t.Fatalf("recordRequiredStore(first): %v", err)
	}
	if done := *reporter.Snapshot().Step.Done; done != 1 {
		t.Fatalf("Done after first admission = %d, want 1", done)
	}

	failErr := errors.New("boom")
	if err := recordRequiredStore(ctx, tracker, "second", failErr); err == nil {
		t.Fatal("recordRequiredStore(second, failErr) = nil error, want failErr")
	}
	if done := *reporter.Snapshot().Step.Done; done != 1 {
		t.Fatalf("Done after failed admission = %d, want unchanged at 1", done)
	}
}

// TestProvideRepositoriesReportsStoresRepositoriesSweep covers the
// stores.repositories half of AC-PLATFORM-STARTUP-PROGRESS-005.2/.7: every
// admission in provideRepositories (including the five inside
// provideSupportRepos) advances the sweep step its catalog descriptor names,
// and the sweep closes and hands off to stores.services at the same
// storage.go:215 boundary that already switches the phase - this is
// completeness test 4 (declared Phase matches the real call site) for these
// two steps, exercised end to end rather than centrally.
func TestProvideRepositoriesReportsStoresRepositoriesSweep(t *testing.T) {
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	reporter := startup.New(nil)
	ctx := startup.WithReporter(context.Background(), reporter)

	_, _, cleanups, err := provideRepositories(ctx, cfg, newTestLogger(), "test")
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

	wantRepositoriesTotal := int64(requiredstoresSweepTotal(t, startup.StepStoresRepositories))
	wantServicesTotal := int64(requiredstoresSweepTotal(t, startup.StepStoresServices))

	snap := reporter.Snapshot()
	if snap.Phase != startup.InitializingServices {
		t.Fatalf("Phase = %q, want %q", snap.Phase, startup.InitializingServices)
	}
	if snap.Step == nil {
		t.Fatal("Step = nil, want stores.services active after provideRepositories returns")
	}
	if snap.Step.ID != startup.StepStoresServices {
		t.Fatalf("Step.ID = %q, want %q (stores.repositories must have closed at the InitializingServices boundary)", snap.Step.ID, startup.StepStoresServices)
	}
	if snap.Step.Done == nil || *snap.Step.Done != 0 {
		t.Fatalf("Step.Done = %v, want 0 (stores.services has admitted nothing yet)", snap.Step.Done)
	}
	if snap.Step.Total == nil || *snap.Step.Total != wantServicesTotal {
		t.Fatalf("Step.Total = %v, want %d", snap.Step.Total, wantServicesTotal)
	}
	if wantRepositoriesTotal != 19 {
		t.Fatalf("stores.repositories catalog total = %d, want 19 (sanity check on the fixture)", wantRepositoriesTotal)
	}
}

// TestProvideRepositoriesStoresRepositoriesSweepReachesFullTotal covers the
// interleaving between the stores.repositories sweep and
// repository.ProvideContext's own migrations/backfills, which open and close
// several finer-grained steps (journal turns/messages, prompt_seq,
// message_timestamps, subagent_context) under the same ApplyingMigrations
// phase. On a fresh database (first-ever boot) every one of those backfill
// steps actually opens, so if stores.repositories's own BeginStep/EndStep
// window still overlapped theirs, its later recordRequiredStore admissions
// would silently no-op against a step that had already been ended out from
// under it, and it would never log "Startup step completed" at all. This
// asserts the sweep both logs its completion and reaches done == total ==
// 19, proving every admission after the nested steps still landed.
func TestProvideRepositoriesStoresRepositoriesSweepReachesFullTotal(t *testing.T) {
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	reporter := startup.New(log)
	ctx := startup.WithReporter(context.Background(), reporter)

	_, _, cleanups, err := provideRepositories(ctx, cfg, log, "test")
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

	var sawNestedStep bool
	var completedFields map[string]interface{}
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "Startup step started" && fields["step"] != string(startup.StepStoresRepositories) {
			sawNestedStep = true
		}
		if entry.Message == "Startup step completed" && fields["step"] == string(startup.StepStoresRepositories) {
			completedFields = fields
		}
	}
	if !sawNestedStep {
		t.Fatal("no nested step opened during provideRepositories on a fresh database; this test no longer exercises the interleaving it targets")
	}
	if completedFields == nil {
		t.Fatal(`no "Startup step completed" entry for stores.repositories: EndStep silently no-op'd against a step already ended by a nested BeginStep`)
	}
	wantTotal := int64(requiredstoresSweepTotal(t, startup.StepStoresRepositories))
	done, _ := completedFields["done"].(int64)
	total, _ := completedFields["total"].(int64)
	if done != wantTotal || total != wantTotal {
		t.Fatalf("stores.repositories completed with done=%v total=%v, want both %d", done, total, wantTotal)
	}
}

// TestStoresServicesSweepReachesFullTotalAfterStorageAdmission is the
// stores.services sibling of
// TestProvideRepositoriesStoresRepositoriesSweepReachesFullTotal. storage is
// stores.services's last admission chronologically, constructed by
// provideStorageStore. Production runs provideStorageStore (and the plugins
// journal mirror sync that must follow it) immediately before
// sessions.recovery's BeginStep specifically so that BeginStep never finds
// stores.services still open: were the order reversed, BeginStep would
// force-close stores.services early and every admission recorded up to that
// point -- including storage's own -- would be silently lost. This asserts
// the sweep both logs its completion and reaches done == total == 22 when
// admitted in that order, with no interleaving step in between.
func TestStoresServicesSweepReachesFullTotalAfterStorageAdmission(t *testing.T) {
	cfg := &config.Config{
		HomeDir:  t.TempDir(),
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	reporter := startup.New(log)
	ctx := startup.WithReporter(context.Background(), reporter)

	pool, repos, cleanups, err := provideRepositories(ctx, cfg, log, "test")
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

	services, _, err := provideServices(ctx, cfg, log, repos, pool, bus.NewMemoryEventBus(log), agentRegistry, "test")
	if err != nil {
		t.Fatalf("provideServices: %v", err)
	}
	if services.PluginsCleanup != nil {
		t.Cleanup(func() { _ = services.PluginsCleanup() })
	}

	// message-queue and delivery are recorded in production by
	// provideOrchestrator and startAgentInfrastructure respectively; record
	// them the same way here without standing up the full orchestrator and
	// lifecycle manager.
	if err := recordRequiredStore(ctx, repos.RequiredStores, "message-queue", nil); err != nil {
		t.Fatalf("recordRequiredStore(message-queue): %v", err)
	}
	if err := recordRequiredStore(ctx, repos.RequiredStores, "delivery", nil); err != nil {
		t.Fatalf("recordRequiredStore(delivery): %v", err)
	}

	if _, err := provideStorageStore(ctx, pool, repos.RequiredStores); err != nil {
		t.Fatalf("provideStorageStore: %v", err)
	}

	var completedFields map[string]interface{}
	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		if entry.Message == "Startup step completed" && fields["step"] == string(startup.StepStoresServices) {
			completedFields = fields
		}
	}
	if completedFields == nil {
		t.Fatal(`no "Startup step completed" entry for stores.services: EndStep silently no-op'd against a step already ended`)
	}
	wantTotal := int64(requiredstoresSweepTotal(t, startup.StepStoresServices))
	done, _ := completedFields["done"].(int64)
	total, _ := completedFields["total"].(int64)
	if done != wantTotal || total != wantTotal {
		t.Fatalf("stores.services completed with done=%v total=%v, want both %d", done, total, wantTotal)
	}
}

func requiredstoresSweepTotal(t *testing.T, sweep startup.StepID) int {
	t.Helper()
	tracker, err := requiredstores.NewCatalogTracker()
	if err != nil {
		t.Fatalf("NewCatalogTracker: %v", err)
	}
	return tracker.SweepTotal(sweep)
}
