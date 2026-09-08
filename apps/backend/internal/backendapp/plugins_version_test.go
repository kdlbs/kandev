package backendapp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/secrets"
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

	services, _, _ := provideTestServices(t, wantVersion)
	if services.Plugins == nil {
		t.Fatal("provideServices returned a nil Plugins service")
	}
	if got := services.Plugins.KandevVersion(); got != wantVersion {
		t.Fatalf("Plugins.KandevVersion() = %q, want %q — min_kandev_version is unenforced without it", got, wantVersion)
	}
}

func TestRequiredStoreBootstrapCompleteness(t *testing.T) {
	_, _, repos := provideTestServices(t, "test-bootstrap-completeness")

	// Repository and service construction cover every catalog entry except the
	// three stores deliberately initialized in later startup phases.
	wantPending := []string{"message-queue", "delivery", "storage"}
	gotPending := repos.RequiredStores.UnavailableStoreIDs()
	if !reflect.DeepEqual(gotPending, wantPending) {
		t.Fatalf("pending required stores = %v, want %v", gotPending, wantPending)
	}
}

func TestRequiredStoreFailure(t *testing.T) {
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "task", OwnerPackage: "internal/task", RequiredTables: []string{"tasks"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	constructorErr := errors.New("schema constructor failed")
	got := recordRequiredStore(tracker, "task", constructorErr)
	if !errors.Is(got, constructorErr) {
		t.Fatalf("recordRequiredStore() error = %v, want %v", got, constructorErr)
	}
	if !strings.Contains(got.Error(), `required store "task"`) {
		t.Fatalf("recordRequiredStore() error = %q, want store ID", got)
	}
	if err := tracker.ValidateComplete(); err == nil || !strings.Contains(err.Error(), "task") {
		t.Fatalf("ValidateComplete() error = %v, want failed task store", err)
	}
}

func TestExternalProviderFailureIsolation(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITHUB", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", t.TempDir())

	pool := newMessageQueueSettingsTestPool(t)
	log := newTestLogger()
	externalErr := errors.New("credential backend unavailable")
	service, cleanup, err := initGitHubServiceRequired(
		&config.Config{},
		pool,
		bus.NewMemoryEventBus(log),
		failingExternalSecretStore{err: externalErr},
		log,
	)
	if err != nil {
		t.Fatalf("external credential failure made local store fatal: %v", err)
	}
	if service == nil {
		t.Fatal("GitHub service is nil after external credential failure")
	}
	if cleanup != nil {
		t.Cleanup(func() { _ = cleanup() })
	}
	if _, err := service.GetWorkspaceSettings(context.Background(), "external-failure-workspace"); err != nil {
		t.Fatalf("local GitHub store unavailable after credential failure: %v", err)
	}
}

// provideTestServices boots the real service graph over a temp-dir SQLite
// database, the same way Run does, and returns the resulting services.
func provideTestServices(t *testing.T, version string) (*Services, *config.Config, *Repositories) {
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
	if services.Workflow != nil {
		t.Cleanup(func() { _ = services.Workflow.Close() })
	}
	return services, cfg, repos
}

type failingExternalSecretStore struct {
	emptySecretStore
	err error
}

func (s failingExternalSecretStore) List(context.Context) ([]*secrets.SecretListItem, error) {
	return nil, s.err
}
