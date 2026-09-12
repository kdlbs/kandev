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

// TestPostgresListPendingInteractionsSkipsLifecycleOnlyTurn mirrors
// TestListPendingInteractionsSkipsLifecycleOnlyTurn on PostgreSQL. The
// current-turn CTE's predicate and ordering (currentTurnAuthority) are built
// from dialect.JSONExtract, which emits different SQL per driver
// (json_extract on SQLite, ::jsonb->> on Postgres), so the lifecycle_only
// exclusion needs its own coverage on this dialect rather than trusting the
// SQLite run. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListPendingInteractionsSkipsLifecycleOnlyTurn(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.September, 12, 9, 0, 0, 0, time.UTC)

	seedPendingActionSession(t, repo, "task-lifecycle-pg", "session-lifecycle-pg")
	createPendingActionTurn(t, repo, "task-lifecycle-pg", "session-lifecycle-pg", "turn-question-pg", base, base)
	createClarificationBundleMessage(t, repo, "clar-q1-pg", "task-lifecycle-pg", "session-lifecycle-pg", "turn-question-pg", "pending-lifecycle-pg", "q1", base)
	createClarificationBundleMessage(t, repo, "clar-q2-pg", "task-lifecycle-pg", "session-lifecycle-pg", "turn-question-pg", "pending-lifecycle-pg", "q2", base.Add(time.Second))

	lifecycleCompletedAt := base.Add(time.Minute)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-lifecycle-pg",
		TaskSessionID: "session-lifecycle-pg",
		TaskID:        "task-lifecycle-pg",
		StartedAt:     base.Add(time.Minute),
		CreatedAt:     base.Add(time.Minute),
		CompletedAt:   &lifecycleCompletedAt,
		Metadata:      map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	}); err != nil {
		t.Fatalf("CreateTurn(turn-lifecycle-pg): %v", err)
	}
	createPendingActionMessage(t, repo, "lifecycle-note-pg", "task-lifecycle-pg", "session-lifecycle-pg", "turn-lifecycle-pg",
		models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	got, err := repo.ListPendingInteractions(ctx, models.PendingInteractionFilter{SessionIDs: []string{"session-lifecycle-pg"}})
	if err != nil {
		t.Fatalf("ListPendingInteractions: %v", err)
	}
	ids := interactionMessageIDs(got)
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want both questions of the bundle stranded behind the lifecycle-only turn", ids)
	}
	for _, want := range []string{"clar-q1-pg", "clar-q2-pg"} {
		found := false
		for _, id := range ids {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("ids = %v, missing %q", ids, want)
		}
	}

	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-lifecycle-pg"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-lifecycle-pg"]; !ok {
		t.Fatal("pending-action projection disagrees with the interaction list: session not reported pending")
	}

	bundles, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	foundBundle := false
	for _, bundle := range bundles.Bundles {
		if bundle.SessionID == "session-lifecycle-pg" {
			foundBundle = true
		}
	}
	if !foundBundle {
		t.Fatal("bundle query disagrees with the interaction list: session not reported pending")
	}
}
