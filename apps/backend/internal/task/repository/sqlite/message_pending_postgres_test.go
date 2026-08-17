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

func TestPostgresPendingActionsExcludeTerminalSessionsAndPreferClarification(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 21, 0, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-precedence-pg", "session-precedence-pg")
	createPendingActionTurn(
		t, repo, "task-precedence-pg", "session-precedence-pg", "turn-precedence-pg", base, base,
	)
	createPendingActionMessage(
		t, repo, "permission-precedence-pg", "task-precedence-pg", "session-precedence-pg",
		"turn-precedence-pg", models.MessageTypePermissionRequest, "pending", base,
	)
	createPendingActionMessage(
		t, repo, "clarification-precedence-pg", "task-precedence-pg", "session-precedence-pg",
		"turn-precedence-pg", models.MessageTypeClarificationRequest, "pending", base.Add(time.Second),
	)
	seedPendingActionSession(t, repo, "task-terminal-action-pg", "session-terminal-action-pg")
	createPendingActionTurn(
		t, repo, "task-terminal-action-pg", "session-terminal-action-pg", "turn-terminal-action-pg", base, base,
	)
	createPendingActionMessage(
		t, repo, "clarification-terminal-action-pg", "task-terminal-action-pg", "session-terminal-action-pg",
		"turn-terminal-action-pg", models.MessageTypeClarificationRequest, "pending", base,
	)
	if err := repo.UpdateTaskSessionState(
		ctx, "session-terminal-action-pg", models.TaskSessionStateCancelled, "cancelled",
	); err != nil {
		t.Fatalf("cancel terminal session: %v", err)
	}

	actions, err := repo.GetPendingActionsBySessionIDs(
		ctx,
		[]string{"session-precedence-pg", "session-terminal-action-pg"},
	)
	if err != nil {
		t.Fatalf("GetPendingActionsBySessionIDs: %v", err)
	}
	if actions["session-precedence-pg"] != models.TaskPendingActionClarification {
		t.Fatalf("postgres precedence action = %#v, want clarification", actions)
	}
	if _, ok := actions["session-terminal-action-pg"]; ok {
		t.Fatalf("postgres terminal session retained pending action: %#v", actions)
	}
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
	_, restored, err := repo.RestoreActiveClarificationBundle(
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

func TestPostgresClarificationClaimWaitsForTerminalSessionWrite(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 5)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 16, 13, 40, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-terminal-claim-pg", "session-terminal-claim-pg")
	createPendingActionTurn(
		t, repo, "task-terminal-claim-pg", "session-terminal-claim-pg",
		"turn-terminal-claim-pg", base, base,
	)
	createClarificationBundleMessage(
		t, repo, "message-terminal-claim-pg", "task-terminal-claim-pg",
		"session-terminal-claim-pg", "turn-terminal-claim-pg",
		"pending-terminal-claim-pg", "q1", base,
	)

	terminalWriter, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin terminal session writer: %v", err)
	}
	t.Cleanup(func() { _ = terminalWriter.Rollback() })
	if _, err := terminalWriter.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_sessions SET state = ? WHERE id = ?
	`), string(models.TaskSessionStateCancelled), "session-terminal-claim-pg"); err != nil {
		t.Fatalf("stage terminal session state: %v", err)
	}

	type completionResult struct {
		claimed bool
		err     error
	}
	completionDone := make(chan completionResult, 1)
	go func() {
		_, claimed, completionErr := repo.CompleteActiveClarificationBundle(
			ctx,
			"pending-terminal-claim-pg",
			clarificationStatusAnswered,
			map[string]interface{}{"q1": "continue"},
		)
		completionDone <- completionResult{claimed: claimed, err: completionErr}
	}()
	waitForPostgresLockWait(t, ctx, repo, "SELECT id FROM task_sessions")
	select {
	case result := <-completionDone:
		t.Fatalf("clarification claim bypassed terminal session row lock: %+v", result)
	default:
	}

	if err := terminalWriter.Commit(); err != nil {
		t.Fatalf("commit terminal session state: %v", err)
	}
	select {
	case result := <-completionDone:
		if result.err != nil || result.claimed {
			t.Fatalf("clarification claim after terminal commit = %+v, want unclaimed", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clarification claim")
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
	for index, flag := range []string{"true", "1"} {
		messageID := "message-detach-string-" + flag + "-pg"
		createClarificationBundleMessage(
			t, repo, messageID, "task-detach-pg", "session-detach-pg",
			"turn-detach-current-pg", "pending-detach-string-"+flag+"-pg", "q-string-"+flag,
			base.Add(time.Minute+time.Duration(index+1)*time.Second),
		)
		setClarificationMessageMetadata(t, repo, messageID, func(metadata map[string]interface{}) {
			metadata["agent_disconnected"] = flag
		})
	}

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
	// Four concurrent holders peak: blocker, detach, turn creation, and lock
	// observer. Two spare connections keep repository bookkeeping off that budget.
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 6)
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

func TestPostgresMessageCreationSerializesWithTurnRollback(t *testing.T) {
	// Four concurrent holders peak; two spare connections preserve lock-test headroom.
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 6)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 15, 16, 20, 0, 0, time.UTC)
	seedPendingActionSession(t, repo, "task-message-lock-pg", "session-message-lock-pg")
	createPendingActionTurn(
		t, repo, "task-message-lock-pg", "session-message-lock-pg", "turn-message-lock-pg", base, base,
	)

	blocker, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin session lock blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback() })
	if err := lockSessionTurnWrites(ctx, blocker, repo.db.DriverName(), "session-message-lock-pg"); err != nil {
		t.Fatalf("hold session turn lock: %v", err)
	}

	createDone := make(chan error, 1)
	go func() {
		createDone <- repo.CreateMessage(ctx, &models.Message{
			ID:            "message-lock-pg",
			TaskSessionID: "session-message-lock-pg",
			TaskID:        "task-message-lock-pg",
			TurnID:        "turn-message-lock-pg",
			AuthorType:    models.MessageAuthorUser,
			Content:       "keep the accepted prompt",
		})
	}()
	select {
	case createErr := <-createDone:
		t.Fatalf("CreateMessage bypassed the session turn lock: %v", createErr)
	case <-time.After(150 * time.Millisecond):
	}
	waitForPostgresLockWait(t, ctx, repo, "pg_advisory_xact_lock")

	type deleteResult struct {
		deleted bool
		err     error
	}
	deleteDone := make(chan deleteResult, 1)
	go func() {
		deleted, deleteErr := repo.DeleteTurnIfUnreferenced(
			ctx,
			"session-message-lock-pg",
			"turn-message-lock-pg",
		)
		deleteDone <- deleteResult{deleted: deleted, err: deleteErr}
	}()
	select {
	case result := <-deleteDone:
		t.Fatalf("DeleteTurnIfUnreferenced bypassed the session turn lock: %+v", result)
	case <-time.After(150 * time.Millisecond):
	}

	if err := blocker.Commit(); err != nil {
		t.Fatalf("release session turn lock: %v", err)
	}
	select {
	case createErr := <-createDone:
		if createErr != nil {
			t.Fatalf("CreateMessage after session lock release: %v", createErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message creation")
	}
	select {
	case result := <-deleteDone:
		if result.err != nil || result.deleted {
			t.Fatalf("turn rollback after message creation = %+v, want preserved turn", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for turn rollback")
	}
	if _, err := repo.GetTurn(ctx, "turn-message-lock-pg"); err != nil {
		t.Fatalf("durable turn was lost: %v", err)
	}
	if _, err := repo.GetMessage(ctx, "message-lock-pg"); err != nil {
		t.Fatalf("durable message was lost: %v", err)
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
