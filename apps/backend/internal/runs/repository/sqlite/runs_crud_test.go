package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// fixedRetryAt is a whole-second timestamp so the round-trip assertion
// compares an exact value rather than tolerating driver precision.
var fixedRetryAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// fullRun is a run with every column CreateRun writes set to a value
// that differs from the schema default, so dropping any one column
// from the INSERT changes what comes back out.
func fullRun() *models.Run {
	return &models.Run{
		ID:               "run-full",
		AgentProfileID:   "agent-42",
		Reason:           "task_assigned",
		Payload:          `{"task_id":"t-9"}`,
		Status:           "claimed",
		CoalescedCount:   7,
		IdempotencyKey:   strPtr("idem-key-1"),
		ContextSnapshot:  `{"ctx":1}`,
		Capabilities:     `{"cap":true}`,
		InputSnapshot:    `{"in":"x"}`,
		OutputSummary:    "output summary",
		FailureReason:    "failure reason",
		SessionID:        "sess-7",
		RetryCount:       3,
		ScheduledRetryAt: timePtr(fixedRetryAt),
		ErrorMessage:     "boom",
		CancelReason:     strPtr("user cancelled"),
	}
}

// TestCreateRun_PersistsEveryWrittenColumn is the round-trip guard for
// the INSERT: write a row with every field populated, read it back,
// compare all fields.
func TestCreateRun_PersistsEveryWrittenColumn(t *testing.T) {
	repo := newTestRepo(t)
	want := fullRun()
	mustCreateRun(t, repo, want)

	got := mustGetRun(t, repo, want.ID)

	checkString(t, "id", got.ID, want.ID)
	checkString(t, "agent_profile_id", got.AgentProfileID, want.AgentProfileID)
	checkString(t, "reason", got.Reason, want.Reason)
	checkString(t, "payload", got.Payload, want.Payload)
	checkString(t, "status", string(got.Status), string(want.Status))
	checkString(t, "context_snapshot", got.ContextSnapshot, want.ContextSnapshot)
	checkString(t, "capabilities", got.Capabilities, want.Capabilities)
	checkString(t, "input_snapshot", got.InputSnapshot, want.InputSnapshot)
	checkString(t, "output_summary", got.OutputSummary, want.OutputSummary)
	checkString(t, "failure_reason", got.FailureReason, want.FailureReason)
	checkString(t, "session_id", got.SessionID, want.SessionID)
	checkString(t, "error_message", got.ErrorMessage, want.ErrorMessage)
	checkString(t, "continuation_scope", got.ContinuationScope, "agent:agent-42")
	checkInt(t, "coalesced_count", got.CoalescedCount, want.CoalescedCount)
	checkInt(t, "retry_count", got.RetryCount, want.RetryCount)
	checkStringPtr(t, "idempotency_key", got.IdempotencyKey, want.IdempotencyKey)
	checkStringPtr(t, "cancel_reason", got.CancelReason, want.CancelReason)

	if got.ScheduledRetryAt == nil {
		t.Errorf("scheduled_retry_at = nil, want %s", fixedRetryAt)
	} else if !got.ScheduledRetryAt.Equal(fixedRetryAt) {
		t.Errorf("scheduled_retry_at = %s, want %s", got.ScheduledRetryAt, fixedRetryAt)
	}
}

func TestCreateRun_PersistsRoutineContinuationScope(t *testing.T) {
	repo := newTestRepo(t)
	want := &models.Run{
		AgentProfileID:  "agent-routine",
		Reason:          "routine_trigger",
		ContextSnapshot: `{"routine_id":"routine-1"}`,
	}
	mustCreateRun(t, repo, want)

	got := mustGetRun(t, repo, want.ID)
	checkString(t, "continuation_scope", got.ContinuationScope, "routine:routine-1")
}

// TestCreateRun_LeavesUnwrittenColumnsAtTheirDefaults pins the other
// half of the INSERT contract: the columns CreateRun deliberately does
// not write (routing, prompt artifacts, timing) must land on their
// schema defaults rather than NULL-scanning into a broken struct.
func TestCreateRun_LeavesUnwrittenColumnsAtTheirDefaults(t *testing.T) {
	repo := newTestRepo(t)
	run := mustCreateRun(t, repo, fullRun())

	got := mustGetRun(t, repo, run.ID)

	checkString(t, "result_json", got.ResultJSON, "{}")
	checkString(t, "assembled_prompt", got.AssembledPrompt, "")
	checkString(t, "summary_injected", got.SummaryInjected, "")
	checkInt(t, "current_route_attempt_seq", got.CurrentRouteAttemptSeq, 0)
	checkInt(t, "route_cycle_baseline_seq", got.RouteCycleBaselineSeq, 0)
	if got.ClaimedAt != nil {
		t.Errorf("claimed_at = %s, want nil", got.ClaimedAt)
	}
	if got.FinishedAt != nil {
		t.Errorf("finished_at = %s, want nil", got.FinishedAt)
	}
	if got.RoutingBlockedStatus != nil {
		t.Errorf("routing_blocked_status = %v, want nil", *got.RoutingBlockedStatus)
	}
	if got.ResolvedProviderID != nil {
		t.Errorf("resolved_provider_id = %v, want nil", *got.ResolvedProviderID)
	}
	if got.EarliestRetryAt != nil {
		t.Errorf("earliest_retry_at = %s, want nil", got.EarliestRetryAt)
	}
}

