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
	"github.com/kandev/kandev/internal/office/shared"
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
		Status:          officemodels.RunStatus(status),
		CoalescedCount:  1,
		ContextSnapshot: `{"prior":"snapshot"}`,
	}
	if err := h.repo.CreateRun(context.Background(), run); err != nil {
		h.t.Fatalf("seed run: %v", err)
	}
}

// seedRunWithReason is seedRun with an explicit reason, for tests that
// exercise the idle-skip reason-classification promotion logic.
func (h *testHarness) seedRunWithReason(id, status, reason string) {
	h.t.Helper()
	run := &officemodels.Run{
		ID:              id,
		AgentProfileID:  h.agentID,
		Reason:          reason,
		Payload:         "{}",
		Status:          officemodels.RunStatus(status),
		CoalescedCount:  1,
		ContextSnapshot: `{"prior":"snapshot"}`,
	}
	if err := h.repo.CreateRun(context.Background(), run); err != nil {
		h.t.Fatalf("seed run: %v", err)
	}
}

// seedWakeupWithReason is seedWakeup with an explicit reason, mirroring
// how routines.RoutineService sets Reason via shared.RoutineDispatchReason
// before enqueuing.
func (h *testHarness) seedWakeupWithReason(id, source, payload, reason string) {
	h.t.Helper()
	if err := h.repo.CreateWakeupRequest(context.Background(), &officesqlite.WakeupRequest{
		ID: id, AgentProfileID: h.agentID, Source: source, Payload: payload, Reason: reason,
	}); err != nil {
		h.t.Fatalf("seed wakeup: %v", err)
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

// TestDispatch_EventCoalescingIntoQueuedCronRun_PromotesReason is the
// WO-46.1 Finding 1 regression test. A manual/webhook wakeup-request
// (event-classified) that coalesces into a still-queued cron run must
// promote the run's reason to the event classification — otherwise the
// run stays periodic-class and checkIdleSkip can silently discard the
// event trigger. See docs/specs/office/scheduler.md ("Event-triggered
// wakeups always proceed").
func TestDispatch_EventCoalescingIntoQueuedCronRun_PromotesReason(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReason("run-cron", "queued", shared.RunReasonRoutineDispatchCron)
	h.seedWakeupWithReason("w-event", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, shared.RunReasonRoutineDispatchEvent)

	if err := h.dispatcher.Dispatch(context.Background(), "w-event"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	gotReq, err := h.repo.GetWakeupRequest(context.Background(), "w-event")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.Status != officesqlite.WakeupStatusCoalesced {
		t.Errorf("status: got %q, want coalesced", gotReq.Status)
	}
	if gotReq.RunID != "run-cron" {
		t.Errorf("run_id: got %q, want run-cron", gotReq.RunID)
	}
	run, err := h.repo.GetRunByID(context.Background(), "run-cron")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("run.Reason = %q, want promoted to %q", run.Reason, shared.RunReasonRoutineDispatchEvent)
	}
	if shared.IsPeriodicTasklessWake(run.Reason) {
		t.Error("merged run must not classify as a periodic taskless wake — " +
			"the idle-skip gate could silently swallow the event trigger")
	}
	if run.CoalescedCount != 2 {
		t.Errorf("coalesced_count: got %d want 2", run.CoalescedCount)
	}
	if !strings.Contains(run.ContextSnapshot, `"routine_id":"r-1"`) {
		t.Errorf("expected merged context snapshot, got %q", run.ContextSnapshot)
	}
}

// TestDispatch_CronCoalescingIntoQueuedEventRun_DoesNotDemote is the
// monotonic half of the Finding 1 fix: a cron request merging into an
// already event-classified in-flight run must never demote the run
// back to periodic.
func TestDispatch_CronCoalescingIntoQueuedEventRun_DoesNotDemote(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReason("run-event", "queued", shared.RunReasonRoutineDispatchEvent)
	h.seedWakeupWithReason("w-cron", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, shared.RunReasonRoutineDispatchCron)

	if err := h.dispatcher.Dispatch(context.Background(), "w-cron"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	run, err := h.repo.GetRunByID(context.Background(), "run-event")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("run.Reason = %q, want unchanged %q (no demotion)", run.Reason, shared.RunReasonRoutineDispatchEvent)
	}
}

// TestDispatch_PeriodicCoalescingIntoQueuedPeriodicRun_DoesNotPromote
// pins the other half of the monotonicity rule: a periodic request
// merging into an already periodic-classified run must not be
// "promoted" either, even though both reasons are periodic-classified
// (heartbeat vs. routine_dispatch_cron). The condition guarding
// promotion short-circuits on inflight.Reason's classification before
// ever checking the new request's, so this case is the only one that
// exercises the `!IsPeriodicTasklessWake(reason)` clause at all.
func TestDispatch_PeriodicCoalescingIntoQueuedPeriodicRun_DoesNotPromote(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReason("run-heartbeat", "queued", shared.RunReasonHeartbeat)
	h.seedWakeupWithReason("w-cron", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, shared.RunReasonRoutineDispatchCron)

	if err := h.dispatcher.Dispatch(context.Background(), "w-cron"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	run, err := h.repo.GetRunByID(context.Background(), "run-heartbeat")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != shared.RunReasonHeartbeat {
		t.Errorf("run.Reason = %q, want unchanged %q (periodic->periodic is not a promotion)", run.Reason, shared.RunReasonHeartbeat)
	}
}

// TestDispatch_EventCoalescingIntoClaimedCronRun_CreatesFreshRun is the
// WO-46.1 Review-round-1 R1-F1 regression test. processRun evaluates
// checkIdleSkip against the *models.Run it captured at claim time and
// never re-reads Reason, so promoting a claimed run's persisted reason
// (as the queued case does) would race a decision already in flight —
// the scheduler could still idle-skip on the stale periodic reason
// while the wakeup-request is already marked coalesced, silently
// discarding the event trigger. The dispatcher must route the event
// to its own fresh run instead of coalescing into the claimed one.
func TestDispatch_EventCoalescingIntoClaimedCronRun_CreatesFreshRun(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReason("run-cron", "claimed", shared.RunReasonRoutineDispatchCron)
	h.seedWakeupWithReason("w-event", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, shared.RunReasonRoutineDispatchEvent)

	if err := h.dispatcher.Dispatch(context.Background(), "w-event"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	gotReq, err := h.repo.GetWakeupRequest(context.Background(), "w-event")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.Status != officesqlite.WakeupStatusClaimed {
		t.Errorf("status: got %q, want claimed (own run, not coalesced)", gotReq.Status)
	}
	if gotReq.RunID == "" || gotReq.RunID == "run-cron" {
		t.Errorf("expected a fresh run distinct from the claimed one, got %q", gotReq.RunID)
	}

	freshRun, err := h.repo.GetRunByID(context.Background(), gotReq.RunID)
	if err != nil {
		t.Fatalf("get fresh run: %v", err)
	}
	if freshRun.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("fresh run.Reason = %q, want %q", freshRun.Reason, shared.RunReasonRoutineDispatchEvent)
	}

	// The claimed run's reason must stay untouched — the scheduler may
	// already be mid-decision against it, so the fix must not mutate it.
	claimedRun, err := h.repo.GetRunByID(context.Background(), "run-cron")
	if err != nil {
		t.Fatalf("get claimed run: %v", err)
	}
	if claimedRun.Reason != shared.RunReasonRoutineDispatchCron {
		t.Errorf("claimed run.Reason = %q, want unchanged %q", claimedRun.Reason, shared.RunReasonRoutineDispatchCron)
	}
}

// TestDispatch_EmptyReasonFallsBackToSourceForPromotion is the WO-46.1
// Review-round-1 R1-F2 regression test. coalesceIntoInflightRun must
// use the same reason-or-source fallback as createFreshRun, so an event
// request with an empty Reason still promotes a periodic queued run
// instead of silently leaving it periodic-classified.
func TestDispatch_EmptyReasonFallsBackToSourceForPromotion(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReason("run-cron", "queued", shared.RunReasonRoutineDispatchCron)
	h.seedWakeup("w-1", wakeup.SourceComment, `{"task_id":"t-1","comment_id":"c-1"}`)

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	gotReq, err := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.Status != officesqlite.WakeupStatusCoalesced {
		t.Errorf("status: got %q, want coalesced", gotReq.Status)
	}
	if gotReq.RunID != "run-cron" {
		t.Errorf("run_id: got %q, want run-cron", gotReq.RunID)
	}
	run, err := h.repo.GetRunByID(context.Background(), "run-cron")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != wakeup.SourceComment {
		t.Errorf("run.Reason = %q, want promoted to source %q", run.Reason, wakeup.SourceComment)
	}
	if shared.IsPeriodicTasklessWake(run.Reason) {
		t.Error("merged run must not classify as a periodic taskless wake")
	}
	if run.CoalescedCount != 2 {
		t.Errorf("coalesced_count: got %d want 2", run.CoalescedCount)
	}
	if !strings.Contains(run.ContextSnapshot, `"task_id":"t-1"`) {
		t.Errorf("expected merged context snapshot, got %q", run.ContextSnapshot)
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
	return &officemodels.Routine{ID: id, ConcurrencyPolicy: officemodels.RoutineConcurrencyPolicy(f.policy)}, nil
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

// TestDispatch_UnknownPolicy_FallsBackToCoalesceAndPromotes exercises an
// unrecognized routine policy value. normaliseRoutinePolicy's own
// default maps it to PolicyCoalesceIfActive before Dispatch's switch
// ever sees it, so this runs the same `case PolicyCoalesceIfActive`
// path as an explicit coalesce_if_active policy — not dispatcher.go's
// switch default, which unknown *routine* policies never reach. Using
// a periodic in-flight run + event-classified request also forces that
// path through the same reason-promotion logic as an explicit
// PolicyCoalesceIfActive, so the branch cannot silently regress to a
// bare MarkWakeupRequestCoalesced call without losing this assertion.
func TestDispatch_UnknownPolicy_FallsBackToCoalesceAndPromotes(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.dispatcher.SetRoutineLookup(&fakeRoutineLookup{policy: "not_a_real_policy"})
	h.seedRunWithReason("run-cron", "queued", shared.RunReasonRoutineDispatchCron)
	h.seedWakeupWithReason("w-event", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, shared.RunReasonRoutineDispatchEvent)

	if err := h.dispatcher.Dispatch(context.Background(), "w-event"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	gotReq, err := h.repo.GetWakeupRequest(context.Background(), "w-event")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.Status != officesqlite.WakeupStatusCoalesced {
		t.Errorf("status = %q, want coalesced (unknown policy falls back to coalesce)", gotReq.Status)
	}
	if gotReq.RunID != "run-cron" {
		t.Errorf("run_id: %q, want run-cron", gotReq.RunID)
	}
	run, err := h.repo.GetRunByID(context.Background(), "run-cron")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Errorf("run.Reason = %q, want promoted to %q", run.Reason, shared.RunReasonRoutineDispatchEvent)
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
