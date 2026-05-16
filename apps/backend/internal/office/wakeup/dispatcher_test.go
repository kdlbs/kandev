package wakeup_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/wakeup"
)

// testHarness packages the dispatcher with its dependencies. Keeps each
// test brief and lets us seed agents with a chosen heartbeat policy.
type testHarness struct {
	t          *testing.T
	repo       *officesqlite.Repository
	dispatcher *wakeup.Dispatcher
	agentID    string
}

func newHarness(t *testing.T, policy string) *testHarness {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	log := logger.Default()

	// Seed an agent. The dispatcher no longer reads per-agent policy
	// fields (every scheduled wake flows through a routine now), but
	// the harness keeps the `policy` parameter to drive routine-sourced
	// dispatch tests below via SetRoutineLookup. For non-routine tests
	// the policy parameter is ignored — the dispatcher defaults to
	// coalesce_if_active.
	_ = policy
	agent := &officemodels.AgentInstance{
		ID:               "agent-1",
		WorkspaceID:      "ws-1",
		Name:             "ceo",
		AgentDisplayName: "CEO",
		Role:             officemodels.AgentRoleCEO,
		Status:           officemodels.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	return &testHarness{
		t:          t,
		repo:       repo,
		dispatcher: wakeup.NewDispatcher(repo, repo, log),
		agentID:    "agent-1",
	}
}

func (h *testHarness) seedWakeup(id, source, payload string) {
	h.t.Helper()
	if err := h.repo.CreateWakeupRequest(context.Background(), &officesqlite.WakeupRequest{
		ID: id, AgentProfileID: h.agentID, Source: source, Payload: payload,
	}); err != nil {
		h.t.Fatalf("seed wakeup: %v", err)
	}
}

func (h *testHarness) seedRun(id, status string) {
	h.t.Helper()
	run := &officemodels.Run{
		ID:              id,
		AgentProfileID:  h.agentID,
		Reason:          "routine",
		Payload:         "{}",
		Status:          status,
		CoalescedCount:  1,
		ContextSnapshot: `{"prior":"snapshot"}`,
	}
	if err := h.repo.CreateRun(context.Background(), run); err != nil {
		h.t.Fatalf("seed run: %v", err)
	}
}

func TestDispatch_NoInflight_CreatesFreshRun(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeup("w-1", wakeup.SourceSelf, `{"reason":"test"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, _ := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if got.Status != officesqlite.WakeupStatusClaimed {
		t.Errorf("status: got %q, want claimed", got.Status)
	}
	if got.RunID == "" {
		t.Fatal("expected run_id to be set")
	}
	run, err := h.repo.GetRunByID(context.Background(), got.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.AgentProfileID != "agent-1" {
		t.Errorf("agent_profile_id: %q", run.AgentProfileID)
	}
	if run.Reason != wakeup.SourceSelf {
		t.Errorf("reason: %q", run.Reason)
	}
	if !strings.Contains(run.ContextSnapshot, `"reason":"test"`) {
		t.Errorf("expected payload merged into context_snapshot, got %q", run.ContextSnapshot)
	}
}

func TestDispatch_CoalesceIntoQueuedRun(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRun("run-pre", "queued")
	h.seedWakeup("w-1", wakeup.SourceComment, `{"task_id":"t-1","comment_id":"c-1"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	gotReq, _ := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gotReq.Status != officesqlite.WakeupStatusCoalesced {
		t.Errorf("status: got %q want coalesced", gotReq.Status)
	}
	if gotReq.RunID != "run-pre" {
		t.Errorf("run_id: %q", gotReq.RunID)
	}
	gotRun, _ := h.repo.GetRunByID(context.Background(), "run-pre")
	if gotRun.CoalescedCount != 2 {
		t.Errorf("coalesced_count: got %d want 2", gotRun.CoalescedCount)
	}
	if !strings.Contains(gotRun.ContextSnapshot, `"task_id":"t-1"`) {
		t.Errorf("expected merged context snapshot, got %q", gotRun.ContextSnapshot)
	}
}

func TestDispatch_CoalesceIntoClaimedRun(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRun("run-running", "claimed")
	h.seedWakeup("w-1", wakeup.SourceComment, `{"task_id":"t-1","comment_id":"c-1"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	gotReq, _ := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if gotReq.Status != officesqlite.WakeupStatusCoalesced {
		t.Errorf("expected coalesced, got %q", gotReq.Status)
	}
	if gotReq.RunID != "run-running" {
		t.Errorf("run_id: %q", gotReq.RunID)
	}
}

func TestDispatch_AlreadyProcessed_NoOp(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeup("w-1", wakeup.SourceSelf, "{}")
	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	// Second dispatch is a no-op (status is already claimed).
	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
}

// -- PR 3 (routine-sourced policy resolution) tests --

// fakeRoutineLookup returns a fixed routine for every id. Lets
// dispatcher tests vary the routine.ConcurrencyPolicy without seeding
// rows through the office repo's CreateRoutine path (which has its own
// schema requirements).
type fakeRoutineLookup struct {
	policy string
}

func (f *fakeRoutineLookup) GetRoutine(_ context.Context, id string) (*officemodels.Routine, error) {
	return &officemodels.Routine{ID: id, ConcurrencyPolicy: f.policy}, nil
}

// dispatchRoutineWith seeds an in-flight run + a routine wakeup-request
// then dispatches with the given routine policy. Returns the resulting
// wakeup status so policy translation is unambiguous in assertions.
func dispatchRoutineWith(t *testing.T, policy string) string {
	t.Helper()
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.dispatcher.SetRoutineLookup(&fakeRoutineLookup{policy: policy})
	h.seedRun("run-pre", "queued")
	h.seedWakeup("w-1", wakeup.SourceRoutine, `{"routine_id":"r-1"}`)
	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, _ := h.repo.GetWakeupRequest(context.Background(), "w-1")
	return got.Status
}

func TestDispatch_Routine_SkipIfActive(t *testing.T) {
	if got := dispatchRoutineWith(t, "skip_if_active"); got != officesqlite.WakeupStatusSkipped {
		t.Errorf("status = %q, want skipped", got)
	}
}

func TestDispatch_Routine_CoalesceIfActive(t *testing.T) {
	if got := dispatchRoutineWith(t, "coalesce_if_active"); got != officesqlite.WakeupStatusCoalesced {
		t.Errorf("status = %q, want coalesced", got)
	}
}

func TestDispatch_Routine_AlwaysCreate_CreatesFreshRunWithInflight(t *testing.T) {
	// "always_create" is the routines-package legacy spelling for the
	// wakeup-layer "always_enqueue" policy. It must produce a fresh
	// run even when an in-flight one already exists.
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.dispatcher.SetRoutineLookup(&fakeRoutineLookup{policy: "always_create"})
	h.seedRun("run-pre", "queued")
	h.seedWakeup("w-1", wakeup.SourceRoutine, `{"routine_id":"r-1"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, _ := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if got.Status != officesqlite.WakeupStatusClaimed {
		t.Errorf("status = %q, want claimed", got.Status)
	}
	if got.RunID == "" || got.RunID == "run-pre" {
		t.Errorf("expected a fresh run, got %q", got.RunID)
	}
}

func TestDispatch_Routine_NoLookupFallsBackToCoalesce(t *testing.T) {
	// Without a RoutineLookup wired the dispatcher must default to
	// coalesce so a misconfigured wiring path doesn't escalate to
	// always_enqueue (which would defeat the bottleneck guard).
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRun("run-pre", "queued")
	h.seedWakeup("w-1", wakeup.SourceRoutine, `{"routine_id":"r-1"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, _ := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if got.Status != officesqlite.WakeupStatusCoalesced {
		t.Errorf("status = %q, want coalesced (default)", got.Status)
	}
}

func TestCreateWakeupRequest_IdempotencyConflictReturnsSentinel(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	first := &officesqlite.WakeupRequest{
		ID: "w-1", AgentProfileID: h.agentID, Source: wakeup.SourceRoutine,
		IdempotencyKey: sql.NullString{String: "k1", Valid: true},
	}
	if err := h.repo.CreateWakeupRequest(context.Background(), first); err != nil {
		t.Fatalf("first: %v", err)
	}
	second := &officesqlite.WakeupRequest{
		ID: "w-2", AgentProfileID: h.agentID, Source: wakeup.SourceRoutine,
		IdempotencyKey: sql.NullString{String: "k1", Valid: true},
	}
	err := h.repo.CreateWakeupRequest(context.Background(), second)
	if !errors.Is(err, officesqlite.ErrWakeupIdempotencyConflict) {
		t.Errorf("expected sentinel; got %v", err)
	}
}
