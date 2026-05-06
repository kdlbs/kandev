package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestWakeupRequest_CreateAndGet(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	req := &sqlite.WakeupRequest{
		ID:             "wakeup-1",
		AgentProfileID: "agent-1",
		Source:         "heartbeat",
		Reason:         "scheduled",
		Payload:        `{"missed_ticks":3}`,
	}
	if err := repo.CreateWakeupRequest(ctx, req); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetWakeupRequest(ctx, "wakeup-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AgentProfileID != "agent-1" {
		t.Errorf("agent_profile_id mismatch: got %q", got.AgentProfileID)
	}
	if got.Status != sqlite.WakeupStatusQueued {
		t.Errorf("status default: got %q want queued", got.Status)
	}
	if got.CoalescedCount != 1 {
		t.Errorf("coalesced_count default: got %d want 1", got.CoalescedCount)
	}
	if got.Payload != `{"missed_ticks":3}` {
		t.Errorf("payload roundtrip: got %q", got.Payload)
	}
}

func TestWakeupRequest_IdempotencyConflict(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	key := "heartbeat:agent-1:1234567890"
	first := &sqlite.WakeupRequest{
		ID:             "wakeup-a",
		AgentProfileID: "agent-1",
		Source:         "heartbeat",
		IdempotencyKey: sql.NullString{String: key, Valid: true},
	}
	if err := repo.CreateWakeupRequest(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	second := &sqlite.WakeupRequest{
		ID:             "wakeup-b",
		AgentProfileID: "agent-1",
		Source:         "heartbeat",
		IdempotencyKey: sql.NullString{String: key, Valid: true},
	}
	err := repo.CreateWakeupRequest(ctx, second)
	if !errors.Is(err, sqlite.ErrWakeupIdempotencyConflict) {
		t.Fatalf("expected ErrWakeupIdempotencyConflict, got %v", err)
	}
}

func TestWakeupRequest_ListQueuedForAgent_OnlyQueued(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	mk := func(id, status string, requestedAt time.Time) {
		_ = repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
			ID: id, AgentProfileID: "agent-1", Source: "heartbeat",
			Status: status, RequestedAt: requestedAt,
		})
	}
	now := time.Now().UTC()
	mk("a-queued", sqlite.WakeupStatusQueued, now.Add(-2*time.Minute))
	mk("a-claimed", sqlite.WakeupStatusClaimed, now.Add(-1*time.Minute))
	mk("a-skipped", sqlite.WakeupStatusSkipped, now)
	// other agent
	_ = repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
		ID: "b-queued", AgentProfileID: "agent-2", Source: "heartbeat",
	})

	rows, err := repo.ListQueuedWakeupRequestsForAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 queued for agent-1, got %d", len(rows))
	}
	if rows[0].ID != "a-queued" {
		t.Errorf("got id %q, want a-queued", rows[0].ID)
	}
}

func TestWakeupRequest_MarkClaimed(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_ = repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
		ID: "w-1", AgentProfileID: "agent-1", Source: "heartbeat",
	})
	if err := repo.MarkWakeupRequestClaimed(ctx, "w-1", "run-42"); err != nil {
		t.Fatalf("mark claimed: %v", err)
	}
	got, err := repo.GetWakeupRequest(ctx, "w-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != sqlite.WakeupStatusClaimed {
		t.Errorf("status: got %q", got.Status)
	}
	if got.RunID != "run-42" {
		t.Errorf("run_id: got %q", got.RunID)
	}
	if !got.ClaimedAt.Valid {
		t.Error("claimed_at should be set")
	}
}

func TestWakeupRequest_MarkSkipped(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_ = repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
		ID: "w-1", AgentProfileID: "agent-1", Source: "heartbeat", Reason: "original",
	})
	if err := repo.MarkWakeupRequestSkipped(ctx, "w-1", "policy:skip_if_active"); err != nil {
		t.Fatalf("mark skipped: %v", err)
	}
	got, _ := repo.GetWakeupRequest(ctx, "w-1")
	if got.Status != sqlite.WakeupStatusSkipped {
		t.Errorf("status: got %q", got.Status)
	}
	if got.Reason != "policy:skip_if_active" {
		t.Errorf("reason: got %q", got.Reason)
	}
}

// TestWakeupRequest_MarkCoalesced creates a runs row, then a wakeup
// request, then marks the request coalesced into the run. Verifies the
// status transition, coalesced_count bump, and that context_snapshot
// merge happens (top-level keys overwrite).
func TestWakeupRequest_MarkCoalesced(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Seed an in-flight run with an initial context snapshot.
	runID := uuid.New().String()
	run := &officemodels.Run{
		ID:              runID,
		AgentProfileID:  "agent-1",
		Reason:          "heartbeat",
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
		ContextSnapshot: `{"existing_key":"old"}`,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// Wakeup request with a payload that should overwrite "existing_key".
	_ = repo.CreateWakeupRequest(ctx, &sqlite.WakeupRequest{
		ID: "w-1", AgentProfileID: "agent-1", Source: "comment",
		Payload: `{"existing_key":"new","extra":"value"}`,
	})

	if err := repo.MarkWakeupRequestCoalesced(ctx, "w-1", runID); err != nil {
		t.Fatalf("mark coalesced: %v", err)
	}

	gotReq, _ := repo.GetWakeupRequest(ctx, "w-1")
	if gotReq.Status != sqlite.WakeupStatusCoalesced {
		t.Errorf("status: got %q", gotReq.Status)
	}
	if gotReq.RunID != runID {
		t.Errorf("run_id: got %q", gotReq.RunID)
	}

	gotRun, err := repo.GetRunByID(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.CoalescedCount != 2 {
		t.Errorf("coalesced_count: got %d want 2", gotRun.CoalescedCount)
	}
	// Top-level merge: existing_key now "new", extra=value, no other keys lost.
	if gotRun.ContextSnapshot == "" {
		t.Fatal("context_snapshot empty after coalesce")
	}
	// Order isn't guaranteed by json_patch — just check both keys are visible.
	if !contains(gotRun.ContextSnapshot, `"existing_key":"new"`) {
		t.Errorf("expected merged existing_key=new in snapshot, got %q", gotRun.ContextSnapshot)
	}
	if !contains(gotRun.ContextSnapshot, `"extra":"value"`) {
		t.Errorf("expected merged extra=value in snapshot, got %q", gotRun.ContextSnapshot)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && stringIndexOf(s, sub) >= 0))
}

func stringIndexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
