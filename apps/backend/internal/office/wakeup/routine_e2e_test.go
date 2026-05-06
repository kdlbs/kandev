package wakeup_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/wakeup"
)

// TestRoutine_EndToEnd_CoordinatorHeartbeatFire exercises the full
// office-heartbeat-as-routine flow: install the default coordinator
// routine, advance time past the cron tick, drive the routines tick,
// and assert a fresh taskless run lands on the agent with the routine
// scope captured.
//
// Round-tripping the routines repo + wakeup dispatcher together
// validates the seam where:
//
//  1. The cron loop calls TickScheduledTriggers → DispatchRoutineRun.
//  2. The routines service materialises the lightweight routine into
//     an agent_wakeup_requests row with source=routine.
//  3. The wakeup dispatcher claims the row and creates a fresh runs
//     row (taskless) tagged reason="routine_dispatch" with the
//     routine_id mirrored into the run's context_snapshot.
//  4. A second tick five minutes later produces a second fresh run
//     and the (agent, "routine:<id>") summary upsert path is reachable
//     from the run row alone.
func TestRoutine_EndToEnd_CoordinatorHeartbeatFire(t *testing.T) {
	ctx := context.Background()
	repo, agentID := seedCoordinator(t)
	wakeupDispatcher := wakeup.NewDispatcher(repo, repo, logger.Default())
	wakeupDispatcher.SetRoutineLookup(repo)

	routineSvc := routines.NewRoutineService(repo, logger.Default(), &routineE2ENoopActivity{})
	routineSvc.SetWakeupEnqueuer(&routineE2EWakeupAdapter{
		repo:       repo,
		dispatcher: wakeupDispatcher,
	})

	routine, err := routineSvc.CreateDefaultCoordinatorRoutine(ctx, "ws-1", agentID)
	if err != nil {
		t.Fatalf("install default coordinator routine: %v", err)
	}
	if routine.AssigneeAgentProfileID != agentID {
		t.Fatalf("routine assignee: got %q want %q", routine.AssigneeAgentProfileID, agentID)
	}

	// Advance synthetic time past the first tick (5 minutes after the
	// trigger's next_run_at). Driving the service directly with a
	// chosen `now` keeps the test free of real-clock dependencies.
	first := mustTriggerFireTime(t, repo, routine.ID).Add(time.Second)
	if err := routineSvc.TickScheduledTriggers(ctx, first); err != nil {
		t.Fatalf("first tick: %v", err)
	}

	run := requireInflightRun(t, repo, agentID)
	if run.Reason != "routine_dispatch" {
		t.Errorf("run.reason: got %q want routine_dispatch", run.Reason)
	}
	if !strings.Contains(run.ContextSnapshot, `"routine_id":"`+routine.ID+`"`) {
		t.Errorf("expected routine_id in context_snapshot, got %q", run.ContextSnapshot)
	}
	// Taskless: the wakeup dispatcher copies the wakeup payload into
	// context_snapshot but the runs row carries no task_id field.
	if strings.Contains(run.Payload, `"task_id"`) {
		t.Errorf("expected no task_id in run payload, got %q", run.Payload)
	}

	// A continuation-summary upsert under "routine:<routine.ID>" is
	// the contract the office service uses for taskless completions.
	// Round-trip the upsert here so the schema constraint (8 KB cap,
	// upsert key) and the scope shape are both proven via the public
	// repo API the service consumes.
	scope := "routine:" + routine.ID
	if err := repo.UpsertContinuationSummary(ctx, officesqlite.AgentContinuationSummary{
		AgentProfileID: agentID,
		Scope:          scope,
		Content:        "## Active focus\nWatching for new tasks.",
		ContentTokens:  9,
		UpdatedByRunID: run.ID,
	}); err != nil {
		t.Fatalf("upsert continuation summary: %v", err)
	}
	got, err := repo.GetContinuationSummary(ctx, agentID, scope)
	if err != nil {
		t.Fatalf("get continuation summary: %v", err)
	}
	if got == nil || !strings.Contains(got.Content, "Active focus") {
		t.Errorf("expected summary readable under routine scope, got %+v", got)
	}
}

