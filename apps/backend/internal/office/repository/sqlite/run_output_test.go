package sqlite_test

import (
	"context"
	"strings"
	"testing"
)

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

	got, err := repo.GetFinalAgentMessage(ctx, "s-1")
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

	got, err := repo.GetFinalAgentMessage(ctx, "s-missing")
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

	got, err := repo.GetFinalAgentMessage(ctx, "s-1")
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
