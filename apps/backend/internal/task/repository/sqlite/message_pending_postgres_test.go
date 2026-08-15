package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresActiveClarificationUsesNewestDurableTurn pins the same ownership
// rule as SQLite. It skips unless KANDEV_TEST_POSTGRES_DSN is configured.
func TestPostgresActiveClarificationUsesNewestDurableTurn(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 14, 15, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-active-pg", "session-active-pg")
	createPendingActionTurn(t, repo, "task-active-pg", "session-active-pg", "turn-old-pg", base, base)
	createPendingActionMessage(t, repo, "clarification-old-pg", "task-active-pg", "session-active-pg", "turn-old-pg", models.MessageTypeClarificationRequest, "pending", base)
	createPendingActionTurn(t, repo, "task-active-pg", "session-active-pg", "turn-new-pg", base.Add(time.Minute), base.Add(time.Minute))

	assertNoActivePostgresClarification(t, ctx, repo)
	createPendingActionMessage(t, repo, "clarification-new-pg", "task-active-pg", "session-active-pg", "turn-new-pg", models.MessageTypeClarificationRequest, "<missing>", base.Add(time.Minute))
	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active-pg")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(active); len(ids) != 1 || ids[0] != "clarification-new-pg" {
		t.Fatalf("postgres active clarification IDs = %v", ids)
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active-pg"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if actions["session-active-pg"] != models.TaskPendingActionClarification {
		t.Fatalf("postgres pending action = %#v", actions)
	}

	if err := repo.DeleteMessage(ctx, "clarification-new-pg"); err != nil {
		t.Fatalf("DeleteMessage(new current-turn row): %v", err)
	}
	assertNoActivePostgresClarification(t, ctx, repo)
}

func TestPostgresCompleteActiveClarificationBundleClaimsOnce(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 14, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-claim-pg", "session-claim-pg")
	createPendingActionTurn(t, repo, "task-claim-pg", "session-claim-pg", "turn-claim-pg", base, base)
	createClarificationBundleMessage(t, repo, "message-claim-pg", "task-claim-pg", "session-claim-pg", "turn-claim-pg", "pending-claim-pg", "q1", base)
	responses := map[string]interface{}{
		"q1": map[string]interface{}{"question_id": "q1", "custom_text": "continue"},
	}

	updated, claimed, err := repo.CompleteActiveClarificationBundle(ctx, "pending-claim-pg", "answered", responses)
	if err != nil {
		t.Fatalf("first CompleteActiveClarificationBundle: %v", err)
	}
	if !claimed || len(updated) != 1 {
		t.Fatalf("first completion = claimed %v, rows %d; want true, 1", claimed, len(updated))
	}
	_, claimed, err = repo.CompleteActiveClarificationBundle(ctx, "pending-claim-pg", "answered", responses)
	if err != nil {
		t.Fatalf("second CompleteActiveClarificationBundle: %v", err)
	}
	if claimed {
		t.Fatal("already-answered Postgres bundle was claimed twice")
	}
	restored, err := repo.RestoreActiveClarificationBundle(
		ctx,
		"pending-claim-pg",
		"answered",
		updated,
	)
	if err != nil || !restored {
		t.Fatalf("RestoreActiveClarificationBundle: restored=%v err=%v", restored, err)
	}
	_, claimed, err = repo.CompleteActiveClarificationBundle(ctx, "pending-claim-pg", "answered", responses)
	if err != nil || !claimed {
		t.Fatalf("completion after restore: claimed=%v err=%v", claimed, err)
	}
}

func TestPostgresDetachActiveClarificationMessagesClaimsCurrentRows(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 16, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-detach-pg", "session-detach-pg")
	createPendingActionTurn(
		t, repo, "task-detach-pg", "session-detach-pg", "turn-detach-old-pg", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-detach-old-pg", "task-detach-pg", "session-detach-pg",
		"turn-detach-old-pg", "pending-detach-old-pg", "q-old", base,
	)
	createPendingActionTurn(
		t, repo, "task-detach-pg", "session-detach-pg", "turn-detach-current-pg",
		base.Add(time.Minute), base.Add(time.Minute),
	)
	createClarificationBundleMessage(
		t, repo, "message-detach-current-pg", "task-detach-pg", "session-detach-pg",
		"turn-detach-current-pg", "pending-detach-current-pg", "q-current",
		base.Add(time.Minute),
	)

	updated, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-detach-pg")
	if err != nil {
		t.Fatalf("DetachActiveClarificationMessagesBySessionID: %v", err)
	}
	if ids := messageIDs(updated); len(ids) != 1 || ids[0] != "message-detach-current-pg" {
		t.Fatalf("postgres detached message IDs = %v", ids)
	}
	message, err := repo.GetMessage(ctx, "message-detach-current-pg")
	if err != nil {
		t.Fatalf("GetMessage(current): %v", err)
	}
	if detached, _ := message.Metadata["agent_disconnected"].(bool); !detached {
		t.Fatalf("postgres detached metadata = %#v", message.Metadata)
	}
	repeated, err := repo.DetachActiveClarificationMessagesBySessionID(ctx, "session-detach-pg")
	if err != nil {
		t.Fatalf("repeated postgres detach: %v", err)
	}
	if len(repeated) != 0 {
		t.Fatalf("repeated postgres detach changed rows: %v", messageIDs(repeated))
	}
}

func TestPostgresRestoreClarificationMessagesRechecksCurrentTurn(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 16, 30, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-restore-pg", "session-restore-pg")
	createPendingActionTurn(
		t, repo, "task-restore-pg", "session-restore-pg", "turn-restore-pg", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-restore-pg", "task-restore-pg", "session-restore-pg",
		"turn-restore-pg", "pending-restore-pg", "q1", base,
	)
	claimedMessages, claimed, err := repo.CompleteActiveClarificationBundle(
		ctx,
		"pending-restore-pg",
		"answered",
		map[string]interface{}{"q1": map[string]interface{}{"question_id": "q1"}},
	)
	if err != nil || !claimed {
		t.Fatalf("complete before postgres restore race: claimed=%v err=%v", claimed, err)
	}
	createPendingActionTurn(
		t, repo, "task-restore-pg", "session-restore-pg", "turn-restore-successor-pg",
		base.Add(time.Second), base.Add(time.Second),
	)

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	restoreErr := repo.restoreClarificationMessages(
		ctx,
		tx,
		repo.db.DriverName(),
		claimedMessages,
		"answered",
	)
	_ = tx.Rollback()
	if restoreErr == nil {
		t.Fatal("postgres restore accepted a bundle after a successor turn became current")
	}
}

func assertNoActivePostgresClarification(t *testing.T, ctx context.Context, repo *Repository) {
	t.Helper()
	active, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-active-pg")
	if err != nil {
		t.Fatalf("FindActiveClarificationMessagesBySessionID: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("postgres reactivated older clarification: %v", messageIDs(active))
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{"session-active-pg"})
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if _, ok := actions["session-active-pg"]; ok {
		t.Fatalf("postgres reactivated older pending action: %#v", actions)
	}
}
