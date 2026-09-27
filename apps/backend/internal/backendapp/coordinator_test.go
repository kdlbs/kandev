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

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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

// TestRegisterCoordinatorRoutes_CapturesT0BeforeRoutesRegister verifies the
// RV-001 invariant directly at its actual regression site: T0 must be
// captured before coordinator.RegisterRoutes is called, not merely before
// registerCoordinatorRoutes returns. It intercepts the real RegisterRoutes
// call via the registerCoordinatorHTTPRoutes test seam to record when routes
// actually register, and the background pass dispatch via the
// runCoordinatorBackgroundPass test seam to capture the T0 value that was
// computed, then asserts T0 is not after that registration time.
//
// Unlike TestStartCoordinatorBackgroundPass_RunsHooksWithProvidedT0 (which
// only proves the extracted helper forwards whatever T0 it is given) and
// TestRegisterCoordinatorRoutes_DisabledReturns404AndPreservesRows (which
// never reaches the T0 line at all, because svc is nil), this test fails if
// T0 capture is moved back to after the RegisterRoutes call: both seams fire
// synchronously and in call order, so a regression makes registeredAt
// strictly earlier than the captured T0.
func TestRegisterCoordinatorRoutes_CapturesT0BeforeRoutesRegister(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, true, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	if svc == nil {
		t.Fatal("expected a non-nil service when features.coordinator is enabled")
	}

	originalRegister := registerCoordinatorHTTPRoutes
	t.Cleanup(func() { registerCoordinatorHTTPRoutes = originalRegister })
	var registeredAt time.Time
	registerCoordinatorHTTPRoutes = func(router *gin.Engine, s *coordinator.Service, log *logger.Logger) {
		registeredAt = time.Now().UTC()
		originalRegister(router, s, log)
	}

	originalPass := runCoordinatorBackgroundPass
	t.Cleanup(func() { runCoordinatorBackgroundPass = originalPass })
	var gotT0 time.Time
	runCoordinatorBackgroundPass = func(ctx context.Context, t0 time.Time, hooks []func(context.Context, time.Time)) {
		gotT0 = t0
		originalPass(ctx, t0, hooks)
	}

	registerCoordinatorRoutes(routeParams{
		ctx:      context.Background(),
		router:   gin.New(),
		services: &Services{Coordinator: svc},
		log:      newTestLogger(),
	})

	if registeredAt.IsZero() {
		t.Fatal("expected registerCoordinatorHTTPRoutes spy to have run")
	}
	if gotT0.IsZero() {
		t.Fatal("expected runCoordinatorBackgroundPass spy to have run")
	}
	if gotT0.After(registeredAt) {
		t.Fatalf("T0 %v was captured after routes registered at %v; T0 must precede route registration", gotT0, registeredAt)
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

// TestRegisterCoordinatorDecisions_WiresDepsAndRecoversStaleProposal verifies
// task-07's registration function actually wires SetDecisionDeps with real
// dependencies instead of leaving the previous no-op stub in place: it uses
// the real task and workflow services registerCoordinatorRoutes has
// available at wiring time (docs/specs/coordinator/system-design/
// proposals.md#recovery), and the hook it returns is svc.StartupRecoveryPass.
// A proposal claimed well before t0 (simulating a claim left behind by a
// process that stopped) is recovered into "approved" with a freshly created
// task when the hook runs.
func TestRegisterCoordinatorDecisions_WiresDepsAndRecoversStaleProposal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newBootStateTestHarness(t)
	ctx := context.Background()

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("ListWorkspaces: workspaces=%d err=%v", len(workspaces), err)
	}
	workspaceID := workspaces[0].ID

	now := time.Now().UTC()
	const workflowID = "decisions-wiring-wf"
	const stepID = "decisions-wiring-step"
	if err := harness.taskRepo.CreateWorkflow(ctx, &taskmodels.Workflow{
		ID: workflowID, WorkspaceID: workspaceID, Name: "Decisions wiring WF", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := harness.workflowSvc.CreateStep(ctx, &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: workflowID, Name: "Start", Position: 0, IsStartStep: true,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}

	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)
	svc, err := initCoordinatorWiring(ctx, pool, tracker, harness.taskSvc, nil, true, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	if svc == nil {
		t.Fatal("expected a non-nil service when features.coordinator is enabled")
	}

	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	coord := &coordinator.Coordinator{WorkspaceID: workspaceID, Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(ctx, coord); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	proposal := &coordinator.Proposal{
		WorkspaceID:   workspaceID,
		CoordinatorID: coord.ID,
		Spec:          coordinator.ProposalSpec{Title: "Proposed task", WorkflowID: workflowID, StepID: stepID},
	}
	if err := store.InsertProposal(ctx, proposal); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	staleClaimedAt := now.Add(-10 * time.Minute)
	matched, err := store.ClaimProposal(ctx, proposal.ID, "stale-token", proposal.Spec, "", staleClaimedAt)
	if err != nil || !matched {
		t.Fatalf("ClaimProposal: matched=%v err=%v", matched, err)
	}

	hook := registerCoordinatorDecisions(gin.New(), nil, svc, harness.taskSvc, harness.workflowSvc, newTestLogger())
	hook(ctx, now)

	got, err := store.GetProposal(ctx, workspaceID, coord.ID, proposal.ID)
	if err != nil {
		t.Fatalf("GetProposal: %v", err)
	}
	if got.Status != coordinator.ProposalStatusApproved {
		t.Fatalf("Status = %q, want approved (recovery hook did not wire real deps)", got.Status)
	}
	if got.TaskID == nil {
		t.Fatal("expected TaskID to be set")
	}
	task, err := harness.taskSvc.GetTask(ctx, *got.TaskID)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", *got.TaskID, err)
	}
	if task.Title != "Proposed task" {
		t.Errorf("Task.Title = %q, want %q", task.Title, "Proposed task")
	}
}
