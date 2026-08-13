package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestAppendRunEvent_ReturnedEventMatchesTheStoredRow is the round-trip
// guard for run_events: every column written must come back out of the
// list query, and the value returned to the caller (which is published
// on the event bus without re-reading) must equal what landed on disk.
func TestAppendRunEvent_ReturnedEventMatchesTheStoredRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-1", AgentProfileID: "a1", Reason: "task_assigned",
	})
	before := time.Now().UTC().Add(-time.Second)

	got, err := repo.AppendRunEvent(ctx, run.ID, "run.started", "warn", `{"detail":"x"}`)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	checkString(t, "run_id", got.RunID, run.ID)
	checkInt(t, "seq", got.Seq, 0)
	checkString(t, "event_type", string(got.EventType), "run.started")
	checkString(t, "level", string(got.Level), "warn")
	checkString(t, "payload", got.Payload, `{"detail":"x"}`)
	if !got.CreatedAt.After(before) {
		t.Errorf("created_at = %s, want after %s", got.CreatedAt, before)
	}

	stored, err := repo.ListRunEvents(ctx, run.ID, -1, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("%d stored events, want 1", len(stored))
	}
	checkString(t, "stored run_id", stored[0].RunID, got.RunID)
	checkInt(t, "stored seq", stored[0].Seq, got.Seq)
	checkString(t, "stored event_type", string(stored[0].EventType), string(got.EventType))
	checkString(t, "stored level", string(stored[0].Level), string(got.Level))
	checkString(t, "stored payload", stored[0].Payload, got.Payload)
	if !stored[0].CreatedAt.Equal(got.CreatedAt) {
		t.Errorf("stored created_at = %s, want %s", stored[0].CreatedAt, got.CreatedAt)
	}
}

// TestAppendRunEvent_AppliesLevelAndPayloadDefaults pins the two
// documented fallbacks.
func TestAppendRunEvent_AppliesLevelAndPayloadDefaults(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	got, err := repo.AppendRunEvent(ctx, "run-1", "run.progress", "", "")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	checkString(t, "returned level", string(got.Level), "info")
	checkString(t, "returned payload", got.Payload, "{}")

	stored, err := repo.ListRunEvents(ctx, "run-1", -1, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("%d stored events, want 1", len(stored))
	}
	checkString(t, "stored level", string(stored[0].Level), "info")
	checkString(t, "stored payload", stored[0].Payload, "{}")
}

// TestAppendRunEvent_SeqIsMonotonicPerRun pins that the sequence is
// scoped to one run: each run starts at 0 and increments independently.
func TestAppendRunEvent_SeqIsMonotonicPerRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for want := range 3 {
		got, err := repo.AppendRunEvent(ctx, "run-1", "run.progress", "info", "{}")
		if err != nil {
			t.Fatalf("append %d: %v", want, err)
		}
		checkInt(t, "seq", got.Seq, want)
	}

	other, err := repo.AppendRunEvent(ctx, "run-2", "run.progress", "info", "{}")
	if err != nil {
		t.Fatalf("append to second run: %v", err)
	}
	checkInt(t, "seq of a different run", other.Seq, 0)

	next, err := repo.AppendRunEvent(ctx, "run-1", "run.progress", "info", "{}")
	if err != nil {
		t.Fatalf("append after the second run: %v", err)
	}
	checkInt(t, "seq resumes on the first run", next.Seq, 3)
}

// TestListRunEvents_OrdersBySeqAscendingAndScopesToTheRun inserts rows
// out of seq order so the ORDER BY is what produces the result order,
// not the insertion order.
func TestListRunEvents_OrdersBySeqAscendingAndScopesToTheRun(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, seq := range []int{5, 1, 3} {
		if _, err := writer.Exec(`INSERT INTO run_events
			(run_id, seq, event_type, level, payload, created_at)
			VALUES ('run-1', ?, 'run.progress', 'info', '{}', ?)`, seq, now); err != nil {
			t.Fatalf("insert seq %d: %v", seq, err)
		}
	}
	if _, err := writer.Exec(`INSERT INTO run_events
		(run_id, seq, event_type, level, payload, created_at)
		VALUES ('run-2', 0, 'run.progress', 'info', '{}', ?)`, now); err != nil {
		t.Fatalf("insert other run: %v", err)
	}

	got, err := repo.ListRunEvents(ctx, "run-1", -1, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	checkSeqOrder(t, "run-1 events", got, 1, 3, 5)
}

// TestListRunEvents_AfterSeqReturnsStrictlyNewerEvents pins the live
// tail's cursor semantics.
func TestListRunEvents_AfterSeqReturnsStrictlyNewerEvents(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for range 4 {
		if _, err := repo.AppendRunEvent(ctx, "run-1", "run.progress", "info", "{}"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	all, err := repo.ListRunEvents(ctx, "run-1", -1, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	checkSeqOrder(t, "all events", all, 0, 1, 2, 3)

	tail, err := repo.ListRunEvents(ctx, "run-1", 1, 0)
	if err != nil {
		t.Fatalf("list tail: %v", err)
	}
	checkSeqOrder(t, "events after seq 1", tail, 2, 3)

	caughtUp, err := repo.ListRunEvents(ctx, "run-1", 3, 0)
	if err != nil {
		t.Fatalf("list caught up: %v", err)
	}
	if len(caughtUp) != 0 {
		t.Errorf("got %d events after seq 3, want none", len(caughtUp))
	}
}

// TestListRunEvents_LimitCapsTheOldestEvents pins that the cap applies
// after the ascending sort, and that limit 0 means "no cap".
func TestListRunEvents_LimitCapsTheOldestEvents(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for range 5 {
		if _, err := repo.AppendRunEvent(ctx, "run-1", "run.progress", "info", "{}"); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	capped, err := repo.ListRunEvents(ctx, "run-1", -1, 2)
	if err != nil {
		t.Fatalf("list capped: %v", err)
	}
	checkSeqOrder(t, "limit 2", capped, 0, 1)

	uncapped, err := repo.ListRunEvents(ctx, "run-1", -1, 0)
	if err != nil {
		t.Fatalf("list uncapped: %v", err)
	}
	checkSeqOrder(t, "limit 0", uncapped, 0, 1, 2, 3, 4)
}

// TestListRunEvents_UnknownRunReturnsNoEvents pins the empty case.
func TestListRunEvents_UnknownRunReturnsNoEvents(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.ListRunEvents(context.Background(), "nope", -1, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d events, want none", len(got))
	}
}

func checkSeqOrder(t *testing.T, label string, got []*models.RunEvent, want ...int) {
	t.Helper()
	seqs := make([]int, 0, len(got))
	for _, event := range got {
		seqs = append(seqs, event.Seq)
	}
	if len(seqs) != len(want) {
		t.Fatalf("%s = %v, want %v", label, seqs, want)
	}
	for i := range want {
		if seqs[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, seqs, want)
		}
	}
}
