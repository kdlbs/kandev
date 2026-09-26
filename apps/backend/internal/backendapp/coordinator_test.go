package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
)

func newCoordinatorTestTracker(t *testing.T) *requiredstores.Tracker {
	t.Helper()
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{
		{ID: "coordinator", OwnerPackage: "internal/coordinator", RequiredTables: []string{"coordinators"}, Sweep: startup.StepStoresServices},
	})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	return tracker
}

func newCoordinatorTestPool(t *testing.T) *db.Pool {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sqlx.Open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return db.NewPool(conn, conn)
}

// TestInitCoordinatorWiring_DisabledBuildsStoreOnly verifies Build decision
// 15: with features.coordinator off, the store is still constructed and
// recorded (it is a requiredstores catalog entry), but no service is built.
func TestInitCoordinatorWiring_DisabledBuildsStoreOnly(t *testing.T) {
	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, false, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	if svc != nil {
		t.Fatal("expected a nil service when features.coordinator is disabled")
	}
	if _, ok := tracker.DescriptorSweep("coordinator"); !ok {
		t.Fatal("expected the coordinator descriptor to be registered")
	}
}

// TestInitCoordinatorWiring_EnabledBuildsService verifies that with
// features.coordinator on, the store, validator and service are all built.
func TestInitCoordinatorWiring_EnabledBuildsService(t *testing.T) {
	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, true, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	if svc == nil {
		t.Fatal("expected a non-nil service when features.coordinator is enabled")
	}
}

// TestInitCoordinatorWiring_StoreErrorPropagates verifies that a store
// construction failure is fatal, even when features.coordinator is disabled.
func TestInitCoordinatorWiring_StoreErrorPropagates(t *testing.T) {
	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)
	if err := pool.Writer().Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, false, newTestLogger())
	if err == nil {
		t.Fatal("expected an error when the coordinator store fails to initialize")
	}
	if svc != nil {
		t.Fatal("expected a nil service on store initialization failure")
	}
}

// TestStartCoordinatorBackgroundPass_RunsHooksWithProvidedT0 verifies the
// RV-001 fix: the background pass must not record T0 itself (that happens
// before routes register, in registerCoordinatorRoutes); it must run each
// hook with the exact T0 it was given.
func TestStartCoordinatorBackgroundPass_RunsHooksWithProvidedT0(t *testing.T) {
	fixedT0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	received := make(chan time.Time, 1)
	hooks := []func(context.Context, time.Time){
		func(_ context.Context, t0 time.Time) { received <- t0 },
	}

	startCoordinatorBackgroundPass(context.Background(), fixedT0, hooks)

	select {
	case got := <-received:
		if !got.Equal(fixedT0) {
			t.Fatalf("hook received t0 %v, want %v", got, fixedT0)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background pass hook")
	}
}

// TestRegisterCoordinatorRoutes_DisabledReturns404AndPreservesRows verifies
// Build decision 15 at the HTTP level (RV-007): with features.coordinator
// off, initCoordinatorWiring's nil service makes registerCoordinatorRoutes a
// no-op, so a coordinator route 404s, while rows already in the
// unconditionally-built store are untouched.
func TestRegisterCoordinatorRoutes_DisabledReturns404AndPreservesRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, false, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	if svc != nil {
		t.Fatal("expected a nil service when features.coordinator is disabled")
	}

	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	seed := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(context.Background(), seed); err != nil {
		t.Fatalf("seed CreateCoordinator: %v", err)
	}

	router := gin.New()
	registerCoordinatorRoutes(routeParams{
		ctx:      context.Background(),
		router:   router,
		services: &Services{Coordinator: svc},
		log:      newTestLogger(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws-1/coordinators", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (flag off: routes never registered)", rec.Code, http.StatusNotFound)
	}

	found, err := store.GetCoordinator(context.Background(), "ws-1", seed.ID)
	if err != nil {
		t.Fatalf("GetCoordinator after flag-off route registration: %v", err)
	}
	if found.ID != seed.ID {
		t.Errorf("GetCoordinator().ID = %q, want %q", found.ID, seed.ID)
	}
}
