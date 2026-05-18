package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func strPtr(s string) *string { return &s }

func TestClaimNextEligibleRun_Basic(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != req.ID {
		t.Errorf("claimed ID = %q, want %q", claimed.ID, req.ID)
	}
	if claimed.Status != "claimed" {
		t.Errorf("status = %q, want claimed", claimed.Status)
	}
}

func TestClaimNextEligibleRun_SkipsBusyAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create a claimed run (agent at capacity).
	claimed := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "claimed",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, claimed); err != nil {
		t.Fatalf("create claimed: %v", err)
	}

	// Create a queued run for same agent.
	queued := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, queued); err != nil {
		t.Fatalf("create queued: %v", err)
	}

	// Claim should return nothing since agent is at capacity.
	_, err := repo.ClaimNextEligibleRun(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for busy agent, got %v", err)
	}
}

func TestClaimNextEligibleRun_PicksNextEligible(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// a1 is at capacity (has a claimed run).
	atCap := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "claimed",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, atCap); err != nil {
		t.Fatalf("create: %v", err)
	}

	// a1 has a queued run.
	a1Queued := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, a1Queued); err != nil {
		t.Fatalf("create: %v", err)
	}

	// a2 has a queued run.
	a2Queued := &models.Run{
		AgentProfileID: "a2",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, a2Queued); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Should claim a2's run since a1 is busy.
	result, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if result.AgentProfileID != "a2" {
		t.Errorf("claimed agent = %q, want a2", result.AgentProfileID)
	}
}

func TestClaimNextEligibleRun_ClaimsPausedAgent(t *testing.T) {
	// Agent status filtering is now done at the service layer.
	// The DB-level claim only checks capacity (no active claims).
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "paused-agent-1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.AgentProfileID != "paused-agent-1" {
		t.Errorf("agent = %q, want paused-agent-1", claimed.AgentProfileID)
	}
}

func TestCoalesceRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Create agent instance so the workspace-filtered list query can find runs.
	agent := &models.AgentInstance{
		ID:          "a1",
		WorkspaceID: "ws1",
		Name:        "coalesce-agent",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	req := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        `{"task_id":"t1"}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	coalesced, err := repo.CoalesceRun(ctx, "a1", "task_comment", 10, `{"task_id":"t2"}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if !coalesced {
		t.Error("expected coalesce to succeed")
	}

	reqs, _ := repo.ListRuns(ctx, "ws1")
	if len(reqs) != 1 {
		t.Fatalf("want 1, got %d", len(reqs))
	}
	if reqs[0].CoalescedCount != 2 {
		t.Errorf("coalesced_count = %d, want 2", reqs[0].CoalescedCount)
	}
}

func TestCheckIdempotencyKey(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
		IdempotencyKey: strPtr("unique-key-1"),
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	dup, err := repo.CheckIdempotencyKey(ctx, "unique-key-1", 24)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !dup {
		t.Error("expected duplicate=true for existing key")
	}

	dup, err = repo.CheckIdempotencyKey(ctx, "nonexistent-key", 24)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if dup {
		t.Error("expected duplicate=false for nonexistent key")
	}
}

func TestCleanExpired(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Finish it.
	if err := repo.FinishRun(ctx, req.ID, "finished"); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Clean with a future cutoff should remove it.
	n, err := repo.CleanExpired(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if n != 1 {
		t.Errorf("cleaned %d, want 1", n)
	}

	reqs, _ := repo.ListRuns(ctx, "ws1")
	if len(reqs) != 0 {
		t.Errorf("want 0 after clean, got %d", len(reqs))
	}
}

func TestClaimNextEligibleRun_ClaimsWithoutCooldownCheck(t *testing.T) {
	// Cooldown enforcement is now done at the service layer, not the DB query.
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "cooldown-agent-1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.AgentProfileID != "cooldown-agent-1" {
		t.Errorf("agent = %q, want cooldown-agent-1", claimed.AgentProfileID)
	}
}

