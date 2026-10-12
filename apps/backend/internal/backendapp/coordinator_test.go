package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/startup"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	_ "github.com/mattn/go-sqlite3"
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

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, false, false, newTestLogger())
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

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, true, false, newTestLogger())
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

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, false, false, newTestLogger())
	if err == nil {
		t.Fatal("expected an error when the coordinator store fails to initialize")
	}
	if svc != nil {
		t.Fatal("expected a nil service on store initialization failure")
	}
}

// TestCoordinatorStandingInstructionsReader_BuildsContentFromTheCoordinator
// verifies the closure wired onto orchestrator.Service in main.go: it reads
// the named coordinator's name/context through the service and renders the
// Standing Instructions block from internal/coordinator/prompt.go
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions).
func TestCoordinatorStandingInstructionsReader_BuildsContentFromTheCoordinator(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, newTestLogger())

	ctx := context.Background()
	seed := &coordinator.Coordinator{
		WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e",
		Context: "watch the release queue",
	}
	if err := store.CreateCoordinator(ctx, seed); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}

	content, err := coordinatorStandingInstructionsReader(svc, newTestLogger())(ctx, seed.ID, "Acme Workspace", "ws-1")
	if err != nil {
		t.Fatalf("reader unexpected error: %v", err)
	}
	for _, want := range []string{
		"Ops", "Acme Workspace", "ws-1", "watch the release queue",
		"proposal tools available in this conversation", "A human must decide each proposal",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("reader content missing %q, got:\n%s", want, content)
		}
	}
}

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error {
	return nil
}

// TestCoordinatorStandingInstructionsReader_AppendsStandingOrders verifies the
// orders section follows the base block after one blank line only while phase
// 2 is on, and that an unreadable orders table drops that section alone.
func TestCoordinatorStandingInstructionsReader_AppendsStandingOrders(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	ctx := context.Background()
	seed := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "ctx"}
	if err := store.CreateCoordinator(ctx, seed); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	on := coordinator.NewService(store, coordinator.NewValidator(nil, nil), allowAllAuthorizer{}, newTestLogger(), coordinator.WithPhase2(true))
	off := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, newTestLogger())
	if _, err := on.AddStandingOrder(ctx, "ws-1", seed.ID, coordinator.AddStandingOrderInput{Text: "Prefer small cards."}); err != nil {
		t.Fatalf("AddStandingOrder: %v", err)
	}
	build := func(svc *coordinator.Service) string {
		content, err := coordinatorStandingInstructionsReader(svc, newTestLogger())(ctx, seed.ID, "Acme", "ws-1")
		if err != nil {
			t.Fatalf("reader: %v", err)
		}
		return content
	}
	base := build(off)
	if strings.Contains(base, "<standing-orders>") {
		t.Fatalf("flag off added the orders section:\n%s", base)
	}
	withOrders := build(on)
	if !strings.HasPrefix(withOrders, base+"\n\nStanding orders from this workspace's managers.") || !strings.Contains(withOrders, "Prefer small cards.") {
		t.Fatalf("orders section not appended after a blank line:\n%s", withOrders)
	}
	if _, err := pool.Writer().Exec(`DROP TABLE coordinator_standing_orders`); err != nil {
		t.Fatal(err)
	}
	if got := build(on); got != base+"\n\n"+noGoalSection {
		t.Fatalf("failed orders read changed the rest of the block:\n%s", got)
	}
}

const noGoalSection = "No goal is set for this coordinator."