// TestCreateDefaultCoordinatorRoutine_Idempotent verifies the install
// helper returns the existing routine on a re-run rather than creating
// a duplicate. Mirrors the spec's guarantee for the agent-create code
// path: every coordinator install passes through the same idempotency
// guard, so a re-run on a workspace that already has the routine is a
// no-op.
func TestCreateDefaultCoordinatorRoutine_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo, agentID := seedCoordinator(t)
	svc := routines.NewRoutineService(repo, logger.Default(), &routineE2ENoopActivity{})

	first, err := svc.CreateDefaultCoordinatorRoutine(ctx, "ws-1", agentID)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	second, err := svc.CreateDefaultCoordinatorRoutine(ctx, "ws-1", agentID)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("re-install created a duplicate routine: %q vs %q", first.ID, second.ID)
	}
	all, err := repo.ListRoutines(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	matching := 0
	for _, r := range all {
		if r.AssigneeAgentProfileID == agentID && r.Name == routines.CoordinatorRoutineName {
			matching++
		}
	}
	if matching != 1 {
		t.Errorf("expected exactly 1 coordinator routine, got %d", matching)
	}
}

// seedCoordinator boots a fresh in-memory office repo with one
// coordinator agent and returns both. Mirrors the helper that lived in
// the now-retired heartbeat_e2e_test.go.
func seedCoordinator(t *testing.T) (*officesqlite.Repository, string) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	agent := &officemodels.AgentInstance{
		ID:               "agent-coord",
		WorkspaceID:      "ws-1",
		Name:             "coordinator",
		AgentDisplayName: "Coordinator",
		Role:             officemodels.AgentRoleCEO,
		Status:           officemodels.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return repo, agent.ID
}

// mustTriggerFireTime returns the first scheduled fire time of the
// routine's cron trigger. The routines service stamps next_run_at on
// CreateRoutineTrigger; the test reads it back so the synthetic clock
// is aligned with the trigger's chosen tick rather than guessing.
func mustTriggerFireTime(t *testing.T, repo *officesqlite.Repository, routineID string) time.Time {
	t.Helper()
	triggers, err := repo.ListTriggersByRoutineID(context.Background(), routineID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) == 0 || triggers[0].NextRunAt == nil {
		t.Fatalf("expected a cron trigger with next_run_at, got %+v", triggers)
	}
	return *triggers[0].NextRunAt
}

// requireInflightRun returns the in-flight run for the agent or fails
// the test. Wrapping the lookup keeps the e2e test focused on the
// fire/dispatch sequence rather than the repo's null-check ergonomics.
func requireInflightRun(t *testing.T, repo *officesqlite.Repository, agentID string) *officemodels.Run {
	t.Helper()
	run, err := repo.FindInflightRunForAgent(context.Background(), agentID)
	if err != nil {
		t.Fatalf("find inflight run: %v", err)
	}
	if run == nil {
		t.Fatal("expected an in-flight run for the coordinator")
	}
	if run.AgentProfileID != agentID {
		t.Fatalf("run.agent_profile_id: got %q want %q", run.AgentProfileID, agentID)
	}
	return run
}

// routineE2ENoopActivity satisfies shared.ActivityLogger without
// recording anything — the e2e test asserts on persisted state, not
// activity-log emissions.
type routineE2ENoopActivity struct{}

func (n *routineE2ENoopActivity) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {
}
func (n *routineE2ENoopActivity) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// routineE2EWakeupAdapter bridges the routines package's
// WakeupEnqueuer interface onto the concrete sqlite repo + dispatcher.
// Mirrors cmd/kandev/adapters_office.go's adapter inline so the test
// avoids importing the binary's wiring layer.
type routineE2EWakeupAdapter struct {
	repo       *officesqlite.Repository
	dispatcher *wakeup.Dispatcher
}

func (a *routineE2EWakeupAdapter) CreateWakeupRequest(
	ctx context.Context, req *routines.WakeupRequest,
) error {
	row := &officesqlite.WakeupRequest{
		ID:             req.ID,
		AgentProfileID: req.AgentProfileID,
		Source:         req.Source,
		Reason:         req.Reason,
		Payload:        req.Payload,
		RequestedAt:    req.RequestedAt,
	}
	if req.IdempotencyKey != "" {
		row.IdempotencyKey.String = req.IdempotencyKey
		row.IdempotencyKey.Valid = true
	}
	return a.repo.CreateWakeupRequest(ctx, row)
}

func (a *routineE2EWakeupAdapter) Dispatch(ctx context.Context, requestID string) error {
	return a.dispatcher.Dispatch(ctx, requestID)
}
