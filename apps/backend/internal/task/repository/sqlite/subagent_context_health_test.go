package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestSubagentContextHealthCountsAgreeAfterLiveWrites covers AC-28: writing
// through the same path production uses (a subagent_task message plus a
// live UpsertSubagentContext call, exactly as the orchestrator's two frame
// handlers do) leaves both the shortfall and excess anti-joins at zero, and
// max-observed-since reports the latest write.
func TestSubagentContextHealthCountsAgreeAfterLiveWrites(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-health", "session-health", "turn-health")
	ctx := context.Background()
	since := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	ts := since.Add(time.Hour)

	var latest time.Time
	for i, toolCallID := range []string{"tc-health-1", "tc-health-2", "tc-health-3"} {
		observedAt := ts.Add(time.Duration(i) * time.Minute)
		latest = observedAt
		seedSubagentMessage(t, repo, "msg-health-"+toolCallID, "session-health", "task-health", "turn-health",
			toolCallID, "", "completed", map[string]interface{}{"status": "completed"},
			observedAt, observedAt)
		if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
			TaskSessionID: "session-health", TaskID: "task-health", ToolCallID: toolCallID,
			Source: "live", ObservedAt: observedAt, UpdatedAt: observedAt,
		}); err != nil {
			t.Fatalf("UpsertSubagentContext %s: %v", toolCallID, err)
		}
	}

	shortfall, err := repo.subagentContextShortfall(since)
	if err != nil {
		t.Fatalf("subagentContextShortfall: %v", err)
	}
	if shortfall != 0 {
		t.Fatalf("shortfall = %d, want 0 after every message got a live context row", shortfall)
	}

	excess, err := repo.subagentContextExcess(since)
	if err != nil {
		t.Fatalf("subagentContextExcess: %v", err)
	}
	if excess != 0 {
		t.Fatalf("excess = %d, want 0 when every context row is accounted for by a message", excess)
	}

	maxObserved, ok, err := repo.subagentContextMaxObservedSince(since)
	if err != nil {
		t.Fatalf("subagentContextMaxObservedSince: %v", err)
	}
	if !ok {
		t.Fatal("subagentContextMaxObservedSince: want a value, got none")
	}
	if !maxObserved.Equal(latest) {
		t.Fatalf("subagentContextMaxObservedSince = %v, want %v (the last live write)", maxObserved, latest)
	}
}

// TestSubagentContextHealthShortfallDivergesWhenWriterMissesAMessage covers
// AC-28/AC-29's SHORTFALL direction: a subagent_task message the context
// writer never saw (simulating the writer having stopped while message
// writes continued) is the one divergence direction that must alarm, and
// max-observed-since stops advancing at the last moment the writer was
// known healthy — it does not silently track the missed message's time.
func TestSubagentContextHealthShortfallDivergesWhenWriterMissesAMessage(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-diverge", "session-diverge", "turn-diverge")
	ctx := context.Background()
	since := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	writerAlive := since.Add(time.Hour)
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

	if shortfall, err := repo.subagentContextShortfall(since); err != nil || shortfall != 0 {
		t.Fatalf("shortfall before the simulated writer outage = %d, err = %v; want 0", shortfall, err)
	}

	// The writer "stopped" here: the message write (load-bearing for the UI)
	// still happens, but nothing calls UpsertSubagentContext for it.
	seedSubagentMessage(t, repo, "msg-diverge-missed", "session-diverge", "task-diverge", "turn-diverge",
		"tc-diverge-missed", "", "completed", map[string]interface{}{"status": "completed"}, writerDead, writerDead)

	shortfall, err := repo.subagentContextShortfall(since)
	if err != nil {
		t.Fatalf("subagentContextShortfall: %v", err)
	}
	if shortfall != 1 {
		t.Fatalf("shortfall = %d, want exactly 1 (the message the writer missed)", shortfall)
	}

	excess, err := repo.subagentContextExcess(since)
	if err != nil {
		t.Fatalf("subagentContextExcess: %v", err)
	}
	if excess != 0 {
		t.Fatalf("excess = %d, want 0: a missed write is a shortfall, never an excess", excess)
	}

	maxObserved, ok, err := repo.subagentContextMaxObservedSince(since)
	if err != nil {
		t.Fatalf("subagentContextMaxObservedSince: %v", err)
	}
	if !ok {
		t.Fatal("subagentContextMaxObservedSince: want a value, got none")
	}
	if !maxObserved.Equal(writerAlive) {
		t.Fatalf("subagentContextMaxObservedSince = %v, want %v (the last moment the writer was known healthy)", maxObserved, writerAlive)
	}
	if !maxObserved.Before(writerDead) {
		t.Fatalf("subagentContextMaxObservedSince = %v, want it to predate the missed message at %v", maxObserved, writerDead)
	}
}