// TestCreateRun_AppliesDefaultsAndStampsIdentity covers ensureRunDefaults
// plus the two fields CreateRun owns outright: a generated ID and a
// server-side requested_at that overwrites whatever the caller passed.
func TestCreateRun_AppliesDefaultsAndStampsIdentity(t *testing.T) {
	repo := newTestRepo(t)
	stale := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Now().UTC().Add(-time.Second)

	run := &models.Run{
		AgentProfileID: "a1",
		Reason:         "heartbeat",
		RequestedAt:    stale,
	}
	mustCreateRun(t, repo, run)

	if run.ID == "" {
		t.Fatalf("CreateRun left ID empty; it must generate one")
	}
	if !run.RequestedAt.After(before) {
		t.Errorf("requested_at = %s, want a fresh stamp after %s (caller value must be ignored)",
			run.RequestedAt, before)
	}

	got := mustGetRun(t, repo, run.ID)
	checkString(t, "payload", got.Payload, "{}")
	checkString(t, "context_snapshot", got.ContextSnapshot, "{}")
	checkString(t, "capabilities", got.Capabilities, "{}")
	checkString(t, "input_snapshot", got.InputSnapshot, "{}")
	checkString(t, "status", string(got.Status), "queued")
	checkString(t, "result_json", got.ResultJSON, "{}")
	if got.RequestedAt.Equal(stale) {
		t.Errorf("stored requested_at = %s, want the CreateRun stamp, not the caller value", got.RequestedAt)
	}
	if !got.RequestedAt.After(before) {
		t.Errorf("stored requested_at = %s, want after %s", got.RequestedAt, before)
	}
}

// TestCreateRun_KeepsCallerSuppliedID pins that a caller-chosen ID is
// preserved (the idempotency and coalescing callers depend on it).
func TestCreateRun_KeepsCallerSuppliedID(t *testing.T) {
	repo := newTestRepo(t)
	run := mustCreateRun(t, repo, &models.Run{
		ID:             "chosen-id",
		AgentProfileID: "a1",
		Reason:         "task_assigned",
	})
	if run.ID != "chosen-id" {
		t.Fatalf("ID = %q, want chosen-id", run.ID)
	}
	if got := mustGetRun(t, repo, "chosen-id"); got.ID != "chosen-id" {
		t.Errorf("stored ID = %q, want chosen-id", got.ID)
	}
}

// TestGetRunByID_UnknownRunReturnsNoRows pins the sentinel every caller
// switches on.
func TestGetRunByID_UnknownRunReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.GetRunByID(context.Background(), "nope")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestUpdateRunRuntimeSnapshot_WritesAllThreeColumns covers the
// pre-launch snapshot write and asserts it touches nothing else.
func TestUpdateRunRuntimeSnapshot_WritesAllThreeColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, fullRun())
	other := mustCreateRun(t, repo, &models.Run{
		ID: "other", AgentProfileID: "a2", Reason: "heartbeat",
		Capabilities: `{"untouched":true}`,
	})

	if err := repo.UpdateRunRuntimeSnapshot(ctx, run.ID,
		`{"cap":"new"}`, `{"in":"new"}`, "sess-new"); err != nil {
		t.Fatalf("update runtime snapshot: %v", err)
	}

	got := mustGetRun(t, repo, run.ID)
	checkString(t, "capabilities", got.Capabilities, `{"cap":"new"}`)
	checkString(t, "input_snapshot", got.InputSnapshot, `{"in":"new"}`)
	checkString(t, "session_id", got.SessionID, "sess-new")
	checkString(t, "reason", got.Reason, "task_assigned")

	untouched := mustGetRun(t, repo, other.ID)
	checkString(t, "other capabilities", untouched.Capabilities, `{"untouched":true}`)
	checkString(t, "other session_id", untouched.SessionID, "")
}

// TestUpdateRunPromptArtifacts_RoundTripsBothColumns covers the
// dispatch-time prompt snapshot, including the documented empty case.
func TestUpdateRunPromptArtifacts_RoundTripsBothColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, fullRun())

	if err := repo.UpdateRunPromptArtifacts(ctx, run.ID, "the prompt", "the summary"); err != nil {
		t.Fatalf("update prompt artifacts: %v", err)
	}
	got := mustGetRun(t, repo, run.ID)
	checkString(t, "assembled_prompt", got.AssembledPrompt, "the prompt")
	checkString(t, "summary_injected", got.SummaryInjected, "the summary")

	if err := repo.UpdateRunPromptArtifacts(ctx, run.ID, "", ""); err != nil {
		t.Fatalf("update prompt artifacts (empty): %v", err)
	}
	got = mustGetRun(t, repo, run.ID)
	checkString(t, "assembled_prompt after empty write", got.AssembledPrompt, "")
	checkString(t, "summary_injected after empty write", got.SummaryInjected, "")
}

