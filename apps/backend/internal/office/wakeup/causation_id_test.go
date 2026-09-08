package wakeup_test

import (
	"context"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/office/wakeup"
)

func (h *testHarness) seedWakeupWithCausation(id, source, payload, causationID string) {
	h.t.Helper()
	if err := h.repo.CreateWakeupRequest(context.Background(), &officesqlite.WakeupRequest{
		ID: id, AgentProfileID: h.agentID, Source: source, Payload: payload, CausationID: causationID,
	}); err != nil {
		h.t.Fatalf("seed wakeup: %v", err)
	}
}

func (h *testHarness) seedRunWithReasonAndCausation(id, status, reason, causationID string) {
	h.t.Helper()
	run := &officemodels.Run{
		ID:              id,
		AgentProfileID:  h.agentID,
		Reason:          reason,
		Payload:         "{}",
		Status:          officemodels.RunStatus(status),
		CoalescedCount:  1,
		ContextSnapshot: `{"prior":"snapshot"}`,
		CausationID:     causationID,
	}
	if err := h.repo.CreateRun(context.Background(), run); err != nil {
		h.t.Fatalf("seed run: %v", err)
	}
}

// AC-OFFICE-LOOP-LIVENESS-002.3: createFreshRun copies the causation id
// from the wakeup request onto the fresh run.
func TestDispatch_FreshRunCopiesCausationIDFromWakeup(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedWakeupWithCausation("w-1", wakeup.SourceSelf, `{"reason":"test"}`, "cause-fresh-1")

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, err := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	run, err := h.repo.GetRunByID(context.Background(), got.RunID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.CausationID != "cause-fresh-1" {
		t.Fatalf("run.CausationID = %q, want cause-fresh-1", run.CausationID)
	}
}

// AC-OFFICE-LOOP-LIVENESS-002.4: coalescing touches only the wakeup
// request, never the run. The in-flight run keeps the id of the wake
// that created it; the coalesced request keeps its own — the two stay
// joinable through agent_wakeup_requests.run_id.
func TestDispatch_CoalesceLeavesRunCausationIDUntouched(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReasonAndCausation("run-pre", "queued", shared.RunReasonHeartbeat, "cause-run-original")
	h.seedWakeupWithCausation("w-1", wakeup.SourceComment, `{"task_id":"t-1"}`, "cause-wakeup-coalesced")

	if err := h.dispatcher.Dispatch(context.Background(), "w-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	gotReq, err := h.repo.GetWakeupRequest(context.Background(), "w-1")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.CausationID != "cause-wakeup-coalesced" {
		t.Fatalf("coalesced request CausationID = %q, want unchanged cause-wakeup-coalesced", gotReq.CausationID)
	}
	if gotReq.RunID != "run-pre" {
		t.Fatalf("coalesced request run_id = %q, want run-pre", gotReq.RunID)
	}

	gotRun, err := h.repo.GetRunByID(context.Background(), "run-pre")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.CausationID != "cause-run-original" {
		t.Fatalf("run.CausationID = %q, want unchanged cause-run-original (coalesce must not touch it)", gotRun.CausationID)
	}
}

// AC-002.4 skip half: MarkWakeupRequestSkipped touches only the
// request; it has no run to touch, and the request's own causation id
// survives the transition.
func TestDispatch_SkipLeavesWakeupCausationIDUntouched(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	// skip_if_active requires the routine lookup to resolve that policy;
	// simplest deterministic path here is a non-routine source with an
	// in-flight run under the default coalesce policy is not skip — so
	// drive skip via the routine lookup fallback used elsewhere in this
	// package (skip_if_active policy routine).
	h.dispatcher.SetRoutineLookup(fakeRoutineLookupSkip{})
	h.seedRunWithReasonAndCausation("run-inflight", "claimed", shared.RunReasonRoutineDispatchCron, "cause-inflight")
	h.seedWakeupWithCausation("w-skip", wakeup.SourceRoutine, `{"routine_id":"r-skip"}`, "cause-skip-1")

	if err := h.dispatcher.Dispatch(context.Background(), "w-skip"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	got, err := h.repo.GetWakeupRequest(context.Background(), "w-skip")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if got.Status != officesqlite.WakeupStatusSkipped {
		t.Fatalf("status = %q, want skipped", got.Status)
	}
	if got.CausationID != "cause-skip-1" {
		t.Fatalf("skipped request CausationID = %q, want unchanged cause-skip-1", got.CausationID)
	}
}

// AC-002.4/AC-002.5: PromoteRunAndCoalesceWakeupIfQueued promotes the
// run's reason but must never touch causation_id — a promotion is not a
// new cause.
func TestDispatch_PromotionDoesNotChangeRunCausationID(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReasonAndCausation("run-cron", "queued", shared.RunReasonRoutineDispatchCron, "cause-cron-original")
	h.seedWakeupWithCausation("w-event", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, "cause-event-1")
	// Give the event wakeup an event-classified reason so promotion fires.
	if _, err := h.repo.ExecRaw(context.Background(),
		`UPDATE agent_wakeup_requests SET reason = ? WHERE id = ?`,
		shared.RunReasonRoutineDispatchEvent, "w-event"); err != nil {
		t.Fatalf("seed reason: %v", err)
	}

	if err := h.dispatcher.Dispatch(context.Background(), "w-event"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	run, err := h.repo.GetRunByID(context.Background(), "run-cron")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Reason != shared.RunReasonRoutineDispatchEvent {
		t.Fatalf("run.Reason = %q, want promoted to %q", run.Reason, shared.RunReasonRoutineDispatchEvent)
	}
	if run.CausationID != "cause-cron-original" {
		t.Fatalf("run.CausationID = %q, want unchanged cause-cron-original (promotion is not a new cause)", run.CausationID)
	}
}

// AC-002.3/AC-002.4: the lost-CAS fresh-run path (event coalescing into
// an already-claimed cron run) creates its own run, and that run
// carries the *requesting* wake's causation id, not the claimed run's.
func TestDispatch_LostCASFreshRunCarriesRequestingWakeCausationID(t *testing.T) {
	h := newHarness(t, wakeup.PolicyCoalesceIfActive)
	h.seedRunWithReasonAndCausation("run-cron", "claimed", shared.RunReasonRoutineDispatchCron, "cause-claimed-cron")
	h.seedWakeupWithCausation("w-event", wakeup.SourceRoutine, `{"routine_id":"r-1"}`, "cause-event-fresh")
	if _, err := h.repo.ExecRaw(context.Background(),
		`UPDATE agent_wakeup_requests SET reason = ? WHERE id = ?`,
		shared.RunReasonRoutineDispatchEvent, "w-event"); err != nil {
		t.Fatalf("seed reason: %v", err)
	}

	if err := h.dispatcher.Dispatch(context.Background(), "w-event"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	gotReq, err := h.repo.GetWakeupRequest(context.Background(), "w-event")
	if err != nil {
		t.Fatalf("get wakeup request: %v", err)
	}
	if gotReq.RunID == "" || gotReq.RunID == "run-cron" {
		t.Fatalf("expected a fresh run distinct from the claimed one, got %q", gotReq.RunID)
	}
	freshRun, err := h.repo.GetRunByID(context.Background(), gotReq.RunID)
	if err != nil {
		t.Fatalf("get fresh run: %v", err)
	}
	if freshRun.CausationID != "cause-event-fresh" {
		t.Fatalf("fresh run.CausationID = %q, want cause-event-fresh (the requesting wake's id)", freshRun.CausationID)
	}

	claimedRun, err := h.repo.GetRunByID(context.Background(), "run-cron")
	if err != nil {
		t.Fatalf("get claimed run: %v", err)
	}
	if claimedRun.CausationID != "cause-claimed-cron" {
		t.Fatalf("claimed run.CausationID = %q, want unchanged cause-claimed-cron", claimedRun.CausationID)
	}
}

// fakeRoutineLookupSkip resolves every routine to skip_if_active so the
// skip transition can be driven deterministically without a real
// office_routines row.
type fakeRoutineLookupSkip struct{}

func (fakeRoutineLookupSkip) GetRoutine(_ context.Context, id string) (*officemodels.Routine, error) {
	return &officemodels.Routine{ID: id, ConcurrencyPolicy: officemodels.RoutineConcurrencyPolicy(wakeup.PolicySkipIfActive)}, nil
}
