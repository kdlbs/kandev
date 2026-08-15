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

func TestPostgresExpireActiveClarificationMessagesPreservesTerminalRows(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 16, 10, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-expire-pg", "session-expire-pg")
	createPendingActionTurn(
		t, repo, "task-expire-pg", "session-expire-pg", "turn-expire-pg", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-expire-pg", "task-expire-pg", "session-expire-pg",
		"turn-expire-pg", "pending-expire-pg", "q-pending", base,
	)
	createClarificationBundleMessage(
		t, repo, "message-answered-pg", "task-expire-pg", "session-expire-pg",
		"turn-expire-pg", "pending-answered-pg", "q-answered", base.Add(time.Second),
	)
	setClarificationMessageMetadata(t, repo, "message-answered-pg", func(metadata map[string]interface{}) {
		metadata["status"] = "answered"
	})

	updated, err := repo.ExpireActiveClarificationBundle(ctx, "session-expire-pg", "pending-expire-pg")
	if err != nil {
		t.Fatalf("ExpireActiveClarificationBundle: %v", err)
	}
	if ids := messageIDs(updated); len(ids) != 1 || ids[0] != "message-expire-pg" {
		t.Fatalf("postgres expired message IDs = %v", ids)
	}
	expired, err := repo.GetMessage(ctx, "message-expire-pg")
	if err != nil {
		t.Fatalf("GetMessage(expired): %v", err)
	}
	if expired.Metadata["status"] != "expired" || expired.Metadata["agent_disconnected"] != true {
		t.Fatalf("postgres expired metadata = %#v", expired.Metadata)
	}
	answered, err := repo.GetMessage(ctx, "message-answered-pg")
	if err != nil {
		t.Fatalf("GetMessage(answered): %v", err)
	}
	if answered.Metadata["status"] != "answered" {
		t.Fatalf("postgres answered status = %v", answered.Metadata["status"])
	}
}

func TestPostgresTurnCreationSerializesWithClarificationDetach(t *testing.T) {
	// The blocker, detach, turn creation, and lock observer must use separate
	// physical connections. The regular helper intentionally exposes one.
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 16, 15, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-detach-lock-pg", "session-detach-lock-pg")
	createPendingActionTurn(
		t, repo, "task-detach-lock-pg", "session-detach-lock-pg", "turn-detach-lock-pg", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-detach-lock-pg", "task-detach-lock-pg", "session-detach-lock-pg",
		"turn-detach-lock-pg", "pending-detach-lock-pg", "q1", base,
	)

	blocker, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin row blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback() })
	var lockedMessageID string
	if err := blocker.QueryRowxContext(
		ctx,
		repo.db.Rebind(`SELECT id FROM task_session_messages WHERE id = ? FOR UPDATE`),
		"message-detach-lock-pg",
	).Scan(&lockedMessageID); err != nil {
		t.Fatalf("lock clarification row: %v", err)
	}

	type detachResult struct {
		messages []*models.Message
		err      error
	}
	detachDone := make(chan detachResult, 1)
	go func() {
		messages, detachErr := repo.DetachActiveClarificationMessagesBySessionID(
			ctx,
			"session-detach-lock-pg",
		)
		detachDone <- detachResult{messages: messages, err: detachErr}
	}()
	waitForPostgresLockWait(t, ctx, repo, "UPDATE task_session_messages")

	createStarted := make(chan struct{})
	createDone := make(chan error, 1)
	go func() {
		close(createStarted)
		createDone <- repo.CreateTurn(ctx, &models.Turn{
			ID:            "turn-successor-lock-pg",
			TaskSessionID: "session-detach-lock-pg",
			TaskID:        "task-detach-lock-pg",
			StartedAt:     base.Add(time.Minute),
		})
	}()
	<-createStarted
	select {
	case createErr := <-createDone:
		t.Fatalf("CreateTurn completed before clarification detach released its session lock: %v", createErr)
	case <-time.After(150 * time.Millisecond):
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release clarification row: %v", err)
	}
	select {
	case result := <-detachDone:
		if result.err != nil {
			t.Fatalf("detach clarification: %v", result.err)
		}
		if ids := messageIDs(result.messages); len(ids) != 1 || ids[0] != "message-detach-lock-pg" {
			t.Fatalf("detached message IDs = %v", ids)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clarification detach")
	}
	select {
	case createErr := <-createDone:
		if createErr != nil {
			t.Fatalf("CreateTurn after clarification detach: %v", createErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for successor turn creation")
	}
}

func waitForPostgresLockWait(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	queryFragment string,
) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		err := repo.db.QueryRowxContext(waitCtx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND pid != pg_backend_pid()
				  AND state = 'active'
				  AND wait_event_type = 'Lock'
				  AND query LIKE '%' || $1 || '%'
			)
		`, queryFragment).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect PostgreSQL lock wait: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-waitCtx.Done():
			t.Fatalf("timed out waiting for PostgreSQL query containing %q to block", queryFragment)
		case <-ticker.C:
		}
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
