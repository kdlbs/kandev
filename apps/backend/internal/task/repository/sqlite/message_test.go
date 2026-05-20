package sqlite

import (
	"context"
	"testing"
	"time"
)

// insertMsgWithType inserts a message row with a configurable type column,
// so tests can mix tool_call and plain message rows in the same session.
func insertMsgWithType(t *testing.T, repo *Repository, id, sessionID, turnID, msgType string, ts time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, '', ?, 'agent', '', '', 0, ?, '{}', ?)
	`), id, sessionID, turnID, msgType, ts)
	if err != nil {
		t.Fatalf("insert message %s: %v", id, err)
	}
}

func TestCountToolCallMessagesBySession_Empty(t *testing.T) {
	repo := newRepoForSessionTests(t)
	got, err := repo.CountToolCallMessagesBySession(context.Background(), nil)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(got))
	}
}

func TestCountToolCallMessagesBySession_Single(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-A", "sess-A", "turn-A")
	insertMsgWithType(t, repo, "m1", "sess-A", "turn-A", "tool_call", now)
	insertMsgWithType(t, repo, "m2", "sess-A", "turn-A", "tool_call", now.Add(time.Second))
	insertMsgWithType(t, repo, "m3", "sess-A", "turn-A", "message", now.Add(2*time.Second))

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"sess-A"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["sess-A"] != 2 {
		t.Errorf("sess-A count = %d, want 2", got["sess-A"])
	}
}

func TestCountToolCallMessagesBySession_Multi(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedForMsgTest(t, repo, "task-1", "s1", "turn-1")
	seedForMsgTest(t, repo, "task-2", "s2", "turn-2")
	seedForMsgTest(t, repo, "task-3", "s3", "turn-3")
	insertMsgWithType(t, repo, "m-s1-a", "s1", "turn-1", "tool_call", now)
	insertMsgWithType(t, repo, "m-s2-a", "s2", "turn-2", "tool_call", now)
	insertMsgWithType(t, repo, "m-s2-b", "s2", "turn-2", "tool_call", now.Add(time.Second))
	insertMsgWithType(t, repo, "m-s2-c", "s2", "turn-2", "tool_call", now.Add(2*time.Second))
	// s3 has only a plain message — must be omitted from the result map.
	insertMsgWithType(t, repo, "m-s3-a", "s3", "turn-3", "message", now)

	got, err := repo.CountToolCallMessagesBySession(ctx, []string{"s1", "s2", "s3"})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got["s1"] != 1 {
		t.Errorf("s1 count = %d, want 1", got["s1"])
	}
	if got["s2"] != 3 {
		t.Errorf("s2 count = %d, want 3", got["s2"])
	}
	if _, ok := got["s3"]; ok {
		t.Errorf("s3 should be omitted (zero tool_call rows), got %d", got["s3"])
	}
}
