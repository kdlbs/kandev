package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPurgePromptSequencesForSessionsChunksLargeBatches(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const markerCount = 33000
	ids := make([]string, markerCount)
	for i := range ids {
		ids[i] = fmt.Sprintf("prompt-sequence-chunk-%d", i)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for start := 0; start < len(ids); start += sqliteMaxHostParams / 2 {
		end := min(start+sqliteMaxHostParams/2, len(ids))
		values := make([]string, end-start)
		args := make([]any, 0, 2*(end-start))
		for i, id := range ids[start:end] {
			values[i] = "(?, 1)"
			args = append(args, id)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO task_session_prompt_seq (task_session_id, last_seq) VALUES "+strings.Join(values, ","), args...); err != nil {
			t.Fatalf("seed prompt sequence markers: %v", err)
		}
	}
	if err := repo.purgePromptSequencesForSessionsTx(ctx, tx, ids); err != nil {
		t.Fatalf("purge oversized prompt sequence set: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := repo.db.GetContext(ctx, &remaining, `SELECT COUNT(*) FROM task_session_prompt_seq`); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("prompt sequence markers remaining = %d, want 0", remaining)
	}
}

func TestDeleteEphemeralTasksByAgentProfileDoesNotDeleteNewlyMatchingTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "selected-ephemeral-task", "selected-ephemeral-session", "selected-ephemeral-turn")
	seedForMsgTest(t, repo, "new-ephemeral-task", "new-ephemeral-session", "new-ephemeral-turn")
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET is_ephemeral = 1 WHERE id = ?
	`), "selected-ephemeral-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_sessions SET agent_profile_id = ? WHERE id = ?
	`), "profile-cleanup-race", "selected-ephemeral-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER add_new_profile_session AFTER DELETE ON task_session_prompt_seq
		WHEN OLD.task_session_id = 'selected-ephemeral-session'
		BEGIN
			UPDATE tasks SET is_ephemeral = 1 WHERE id = 'new-ephemeral-task';
			INSERT INTO task_sessions (id, task_id, agent_profile_id, started_at, updated_at)
			VALUES ('new-ephemeral-session', 'new-ephemeral-task', 'profile-cleanup-race', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
			INSERT INTO task_session_prompt_seq (task_session_id, last_seq) VALUES ('new-ephemeral-session', 1);
		END;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		DELETE FROM task_sessions WHERE id = ?
	`), "new-ephemeral-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_session_prompt_seq (task_session_id, last_seq) VALUES (?, 1)
	`), "selected-ephemeral-session"); err != nil {
		t.Fatal(err)
	}

	deleted, err := repo.DeleteEphemeralTasksByAgentProfile(ctx, "profile-cleanup-race")
	if err != nil {
		t.Fatalf("delete ephemeral profile tasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted task count = %d, want only the initially guarded task", deleted)
	}
	if _, err := repo.GetTask(ctx, "new-ephemeral-task"); err != nil {
		t.Fatalf("newly matching task was deleted: %v", err)
	}
	if _, err := repo.GetTaskSession(ctx, "new-ephemeral-session"); err != nil {
		t.Fatalf("concurrent profile session was not created: %v", err)
	}
	hasHistory, err := repo.HasUserPromptHistory(ctx, "new-ephemeral-session")
	if err != nil {
		t.Fatalf("read new session prompt marker: %v", err)
	}
	if !hasHistory {
		t.Fatal("new task's prompt marker was purged with an unrelated deletion")
	}
}

func TestDeleteExpiredQuickChatRemovesPromptMarkerAfterDeletingTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "quick-chat-marker-task", "quick-chat-marker-session", "quick-chat-marker-turn")
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET is_ephemeral = 1, updated_at = ? WHERE id = ?
	`), time.Now().Add(-time.Hour), "quick-chat-marker-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_sessions SET state = ?, updated_at = ? WHERE id = ?
	`), "COMPLETED", time.Now().Add(-time.Hour), "quick-chat-marker-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		INSERT INTO task_session_prompt_seq (task_session_id, last_seq) VALUES (?, 1)
	`), "quick-chat-marker-session"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER reactivate_quick_chat_session AFTER DELETE ON task_session_prompt_seq
		WHEN OLD.task_session_id = 'quick-chat-marker-session'
		BEGIN
			UPDATE task_sessions SET state = 'RUNNING' WHERE id = 'quick-chat-marker-session';
		END;
	`); err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.DeleteExpiredQuickChatTask(ctx, "quick-chat-marker-task", time.Now())
	if err != nil {
		t.Fatalf("delete expired quick chat: %v", err)
	}
	if !deleted {
		t.Fatal("eligible quick chat was not deleted")
	}
	if _, err := repo.GetTask(ctx, "quick-chat-marker-task"); err == nil {
		t.Fatal("task still exists after deletion")
	}
	var markers int
	if err := repo.db.GetContext(ctx, &markers, repo.db.Rebind(`
		SELECT COUNT(*) FROM task_session_prompt_seq WHERE task_session_id = ?
	`), "quick-chat-marker-session"); err != nil {
		t.Fatal(err)
	}
	if markers != 0 {
		t.Fatalf("prompt markers remaining after deletion = %d, want 0", markers)
	}
}

func TestClaimInitialPromptFallbackRejectsMissingSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	claimed, err := repo.ClaimInitialPromptFallback(context.Background(), "missing-fallback-session", "missing-incarnation")
	if err != nil {
		t.Fatalf("claim fallback for missing session: %v", err)
	}
	if claimed {
		t.Fatal("fallback claim succeeded for a session that does not exist")
	}
	var markers int
	if err := repo.db.Get(&markers, `SELECT COUNT(*) FROM task_session_prompt_seq`); err != nil {
		t.Fatal(err)
	}
	if markers != 0 {
		t.Fatalf("orphan prompt markers = %d, want 0", markers)
	}
}