func TestClaimNextEligibleRun_AllowsAgentPastCooldown(t *testing.T) {
	// Cooldown enforcement is done at the service layer, not the DB query.
	// The DB claim only checks capacity. This test verifies the run is
	// claimed regardless of runtime state stored in office_agent_runtime.
	repo := newTestRepo(t)
	ctx := context.Background()

	// Store runtime state with a past run timestamp.
	past := time.Now().UTC().Add(-10 * time.Second)
	if err := repo.UpsertAgentRuntime(ctx, "a-cd2", "idle", ""); err != nil {
		t.Fatalf("upsert runtime: %v", err)
	}
	if err := repo.UpdateRuntimeLastRunFinished(ctx, "a-cd2", past); err != nil {
		t.Fatalf("update finished: %v", err)
	}

	req := &models.Run{
		AgentProfileID: "a-cd2",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.AgentProfileID != "a-cd2" {
		t.Errorf("claimed agent = %q, want a-cd2", claimed.AgentProfileID)
	}
}

func TestClaimNextEligibleRun_SkipsScheduledRetryInFuture(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	future := time.Now().UTC().Add(10 * time.Minute)
	req := &models.Run{
		AgentProfileID:   "a-retry",
		Reason:           "task_assigned",
		Payload:          "{}",
		Status:           "queued",
		CoalescedCount:   1,
		RetryCount:       1,
		ScheduledRetryAt: &future,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := repo.ClaimNextEligibleRun(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows for future retry, got %v", err)
	}
}

func TestScheduleRetry_ResetsToQueued(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "a-sr",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Claim it.
	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Schedule retry.
	retryAt := time.Now().UTC().Add(2 * time.Minute)
	if err := repo.ScheduleRetry(ctx, claimed.ID, retryAt, 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	// Should not be claimable yet (retry in future).
	_, err = repo.ClaimNextEligibleRun(ctx)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows (retry in future), got %v", err)
	}
}

func TestScheduleRetry_PreservesRunPayloadForResume(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	payload := `{"task_id":"task-1","session_id":"session-1"}`
	req := &models.Run{
		AgentProfileID: "a-resume",
		Reason:         "task_assigned",
		Payload:        payload,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}
	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	retryAt := time.Now().UTC().Add(time.Minute)
	if err := repo.ScheduleRetry(ctx, claimed.ID, retryAt, 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}

	requeued, err := repo.GetRun(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if requeued.ID != claimed.ID {
		t.Fatalf("run ID = %q, want %q", requeued.ID, claimed.ID)
	}
	if requeued.Payload != payload {
		t.Fatalf("payload = %q, want %q", requeued.Payload, payload)
	}
	if requeued.Status != "queued" {
		t.Fatalf("status = %q, want queued", requeued.Status)
	}
	if requeued.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", requeued.RetryCount)
	}
}

func TestRecoverStale(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_assigned",
		Payload:        "{}",
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Claim it.
	_, err := repo.ClaimRun(ctx, "a1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Recover with a future cutoff should reset it.
	n, err := repo.RecoverStale(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if n != 1 {
		t.Errorf("recovered %d, want 1", n)
	}
}

// TestGetRunsByCommentIDs_MapsLatestRunPerComment pins that the
// per-comment lookup returns one row per comment, picks the most
// recently requested row when duplicates exist, and surfaces the
// error_message field for failed runs so the dashboard can render the
// red badge tooltip without a second query.
func TestGetRunsByCommentIDs_MapsLatestRunPerComment(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Comment cm-A: a single finished run.
	runA := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        `{"task_id":"t1","comment_id":"cm-A"}`,
		Status:         "finished",
		CoalescedCount: 1,
		IdempotencyKey: strPtr("task_comment:cm-A"),
	}
	if err := repo.CreateRun(ctx, runA); err != nil {
		t.Fatalf("create runA: %v", err)
	}

	// Comment cm-B: a failed run with an error message. CreateRun
	// doesn't write error_message (the column is populated by the
	// failure-handler path); use the test-only setter to land the
	// fixture value.
	runB := &models.Run{
		AgentProfileID: "a1",
		Reason:         "task_comment",
		Payload:        `{"task_id":"t1","comment_id":"cm-B"}`,
		Status:         "failed",
		CoalescedCount: 1,
		IdempotencyKey: strPtr("task_comment:cm-B"),
	}
	if err := repo.CreateRun(ctx, runB); err != nil {
		t.Fatalf("create runB: %v", err)
	}
	if err := repo.SetRunErrorMessageForTest(ctx, runB.ID, "boom"); err != nil {
		t.Fatalf("set error message: %v", err)
	}

	// Comment cm-C is queried but has no run row.
	got, err := repo.GetRunsByCommentIDs(ctx, []string{"cm-A", "cm-B", "cm-C"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (cm-C should be absent): %+v", len(got), got)
	}
	if got["cm-A"].RunID != runA.ID || got["cm-A"].Status != "finished" {
		t.Errorf("cm-A = %+v, want runA finished", got["cm-A"])
	}
	if got["cm-B"].RunID != runB.ID || got["cm-B"].Status != "failed" || got["cm-B"].ErrorMessage != "boom" {
		t.Errorf("cm-B = %+v, want runB failed with error", got["cm-B"])
	}
	if _, present := got["cm-C"]; present {
		t.Errorf("cm-C should be absent from map: %+v", got["cm-C"])
	}
}

// TestGetRunsByCommentIDs_EmptyInput_ReturnsEmptyMap pins the
// degenerate case so callers can hand the result straight into a
// range loop without nil-checking.
func TestGetRunsByCommentIDs_EmptyInput_ReturnsEmptyMap(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.GetRunsByCommentIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %+v", got)
	}
}