// TestSubagentContextHealthExcessDoesNotAlarm covers AC-28's EXCESS
// direction: a context row with no accounting message (a message-write
// failure alongside a successful context upsert) is attributed but must
// never alarm, and must never mask a genuine shortfall elsewhere in the
// same window.
func TestSubagentContextHealthExcessDoesNotAlarm(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-excess", "session-excess", "turn-excess")
	ctx := context.Background()
	since := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	ts := since.Add(time.Hour)

	// A context row with no matching message: the message write "failed"
	// while the context upsert "succeeded" (a straddling/attributed case).
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-excess", TaskID: "task-excess", ToolCallID: "tc-excess-orphan",
		Source: "live", ObservedAt: ts, UpdatedAt: ts,
	}); err != nil {
		t.Fatalf("UpsertSubagentContext: %v", err)
	}

	// A genuine shortfall elsewhere in the same window: a message with no
	// context row at all.
	seedSubagentMessage(t, repo, "msg-excess-shortfall", "session-excess", "task-excess", "turn-excess",
		"tc-excess-shortfall", "", "completed", map[string]interface{}{"status": "completed"},
		ts.Add(time.Minute), ts.Add(time.Minute))

	excess, err := repo.subagentContextExcess(since)
	if err != nil {
		t.Fatalf("subagentContextExcess: %v", err)
	}
	if excess != 1 {
		t.Fatalf("excess = %d, want exactly 1 (the orphaned context row)", excess)
	}

	shortfall, err := repo.subagentContextShortfall(since)
	if err != nil {
		t.Fatalf("subagentContextShortfall: %v", err)
	}
	if shortfall != 1 {
		t.Fatalf("shortfall = %d, want exactly 1: the excess row must not net against or mask the real shortfall", shortfall)
	}
}

// TestSubagentContextHealthMaxObservedSinceUnscopedWouldOverclaim covers
// SR44 and AC-29 revision 4: a store whose only rows predate `since` (e.g.
// backfilled or legacy rows) must report "no value" when scoped to since,
// even though an unscoped MAX(observed_at) over the same table would return
// a non-NULL value. Reading that NULL/false result as "healthy" would hide
// the fact that this installation's live writer has never produced anything
// since it activated.
func TestSubagentContextHealthMaxObservedSinceUnscopedWouldOverclaim(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-old", "session-old", "turn-old")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	since := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_subagents (id, task_session_id, task_id, tool_call_id, source, observed_at, updated_at)
		VALUES (?, ?, ?, ?, 'backfill', ?, ?)
	`), "row-old", "session-old", "task-old", "tc-old", old, old); err != nil {
		t.Fatalf("seed pre-activation row: %v", err)
	}

	var unscopedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM task_session_subagents WHERE observed_at IS NOT NULL`).Scan(&unscopedCount); err != nil {
		t.Fatalf("sanity count: %v", err)
	}
	if unscopedCount == 0 {
		t.Fatal("sanity check failed: expected the seeded row to have a non-NULL observed_at")
	}

	_, ok, err := repo.subagentContextMaxObservedSince(since)
	if err != nil {
		t.Fatalf("subagentContextMaxObservedSince: %v", err)
	}
	if ok {
		t.Fatal("subagentContextMaxObservedSince: want no value (false) when every row predates since, even though an unscoped MAX would be non-NULL")
	}
}
