package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresFindPendingClarificationMessagesBySessionID_G5_NullHandling
// proves spec G5's widened status predicate behaves identically on
// PostgreSQL: a message with an absent metadata.status key (jsonb ->> yields
// SQL NULL, not SQLite's json_extract NULL) and one with an unrecognized
// value both still count as pending, while a message with a recognized
// terminal status does not. G5 explicitly calls out NULL handling in a JSON
// extraction as the most dialect-sensitive expression this spec adds, so a
// SQLite-only run does not exercise this.
func TestPostgresFindPendingClarificationMessagesBySessionID_G5_NullHandling(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-guard-g5", "")
	seedBundleSession(t, repo, "sess-pg-guard-g5", "task-pg-guard-g5")
	seedBundleTurn(t, repo, "turn-pg-guard-g5", "sess-pg-guard-g5", "task-pg-guard-g5")

	base := time.Now().UTC().Truncate(time.Millisecond)
	insertClarificationMessage(t, repo, "msg-pg-g5-absent", "sess-pg-guard-g5", "task-pg-guard-g5", "turn-pg-guard-g5", "pending-pg-g5-absent", "q1", "", 0, base)
	insertClarificationMessage(t, repo, "msg-pg-g5-bogus", "sess-pg-guard-g5", "task-pg-guard-g5", "turn-pg-guard-g5", "pending-pg-g5-bogus", "q1", "bogus", 0, base.Add(time.Second))
	insertClarificationMessage(t, repo, "msg-pg-g5-answered", "sess-pg-guard-g5", "task-pg-guard-g5", "turn-pg-guard-g5", "pending-pg-g5-answered", "q1", "answered", 0, base.Add(2*time.Second))

	got, err := repo.FindPendingClarificationMessagesBySessionID(ctx, "sess-pg-guard-g5")
	if err != nil {
		t.Fatalf("FindPendingClarificationMessagesBySessionID: %v", err)
	}
	want := "msg-pg-g5-absent,msg-pg-g5-bogus"
	if got := strings.Join(messageIDs(got), ","); got != want {
		t.Fatalf("FindPendingClarificationMessagesBySessionID IDs = %q, want %q", got, want)
	}
}

// TestPostgresFindPendingClarificationMessagesBySessionID_G1_ResolutionExclusion
// proves spec G1/G4's conjunct-1 exclusion joins clarification_resolutions
// correctly on PostgreSQL: a message whose pending_id has a resolution row is
// excluded even though its own metadata.status still reads "pending".
func TestPostgresFindPendingClarificationMessagesBySessionID_G1_ResolutionExclusion(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	seedBundleTask(t, repo, "task-pg-guard-g1", "")
	seedBundleSession(t, repo, "sess-pg-guard-g1", "task-pg-guard-g1")
	seedBundleTurn(t, repo, "turn-pg-guard-g1", "sess-pg-guard-g1", "task-pg-guard-g1")

	base := time.Now().UTC().Truncate(time.Millisecond)
	insertClarificationMessage(t, repo, "msg-pg-g1-resolved", "sess-pg-guard-g1", "task-pg-guard-g1", "turn-pg-guard-g1", "pending-pg-g1-resolved", "q1", "pending", 0, base)
	insertClarificationMessage(t, repo, "msg-pg-g1-open", "sess-pg-guard-g1", "task-pg-guard-g1", "turn-pg-guard-g1", "pending-pg-g1-open", "q1", "pending", 0, base.Add(time.Second))

	claimed, _, err := repo.InsertClarificationResolution(ctx, &models.ClarificationResolution{
		PendingID:  "pending-pg-g1-resolved",
		SessionID:  "sess-pg-guard-g1",
		TaskID:     "task-pg-guard-g1",
		Status:     models.ClarificationResolutionStatusAnswered,
		Response:   `{"pending_id":"pending-pg-g1-resolved","answers":[],"rejected":false,"reject_reason":""}`,
		Resume:     "resumed",
		ResolvedBy: "user-1",
		Source:     models.ClarificationResolutionSourceWeb,
		ResolvedAt: base.Add(10 * time.Second),
	})
	if err != nil || !claimed {
		t.Fatalf("InsertClarificationResolution: claimed=%v err=%v", claimed, err)
	}

	got, err := repo.FindPendingClarificationMessagesBySessionID(ctx, "sess-pg-guard-g1")
	if err != nil {
		t.Fatalf("FindPendingClarificationMessagesBySessionID: %v", err)
	}
	want := "msg-pg-g1-open"
	if got := strings.Join(messageIDs(got), ","); got != want {
		t.Fatalf("FindPendingClarificationMessagesBySessionID IDs = %q, want %q (resolved bundle must be excluded)", got, want)
	}
}
