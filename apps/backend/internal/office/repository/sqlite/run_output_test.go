package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"
)

// sinceAllFixtures is a since bound before every fixture timestamp used in
// this file's 2026-08-01 seed data, i.e. "no run-scoping in effect".
var sinceAllFixtures = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func TestGetFinalAgentMessage_ReturnsLatestAgentMessage(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-1', 's-1', 'message', 'user',  'please do the thing',    '2026-08-01 10:00:00'),
			('m-2', 's-1', 'message', 'agent', 'first agent reply',      '2026-08-01 10:00:05'),
			('m-3', 's-1', 'tool_call', 'agent', 'ran a tool',           '2026-08-01 10:00:06'),
			('m-4', 's-1', 'message', 'agent', 'final agent reply',      '2026-08-01 10:00:10'),
			('m-5', 's-2', 'message', 'agent', 'other session reply',    '2026-08-01 10:00:10')
	`); err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	got, err := repo.GetFinalAgentMessage(ctx, "s-1", sinceAllFixtures)
	if err != nil {
		t.Fatalf("GetFinalAgentMessage: %v", err)
	}
	if got != "final agent reply" {
		t.Errorf("got %q, want %q", got, "final agent reply")
	}
}

func TestGetFinalAgentMessage_UnknownSessionReturnsEmpty(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	got, err := repo.GetFinalAgentMessage(ctx, "s-missing", sinceAllFixtures)
	if err != nil {
		t.Fatalf("GetFinalAgentMessage: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestGetFinalAgentMessage_TruncatesAt500Chars(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	long := strings.Repeat("a", 600)
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at)
		 VALUES ('m-1', 's-1', 'message', 'agent', ?, '2026-08-01 10:00:00')`, long); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	got, err := repo.GetFinalAgentMessage(ctx, "s-1", sinceAllFixtures)
	if err != nil {
		t.Fatalf("GetFinalAgentMessage: %v", err)
	}
	if len(got) != 500 {
		t.Fatalf("len(got) = %d, want 500", len(got))
	}
	if got != strings.Repeat("a", 500) {
		t.Errorf("got unexpected content")
	}
}

// TestGetFinalAgentMessage_TiedTimestampUsesIDAsTiebreaker pins the id DESC
// secondary sort: two agent messages sharing a created_at value must return
// a deterministic result rather than whichever row SQLite happens to visit
// last, matching the tie-breaker convention documented in
// task/repository/sqlite/session.go.
func TestGetFinalAgentMessage_TiedTimestampUsesIDAsTiebreaker(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-a', 's-1', 'message', 'agent', 'reply a', '2026-08-01 10:00:00'),
			('m-b', 's-1', 'message', 'agent', 'reply b', '2026-08-01 10:00:00')
	`); err != nil {
		t.Fatalf("seed messages: %v", err)
	}

	got, err := repo.GetFinalAgentMessage(ctx, "s-1", sinceAllFixtures)
	if err != nil {
		t.Fatalf("GetFinalAgentMessage: %v", err)
	}
	if got != "reply b" {
		t.Errorf("got %q, want %q (id DESC tiebreaker: m-b > m-a)", got, "reply b")
	}
}

// TestGetFinalAgentMessage_ScopesToSinceExcludesEarlierMessage pins the
// run-scoping fix: office task-bound sessions are reused across runs, so a
// message created before the since bound (a prior run's turn) must not be
// returned even though it is the session's most recent agent message.
func TestGetFinalAgentMessage_ScopesToSinceExcludesEarlierMessage(t *testing.T) {
	repo := newTestRepo(t)
	ensureSessionTables(t, repo)
	ctx := context.Background()

	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO task_session_messages (id, task_session_id, type, author_type, content, created_at) VALUES
			('m-1', 's-1', 'message', 'agent', 'prior run reply', '2026-08-01 10:00:00')
	`); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	since := time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
	got, err := repo.GetFinalAgentMessage(ctx, "s-1", since)
	if err != nil {
		t.Fatalf("GetFinalAgentMessage: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (prior run's message must not leak into this run)", got)
	}
}
