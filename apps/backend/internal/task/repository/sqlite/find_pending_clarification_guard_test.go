package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestFindPendingClarificationMessagesBySessionID_G5_AbsentOrUnrecognizedStatusCountsAsPending
// proves the guard's status predicate widened to D3's effective-pending form
// (spec G5): a message with no metadata.status key, and one with a status
// outside the four terminal clarification.Status values, both still count as
// pending. A message carrying one of the four terminal statuses is excluded,
// same as before.
func TestFindPendingClarificationMessagesBySessionID_G5_AbsentOrUnrecognizedStatusCountsAsPending(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-g5", "session-g5", "turn-g5")

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	messages := []*models.Message{
		{ID: "g5-absent", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base, Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-absent-pending"}},
		{ID: "g5-unrecognized", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base.Add(time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-unrecognized-pending", "status": "bogus"}},
		{ID: "g5-pending", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base.Add(2 * time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-pending-pending", "status": "pending"}},
		{ID: "g5-answered", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base.Add(3 * time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-answered-pending", "status": "answered"}},
		{ID: "g5-rejected", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base.Add(4 * time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-rejected-pending", "status": "rejected"}},
		{ID: "g5-cancelled", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base.Add(5 * time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-cancelled-pending", "status": "cancelled"}},
		{ID: "g5-expired", TaskSessionID: "session-g5", TaskID: "task-g5", TurnID: "turn-g5", Content: "q", CreatedAt: base.Add(6 * time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g5-expired-pending", "status": "expired"}},
	}
	for _, m := range messages {
		if err := repo.CreateMessage(ctx, m); err != nil {
			t.Fatalf("CreateMessage(%s): %v", m.ID, err)
		}
	}

	got, err := repo.FindPendingClarificationMessagesBySessionID(ctx, "session-g5")
	if err != nil {
		t.Fatalf("FindPendingClarificationMessagesBySessionID: %v", err)
	}
	want := "g5-absent,g5-unrecognized,g5-pending"
	if got := strings.Join(messageIDs(got), ","); got != want {
		t.Fatalf("FindPendingClarificationMessagesBySessionID IDs = %q, want %q", got, want)
	}
}

// TestFindPendingClarificationMessagesBySessionID_G1_ResolvedBundleExcludedRegardlessOfMessageStatus
// proves the guard gained the D4a conjunct-1 exclusion (spec G1/G4): a
// clarification_request message whose pending_id has a clarification_resolutions
// row is never returned, even though its own message metadata still reads
// "pending" (the R5 partial-application / claim-then-crash state spec G2
// describes). Without this exclusion the guard would wedge such a session's
// turn-complete transition forever.
func TestFindPendingClarificationMessagesBySessionID_G1_ResolvedBundleExcludedRegardlessOfMessageStatus(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-g1", "session-g1", "turn-g1")

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	messages := []*models.Message{
		{ID: "g1-resolved-but-stale", TaskSessionID: "session-g1", TaskID: "task-g1", TurnID: "turn-g1", Content: "q", CreatedAt: base, Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g1-resolved-pending", "status": "pending"}},
		{ID: "g1-still-pending", TaskSessionID: "session-g1", TaskID: "task-g1", TurnID: "turn-g1", Content: "q", CreatedAt: base.Add(time.Second), Type: models.MessageTypeClarificationRequest, Metadata: map[string]any{"pending_id": "g1-unresolved-pending", "status": "pending"}},
	}
	for _, m := range messages {
		if err := repo.CreateMessage(ctx, m); err != nil {
			t.Fatalf("CreateMessage(%s): %v", m.ID, err)
		}
	}

	claimed, _, err := repo.InsertClarificationResolution(ctx, &models.ClarificationResolution{
		PendingID:  "g1-resolved-pending",
		SessionID:  "session-g1",
		TaskID:     "task-g1",
		Status:     models.ClarificationResolutionStatusAnswered,
		Response:   `{"pending_id":"g1-resolved-pending","answers":[],"rejected":false,"reject_reason":""}`,
		Resume:     "resumed",
		ResolvedBy: "user-1",
		Source:     models.ClarificationResolutionSourceWeb,
		ResolvedAt: base.Add(10 * time.Second),
	})
	if err != nil || !claimed {
		t.Fatalf("InsertClarificationResolution: claimed=%v err=%v", claimed, err)
	}

	got, err := repo.FindPendingClarificationMessagesBySessionID(ctx, "session-g1")
	if err != nil {
		t.Fatalf("FindPendingClarificationMessagesBySessionID: %v", err)
	}
	want := "g1-still-pending"
	if got := strings.Join(messageIDs(got), ","); got != want {
		t.Fatalf("FindPendingClarificationMessagesBySessionID IDs = %q, want %q (resolved bundle must be excluded despite stale pending status)", got, want)
	}
}
