package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresPendingInteractionsMatchSQLiteAuthority pins the pending-interaction
// read on PostgreSQL. The query is dialect-sensitive in three places — the JSON
// status extraction, the rowid-versus-id tiebreak that picks a turn's newest
// permission, and the WITH clause nested inside the kind-filter subquery — and a
// schema replay test would exercise none of them. It skips unless
// KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresPendingInteractionsMatchSQLiteAuthority(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 22, 9, 0, 0, 0, time.UTC)

	// An approved permission on a session still reading WAITING_FOR_INPUT must
	// not be reported, exactly as on SQLite.
	seedPendingActionSession(t, repo, "task-approved-pg", "session-approved-pg")
	createPendingActionTurn(t, repo, "task-approved-pg", "session-approved-pg", "turn-approved-pg", base, base)
	createInteractionMessage(t, repo, "perm-approved-pg", "task-approved-pg", "session-approved-pg", "turn-approved-pg",
		models.MessageTypePermissionRequest, map[string]interface{}{
			"pending_id": "pending-approved-pg",
			"status":     string(models.InteractionStatusApproved),
		}, base)

	// Two permissions in one turn: only the newest is answerable. Their
	// created_at values differ deliberately — the secondary tiebreak is
	// dialect-specific (SQLite orders by rowid, PostgreSQL by the text id), so
	// identical timestamps would assert a difference between the two engines
	// rather than the rule under test.
	seedPendingActionSession(t, repo, "task-multi-pg", "session-multi-pg")
	createPendingActionTurn(t, repo, "task-multi-pg", "session-multi-pg", "turn-multi-pg", base, base)
	createInteractionMessage(t, repo, "perm-older-pg", "task-multi-pg", "session-multi-pg", "turn-multi-pg",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-older-pg"}, base)
	createInteractionMessage(t, repo, "perm-newer-pg", "task-multi-pg", "session-multi-pg", "turn-multi-pg",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-newer-pg"},
		base.Add(time.Second))

	seedPendingActionSession(t, repo, "task-clar-pg", "session-clar-pg")
	createPendingActionTurn(t, repo, "task-clar-pg", "session-clar-pg", "turn-clar-pg", base, base)
	createClarificationBundleMessage(t, repo, "clar-pg", "task-clar-pg", "session-clar-pg", "turn-clar-pg",
		"pending-clar-pg", "q1", base)

	got, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	ids := map[string]bool{}
	for _, message := range got {
		ids[message.ID] = true
	}
	if ids["perm-approved-pg"] {
		t.Fatal("approved permission reported as an owed interaction on postgres")
	}
	if ids["perm-older-pg"] {
		t.Fatal("superseded permission reported as an owed interaction on postgres")
	}
	if !ids["perm-newer-pg"] || !ids["clar-pg"] {
		t.Fatalf("pending interactions = %v, want the newest permission and the clarification",
			interactionMessageIDs(got))
	}

	// The kind filter wraps the whole CTE query in a subquery; prove PostgreSQL
	// accepts that nesting and narrows correctly.
	clarifications, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{
		Kinds: []string{string(models.InteractionKindClarification)},
	})
	if err != nil {
		t.Fatalf("ListPendingInteractions(kind): %v", err)
	}
	if len(clarifications) != 1 || clarifications[0].ID != "clar-pg" {
		t.Fatalf("kind-filtered interactions = %v, want [clar-pg]", interactionMessageIDs(clarifications))
	}

	// Both projections must reach the same verdict per session on postgres too.
	sessionIDs := []string{"session-approved-pg", "session-multi-pg", "session-clar-pg"}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, sessionIDs)
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	scoped, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{SessionIDs: sessionIDs})
	if err != nil {
		t.Fatalf("ListPendingInteractions(sessions): %v", err)
	}
	bySession := map[string]bool{}
	for _, message := range scoped {
		bySession[message.TaskSessionID] = true
	}
	for _, sessionID := range sessionIDs {
		_, projectionSaysPending := actions[sessionID]
		if projectionSaysPending != bySession[sessionID] {
			t.Fatalf("session %s: pending-action projection=%v, interaction list=%v (must agree)",
				sessionID, projectionSaysPending, bySession[sessionID])
		}
	}
}
