package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// subagentMessageCount runs the AC-28 message-side query verbatim: a count
// of task_session_messages whose metadata.normalized.kind is subagent_task.
// This is the independent expectation because the message write is
// load-bearing for the UI subagent card — a broken message write is noticed
// by a human within one turn, whereas a broken context write is noticed by
// nobody. That asymmetry is what makes the comparison a real check.
func subagentMessageCount(t *testing.T, repo *Repository) int {
	t.Helper()
	var count int
	if err := repo.db.QueryRow(`
		SELECT COUNT(*) FROM task_session_messages
		WHERE json_extract(metadata, '$.normalized.kind') = 'subagent_task'
	`).Scan(&count); err != nil {
		t.Fatalf("subagent message count: %v", err)
	}
	return count
}

func subagentContextCount(t *testing.T, repo *Repository) int {
	t.Helper()
	var count int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM task_session_subagents`).Scan(&count); err != nil {
		t.Fatalf("subagent context count: %v", err)
	}
	return count
}

// TestSubagentContextHealthCountsAgreeAfterLiveWrites covers AC-28: writing
// through the same path production uses (a subagent_task message plus a
// live UpsertSubagentContext call, exactly as the orchestrator's two frame
// handlers do) keeps the message-side and context-side counts equal.
func TestSubagentContextHealthCountsAgreeAfterLiveWrites(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-health", "session-health", "turn-health")
	ctx := context.Background()
	ts := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

	for i, toolCallID := range []string{"tc-health-1", "tc-health-2", "tc-health-3"} {
		seedSubagentMessage(t, repo, "msg-health-"+toolCallID, "session-health", "task-health", "turn-health",
			toolCallID, "", "completed", map[string]interface{}{"status": "completed"},
			ts.Add(time.Duration(i)*time.Minute), ts.Add(time.Duration(i)*time.Minute))
		if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
			TaskSessionID: "session-health", TaskID: "task-health", ToolCallID: toolCallID,
			Source: "live", ObservedAt: ts.Add(time.Duration(i) * time.Minute), UpdatedAt: ts.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("UpsertSubagentContext %s: %v", toolCallID, err)
		}
	}

	messageCount := subagentMessageCount(t, repo)
	contextCount := subagentContextCount(t, repo)
	if messageCount != contextCount {
		t.Fatalf("message count = %d, context count = %d; want equal after live writes", messageCount, contextCount)
	}
	if messageCount != 3 {
		t.Fatalf("message count = %d, want 3", messageCount)
	}
}

// TestSubagentContextHealthDivergesWhenWriterMissesAMessage covers AC-28/
// AC-29: a subagent_task message the context writer never saw (simulating
// the writer having stopped while message writes continued) makes the two
// counts diverge, and MAX(observed_at) on task_session_subagents locates the
// divergence in time — it stops advancing at the point the writer died,
// while message writes (and their created_at) keep moving.
func TestSubagentContextHealthDivergesWhenWriterMissesAMessage(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-diverge", "session-diverge", "turn-diverge")
	ctx := context.Background()
	writerAlive := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	writerDead := writerAlive.Add(time.Hour)

	// The writer was healthy for this one: both sides get a row.
	seedSubagentMessage(t, repo, "msg-diverge-seen", "session-diverge", "task-diverge", "turn-diverge",
		"tc-diverge-seen", "", "completed", map[string]interface{}{"status": "completed"}, writerAlive, writerAlive)
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-diverge", TaskID: "task-diverge", ToolCallID: "tc-diverge-seen",
		Source: "live", ObservedAt: writerAlive, UpdatedAt: writerAlive,
	}); err != nil {
		t.Fatalf("UpsertSubagentContext: %v", err)
	}

	if messageCount, contextCount := subagentMessageCount(t, repo), subagentContextCount(t, repo); messageCount != contextCount {
		t.Fatalf("counts diverged before the simulated writer outage: messages=%d contexts=%d", messageCount, contextCount)
	}

	// The writer "stopped" here: the message write (load-bearing for the UI)
	// still happens, but nothing calls UpsertSubagentContext for it.
	seedSubagentMessage(t, repo, "msg-diverge-missed", "session-diverge", "task-diverge", "turn-diverge",
		"tc-diverge-missed", "", "completed", map[string]interface{}{"status": "completed"}, writerDead, writerDead)

	messageCount := subagentMessageCount(t, repo)
	contextCount := subagentContextCount(t, repo)
	if messageCount == contextCount {
		t.Fatalf("counts = %d/%d, want divergence after the writer misses a message", messageCount, contextCount)
	}
	if messageCount != contextCount+1 {
		t.Fatalf("message count = %d, context count = %d; want exactly one message uncovered by a context row", messageCount, contextCount)
	}

	// SQLite's MAX() aggregate loses the driver's automatic TIMESTAMP-column
	// conversion, so scan the raw text and parse it explicitly rather than
	// into time.Time directly.
	var maxObservedAtRaw string
	if err := repo.db.QueryRow(`SELECT MAX(observed_at) FROM task_session_subagents`).Scan(&maxObservedAtRaw); err != nil {
		t.Fatalf("max observed_at: %v", err)
	}
	maxObservedAt, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", maxObservedAtRaw)
	if err != nil {
		t.Fatalf("parse max observed_at %q: %v", maxObservedAtRaw, err)
	}
	if !maxObservedAt.Equal(writerAlive) {
		t.Fatalf("MAX(observed_at) = %v, want %v (the last moment the writer was known healthy)", maxObservedAt, writerAlive)
	}
	if !maxObservedAt.Before(writerDead) {
		t.Fatalf("MAX(observed_at) = %v, want it to predate the missed message at %v, locating the outage in time", maxObservedAt, writerDead)
	}
}
