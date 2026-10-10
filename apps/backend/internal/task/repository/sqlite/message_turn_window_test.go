package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestMessageTurnWindowBounds(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertMessageTurnWindowBound(t, repo, "message-turn-window-sqlite")
}

func TestMessageTurnWindowPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	assertMessageTurnWindowBound(t, repo, "message-turn-window-postgres")
}

func assertMessageTurnWindowBound(t *testing.T, repo *Repository, suffix string) {
	t.Helper()
	ctx := context.Background()
	taskID := "task-" + suffix
	sessionID := "session-" + suffix
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'turn window', ?, ?)
	`), taskID, base, base); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, taskID, base, base); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	insertTurns := func(first, count int, start time.Time, complete bool) {
		t.Helper()
		tx, err := repo.db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("begin turn fixture: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		query := tx.Rebind(`
			INSERT INTO task_session_turns
				(id, task_session_id, task_id, started_at, completed_at, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, '{}', ?, ?)
		`)
		for index := first; index < first+count; index++ {
			started := start.Add(time.Duration(index) * time.Second)
			var completed any
			if complete {
				completed = started.Add(time.Second)
			}
			if _, err := tx.ExecContext(ctx, query,
				fmt.Sprintf("turn-%s-%04d", suffix, index), sessionID, taskID, started, completed, started, started,
			); err != nil {
				t.Fatalf("insert turn %d: %v", index, err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit turn fixture: %v", err)
		}
	}

	insertTurns(0, 200, base.Add(-2*time.Hour), true)
	insertTurns(200, 51, base, true)
	activeAt := base.Add(time.Hour)
	insertTurns(251, 1, activeAt, false)

	insertMessages := func() {
		t.Helper()
		tx, err := repo.db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatalf("begin message fixture: %v", err)
		}
		defer func() { _ = tx.Rollback() }()
		query := tx.Rebind(`
			INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at, updated_at, prompt_seq)
			VALUES (?, ?, ?, ?, 'agent', '', 'context', 0, 'message', '{}', ?, ?, 0)
		`)
		for index := 200; index < 251; index++ {
			created := base.Add(time.Duration(index-200) * time.Second)
			if _, err := tx.ExecContext(ctx, query,
				fmt.Sprintf("message-%s-%04d", suffix, index), sessionID, taskID,
				fmt.Sprintf("turn-%s-%04d", suffix, index), created, created,
			); err != nil {
				t.Fatalf("insert message %d: %v", index, err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit message fixture: %v", err)
		}
	}
	insertMessages()

	read := func() models.MessageTurnWindow {
		t.Helper()
		window, err := repo.ReadMessageTurnWindow(ctx, sessionID, models.ListMessagesOptions{
			Limit: 50,
			Sort:  "desc",
		})
		if err != nil {
			t.Fatalf("ReadMessageTurnWindow: %v", err)
		}
		return window
	}

	beforeExpansion := read()
	insertTurns(1000, 1800, base.Add(-24*time.Hour), true)
	afterExpansion := read()
	for _, window := range []models.MessageTurnWindow{beforeExpansion, afterExpansion} {
		if len(window.Messages) != 50 || !window.HasMore {
			t.Fatalf("message page = %d messages, hasMore=%v, want 50 and true", len(window.Messages), window.HasMore)
		}
		if len(window.Turns) > 51 {
			t.Fatalf("turn window has %d rows, want at most 51", len(window.Turns))
		}
		if len(window.Turns) != 51 {
			t.Fatalf("turn window has %d rows, want 50 message turns plus active", len(window.Turns))
		}
		if window.Coverage == nil {
			t.Fatal("turn coverage is missing")
		}
		if window.Coverage.ActiveTurnID != "turn-"+suffix+"-0251" {
			t.Fatalf("active turn = %q, want %q", window.Coverage.ActiveTurnID, "turn-"+suffix+"-0251")
		}
		if len(window.Coverage.MessageIDs) != 50 {
			t.Fatalf("covered message IDs = %d, want 50", len(window.Coverage.MessageIDs))
		}
	}
	if len(beforeExpansion.Turns) != len(afterExpansion.Turns) {
		t.Fatalf("turn count grew after unrelated history expansion: %d -> %d", len(beforeExpansion.Turns), len(afterExpansion.Turns))
	}
}