// TestUpdateRunResultJSON_EmptyFallsBackToEmptyObject pins the column
// default invariant the continuation-summary builder relies on.
func TestUpdateRunResultJSON_EmptyFallsBackToEmptyObject(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, fullRun())

	if err := repo.UpdateRunResultJSON(ctx, run.ID, `{"actions":["a"]}`); err != nil {
		t.Fatalf("update result json: %v", err)
	}
	checkString(t, "result_json", mustGetRun(t, repo, run.ID).ResultJSON, `{"actions":["a"]}`)

	if err := repo.UpdateRunResultJSON(ctx, run.ID, ""); err != nil {
		t.Fatalf("update result json (empty): %v", err)
	}
	checkString(t, "result_json after empty write", mustGetRun(t, repo, run.ID).ResultJSON, "{}")
}

// TestUpdateRunOutputSummary_WritesSummaryAndFailureReason covers the
// completion write.
func TestUpdateRunOutputSummary_WritesSummaryAndFailureReason(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, fullRun())

	if err := repo.UpdateRunOutputSummary(ctx, run.ID, "did the thing", "none"); err != nil {
		t.Fatalf("update output summary: %v", err)
	}
	got := mustGetRun(t, repo, run.ID)
	checkString(t, "output_summary", got.OutputSummary, "did the thing")
	checkString(t, "failure_reason", got.FailureReason, "none")
	checkString(t, "status", string(got.Status), "claimed")
}

// TestFinishRun_SetsStatusAndFinishedAt pins both columns the terminal
// write owns.
func TestFinishRun_SetsStatusAndFinishedAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	before := time.Now().UTC().Add(-time.Second)
	run := mustCreateRun(t, repo, fullRun())

	if err := repo.FinishRun(ctx, run.ID, "finished"); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	got := mustGetRun(t, repo, run.ID)
	checkString(t, "status", string(got.Status), "finished")
	if got.FinishedAt == nil {
		t.Fatalf("finished_at = nil, want a stamp")
	}
	if !got.FinishedAt.After(before) {
		t.Errorf("finished_at = %s, want after %s", got.FinishedAt, before)
	}
}

// TestSetRunStatusForTest_WritesStatusAndBothTimestamps covers the
// harness affordance used by the E2E seeder, including its NULL case.
func TestSetRunStatusForTest_WritesStatusAndBothTimestamps(t *testing.T) {
	repo := newTestRepo(t)
	run := mustCreateRun(t, repo, fullRun())
	claimed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finished := claimed.Add(time.Hour)

	setStatus(t, repo, run.ID, "failed", timePtr(claimed), timePtr(finished))
	got := mustGetRun(t, repo, run.ID)
	checkString(t, "status", string(got.Status), "failed")
	if got.ClaimedAt == nil || !got.ClaimedAt.Equal(claimed) {
		t.Errorf("claimed_at = %v, want %s", got.ClaimedAt, claimed)
	}
	if got.FinishedAt == nil || !got.FinishedAt.Equal(finished) {
		t.Errorf("finished_at = %v, want %s", got.FinishedAt, finished)
	}

	setStatus(t, repo, run.ID, "queued", nil, nil)
	got = mustGetRun(t, repo, run.ID)
	checkString(t, "status", string(got.Status), "queued")
	if got.ClaimedAt != nil || got.FinishedAt != nil {
		t.Errorf("timestamps = (%v, %v), want both nil", got.ClaimedAt, got.FinishedAt)
	}
}

// TestSetRunRequestedAtAndErrorMessageForTest covers the two remaining
// test affordances; the E2E ordering fixtures depend on both landing.
func TestSetRunRequestedAtAndErrorMessageForTest(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, fullRun())
	backdated := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)

	setRequestedAt(t, repo, run.ID, backdated)
	if err := repo.SetRunErrorMessageForTest(ctx, run.ID, "agent exploded"); err != nil {
		t.Fatalf("set error message: %v", err)
	}

	got := mustGetRun(t, repo, run.ID)
	if !got.RequestedAt.Equal(backdated) {
		t.Errorf("requested_at = %s, want %s", got.RequestedAt, backdated)
	}
	checkString(t, "error_message", got.ErrorMessage, "agent exploded")
}

func checkString(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", field, got, want)
	}
}

func checkInt(t *testing.T, field string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", field, got, want)
	}
}

func checkStringPtr(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s = nil, want %q", field, *want)
	case want == nil:
		t.Errorf("%s = %q, want nil", field, *got)
	case *got != *want:
		t.Errorf("%s = %q, want %q", field, *got, *want)
	}
}