// TestCoordinatorStandingInstructionsReader_AppendsGoal verifies the goal
// section follows the orders only while phase 2 is on, shows the active goal
// and that an unreadable goal drops that section alone.
func TestCoordinatorStandingInstructionsReader_AppendsGoal(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	if _, err := pool.Writer().Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, state TEXT,
		archived_at DATETIME, is_ephemeral INTEGER NOT NULL DEFAULT 0, origin TEXT)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seed := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e", Context: "ctx"}
	if err := store.CreateCoordinator(ctx, seed); err != nil {
		t.Fatalf("CreateCoordinator: %v", err)
	}
	on := coordinator.NewService(store, coordinator.NewValidator(nil, nil), allowAllAuthorizer{}, newTestLogger(), coordinator.WithPhase2(true))
	off := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, newTestLogger())
	build := func(svc *coordinator.Service) string {
		content, err := coordinatorStandingInstructionsReader(svc, newTestLogger())(ctx, seed.ID, "Acme", "ws-1")
		if err != nil {
			t.Fatalf("reader: %v", err)
		}
		return content
	}
	base := build(off)
	if got := build(on); got != base+"\n\n"+noGoalSection {
		t.Fatalf("no goal:\n%s", got)
	}
	if _, err := on.PutGoal(ctx, "ws-1", seed.ID, []byte(`{"name":"Ship","due_on":"2026-10-31","criteria":[{"text":"a"}]}`)); err != nil {
		t.Fatalf("PutGoal: %v", err)
	}
	want := base + "\n\n" + coordinator.GoalSection(&coordinator.Goal{Name: "Ship", DueOn: ptr("2026-10-31"), Criteria: []coordinator.GoalCriterion{{Text: "a"}}})
	if got := build(on); got != want || build(off) != base {
		t.Fatalf("active goal:\n%s", got)
	}
	if _, err := pool.Writer().Exec(`DROP TABLE coordinator_goals`); err != nil {
		t.Fatal(err)
	}
	if got := build(on); got != base {
		t.Fatalf("failed goal read changed the rest of the block:\n%s", got)
	}
}

func ptr(s string) *string { return &s }

// TestCoordinatorStandingInstructionsReader_PropagatesLookupFailure verifies
// an unknown coordinator id surfaces as an error rather than empty content,
// so the orchestrator side's fallback-to-bare-prompt behavior engages.
func TestCoordinatorStandingInstructionsReader_PropagatesLookupFailure(t *testing.T) {
	pool := newCoordinatorTestPool(t)
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), nil, newTestLogger())

	_, err = coordinatorStandingInstructionsReader(svc, newTestLogger())(context.Background(), "missing", "Acme Workspace", "ws-1")
	if err == nil {
		t.Fatal("expected an error for an unknown coordinator id")
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

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, true, false, newTestLogger())
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

	// Runs hooks synchronously (instead of delegating to the real
	// startCoordinatorBackgroundPass, which dispatches them on a detached
	// goroutine) so that by the time registerCoordinatorRoutes returns, the
	// decisions hook has already called StartApprovalSweep and registered on
	// svc.sweepWG — letting the cleanup below join it deterministically
	// instead of racing an async dispatch. T0-vs-registration ordering,
	// which is what this test checks, does not depend on that dispatch being
	// synchronous or not.
	t.Cleanup(func() { runCoordinatorBackgroundPass = startCoordinatorBackgroundPass })
	var gotT0 time.Time
	runCoordinatorBackgroundPass = func(ctx context.Context, t0 time.Time, hooks []func(context.Context, time.Time)) {
		gotT0 = t0
		for _, hook := range hooks {
			hook(ctx, t0)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		svc.WaitApprovalSweepStopped()
	})

	registerCoordinatorRoutes(routeParams{
		ctx:      ctx,
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

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, false, false, newTestLogger())
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

// TestRegisterCoordinatorSubscribers_NilInputsReturnsNoop mirrors the other
// registration functions' nil guard: called without an event bus or
// service, it must return a hook that does nothing rather than panic.
func TestRegisterCoordinatorSubscribers_NilInputsReturnsNoop(t *testing.T) {
	hook := registerCoordinatorSubscribers(nil, nil, nil, newTestLogger())
	hook(context.Background(), time.Now().UTC())
}

// TestRegisterCoordinatorSubscribers_WiresStallSubscriptionAndPruneHook
// verifies the actual wiring at task 04's registration site: calling
// registerCoordinatorSubscribers subscribes the coordinator package to
// task.stalled on the given bus (needs-you.md#stall-records), and the hook
// it returns runs startup pruning through the service.
func TestRegisterCoordinatorSubscribers_WiresStallSubscriptionAndPruneHook(t *testing.T) {
	tracker := newCoordinatorTestTracker(t)
	pool := newCoordinatorTestPool(t)
	log := newTestLogger()

	svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, true, false, log)
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}

	// A second Store handle onto the same pool, used only to seed/read rows
	// directly, bypassing svc's workspace-scope authorization (svc's
	// authorizer is a nil *taskservice.Service in this test, since no HTTP
	// request ever reaches these subscriber-driven paths).
	store, err := coordinator.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatalf("coordinator.NewStore: %v", err)
	}
	seed := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Ops", AgentProfileID: "a", ExecutorProfileID: "e"}
	if err := store.CreateCoordinator(context.Background(), seed); err != nil {
		t.Fatalf("seed CreateCoordinator: %v", err)
	}

	memBus := bus.NewMemoryEventBus(log)
	hook := registerCoordinatorSubscribers(nil, memBus, svc, log)

	lastEventAt := time.Now().UTC()
	stalledEvt := bus.NewEvent(events.TaskStalled, "task-service", map[string]interface{}{
		"task_id":        "task-1",
		"workspace_id":   "ws-1",
		"stalled_for":    (90 * time.Second).String(),
		"last_event_at":  lastEventAt.Format(time.RFC3339Nano),
		"detection_only": true,
	})
	if err := memBus.Publish(context.Background(), events.TaskStalled, stalledEvt); err != nil {
		t.Fatalf("Publish task.stalled: %v", err)
	}

	stalls, err := store.ListStalls(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListStalls: %v", err)
	}
	if len(stalls) != 1 || stalls[0].TaskID != "task-1" {
		t.Fatalf("ListStalls = %+v, want one row for task-1", stalls)
	}

	// Seed a second, 31-day-old stall directly, then run the hook: it must
	// prune that row without disturbing the fresh one task.stalled just
	// wrote.
	old := &coordinator.Stall{
		TaskID:       "task-old",
		WorkspaceID:  "ws-1",
		StalledForMs: 1000,
		LastEventAt:  lastEventAt,
		DetectedAt:   time.Now().UTC().Add(-31 * 24 * time.Hour),
	}
	if _, err := store.UpsertStall(context.Background(), old); err != nil {
		t.Fatalf("seed old UpsertStall: %v", err)
	}

	// The hook spawns a goroutine that releases its subscriptions on
	// ctx.Done(); use a cancellable context and cancel it before the test
	// ends so that goroutine doesn't leak into later tests' leak checks.
	hookCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	hook(hookCtx, time.Now().UTC())

	stalls, err = store.ListStalls(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListStalls after hook: %v", err)
	}
	if len(stalls) != 1 || stalls[0].TaskID != "task-1" {
		t.Fatalf("ListStalls after hook = %+v, want only the fresh task-1 row (task-old pruned)", stalls)
	}
}

func TestRegisterCoordinatorDecisions_WiresDepsAndRecoversStaleProposal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	harness := newBootStateTestHarness(t)
	ctx, cancel := context.WithCancel(context.Background())

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
	svc, err := initCoordinatorWiring(ctx, pool, tracker, harness.taskSvc, harness.workflowSvc, nil, true, false, newTestLogger())
	if err != nil {
		t.Fatalf("initCoordinatorWiring: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		if svc != nil {
			svc.WaitApprovalSweepStopped()
		}
	})
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
	if err := store.InsertProposal(ctx, proposal, false); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	staleClaimedAt := now.Add(-10 * time.Minute)
	matched, err := store.ClaimProposal(ctx, proposal.ID, "stale-token", proposal.Spec, "", staleClaimedAt)
	if err != nil || !matched {
		t.Fatalf("ClaimProposal: matched=%v err=%v", matched, err)
	}

	hook := registerCoordinatorDecisions(gin.New(), nil, svc, harness.taskSvc, harness.workflowSvc, newTestLogger())
	hook(ctx, now)

	got, err := store.GetProposal(ctx, workspaceID, coord.ID, proposal.ID, false)
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

func TestInitCoordinatorWiring_Phase2FollowsBothFlags(t *testing.T) {
	cases := []struct {
		enabled, phase2, wantSvc bool
	}{{false, false, false}, {false, true, false}, {true, false, true}, {true, true, true}}
	for _, tc := range cases {
		tracker := newCoordinatorTestTracker(t)
		pool := newCoordinatorTestPool(t)
		svc, err := initCoordinatorWiring(context.Background(), pool, tracker, nil, nil, nil, tc.enabled, tc.phase2, newTestLogger())
		if err != nil {
			t.Fatalf("initCoordinatorWiring: %v", err)
		}
		if (svc != nil) != tc.wantSvc {
			t.Fatalf("enabled=%v phase2=%v: service present = %v", tc.enabled, tc.phase2, svc != nil)
		}
		if svc != nil && svc.Phase2Enabled() != tc.phase2 {
			t.Fatalf("enabled=%v phase2=%v: Phase2Enabled = %v", tc.enabled, tc.phase2, svc.Phase2Enabled())
		}
	}
}
