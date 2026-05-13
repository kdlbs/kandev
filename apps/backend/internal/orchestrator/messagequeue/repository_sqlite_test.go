package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestSQLiteRepo(t *testing.T) Repository {
	t.Helper()
	raw, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}

func TestSQLiteRepository_InsertList(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	for i, body := range []string{"a", "b", "c"} {
		msg := &QueuedMessage{
			SessionID: "s1", TaskID: "t1", Content: body, QueuedBy: "user-1",
		}
		if err := repo.Insert(ctx, msg, 10); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		if msg.ID == "" {
			t.Errorf("insert %d: expected ID to be assigned", i)
		}
	}

	entries, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i, want := range []string{"a", "b", "c"} {
		if entries[i].Content != want {
			t.Errorf("entry %d content: got %q want %q", i, entries[i].Content, want)
		}
	}
	if entries[0].Position >= entries[1].Position || entries[1].Position >= entries[2].Position {
		t.Errorf("positions not monotonic: %d, %d, %d", entries[0].Position, entries[1].Position, entries[2].Position)
	}
}

func TestSQLiteRepository_InsertRejectsOverflow(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", QueuedBy: "user"}, 10); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	err := repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", QueuedBy: "user"}, 10)
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

func TestSQLiteRepository_TakeHeadFIFO(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	for _, body := range []string{"first", "second", "third"} {
		if err := repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: body, QueuedBy: "u"}, 0); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	for _, want := range []string{"first", "second", "third"} {
		got, err := repo.TakeHead(ctx, "s1")
		if err != nil {
			t.Fatalf("take: %v", err)
		}
		if got == nil {
			t.Fatalf("take: nil for %q", want)
		}
		if got.Content != want {
			t.Errorf("take: got %q, want %q", got.Content, want)
		}
	}
	got, err := repo.TakeHead(ctx, "s1")
	if err != nil {
		t.Fatalf("take empty: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil head on empty queue, got %+v", got)
	}
}

func TestSQLiteRepository_AppendOrInsertTail(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	out, appended, err := repo.AppendOrInsertTail(ctx, "s1", "t1", "first", "", "user", false, nil, nil, 10)
	if err != nil {
		t.Fatalf("append (initial): %v", err)
	}
	if appended {
		t.Error("first call should insert, not append")
	}
	if out.Content != "first" {
		t.Errorf("first content: got %q", out.Content)
	}

	out, appended, err = repo.AppendOrInsertTail(ctx, "s1", "t1", "extra", "", "user", false, nil, nil, 10)
	if err != nil {
		t.Fatalf("append (same sender): %v", err)
	}
	if !appended {
		t.Error("same-sender call should append")
	}
	if out.Content != "first\n\n---\n\nextra" {
		t.Errorf("appended content: got %q", out.Content)
	}

	out, appended, err = repo.AppendOrInsertTail(ctx, "s1", "t1", "from agent", "", "agent", false, nil, nil, 10)
	if err != nil {
		t.Fatalf("append (different sender): %v", err)
	}
	if appended {
		t.Error("different-sender call should insert, not append")
	}
	if out.Content != "from agent" {
		t.Errorf("agent content: got %q", out.Content)
	}

	entries, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (user-coalesced + agent), got %d", len(entries))
	}
}

func TestSQLiteRepository_UpdateContent(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	msg := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "original", QueuedBy: "user-1"}
	if err := repo.Insert(ctx, msg, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.UpdateContent(ctx, "s1", msg.ID, "updated", nil, "user-1"); err != nil {
		t.Fatalf("update (matching sender): %v", err)
	}
	entries, _ := repo.ListBySession(ctx, "s1")
	if entries[0].Content != "updated" {
		t.Errorf("content after update: got %q", entries[0].Content)
	}

	err := repo.UpdateContent(ctx, "s1", msg.ID, "intruder", nil, "user-2")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound for non-matching sender, got %v", err)
	}

	// Cross-session: same entry id but a different session must not match.
	err = repo.UpdateContent(ctx, "s-attacker", msg.ID, "hijack", nil, "user-1")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound for cross-session update, got %v", err)
	}

	err = repo.UpdateContent(ctx, "s1", "nonexistent", "x", nil, "")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound for unknown id, got %v", err)
	}
}

func TestSQLiteRepository_DeleteByID(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	msg := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "x", QueuedBy: "u"}
	if err := repo.Insert(ctx, msg, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Cross-session deletion attempt: must not affect the row.
	if err := repo.DeleteByID(ctx, "s-attacker", msg.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound for cross-session delete, got %v", err)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 1 {
		t.Errorf("entry should survive cross-session delete attempt, got count=%d", count)
	}

	if err := repo.DeleteByID(ctx, "s1", msg.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.DeleteByID(ctx, "s1", msg.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound on second delete, got %v", err)
	}

	agentMsg := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "agent", QueuedBy: QueuedByAgent}
	if err := repo.Insert(ctx, agentMsg, 0); err != nil {
		t.Fatalf("insert agent message: %v", err)
	}
	if err := repo.DeleteByID(ctx, "s1", agentMsg.ID); !errors.Is(err, ErrEntryNotFound) {
		t.Errorf("expected ErrEntryNotFound for agent-authored entry delete, got %v", err)
	}
	count, _ = repo.CountBySession(ctx, "s1")
	if count != 1 {
		t.Errorf("agent-authored entry should survive delete attempt, got count=%d", count)
	}
}

func TestSQLiteRepository_DeleteAllBySession(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_ = repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", QueuedBy: "u"}, 0)
	}
	_ = repo.Insert(ctx, &QueuedMessage{SessionID: "s2", TaskID: "t1", QueuedBy: "u"}, 0)

	n, err := repo.DeleteAllBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("delete all: %v", err)
	}
	if n != 5 {
		t.Errorf("deleted: got %d, want 5", n)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 0 {
		t.Errorf("s1 count after delete-all: %d", count)
	}
	count, _ = repo.CountBySession(ctx, "s2")
	if count != 1 {
		t.Errorf("s2 count: got %d, want 1", count)
	}
}

func TestSQLiteRepository_TransferSession(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	_ = repo.Insert(ctx, &QueuedMessage{SessionID: "s-old", TaskID: "t1", Content: "a", QueuedBy: "u"}, 0)
	_ = repo.Insert(ctx, &QueuedMessage{SessionID: "s-old", TaskID: "t1", Content: "b", QueuedBy: "u"}, 0)
	_ = repo.Insert(ctx, &QueuedMessage{SessionID: "s-new", TaskID: "t1", Content: "existing", QueuedBy: "u"}, 0)

	if err := repo.TransferSession(ctx, "s-old", "s-new"); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	entries, err := repo.ListBySession(ctx, "s-new")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries on dest after transfer, got %d", len(entries))
	}
	if entries[0].Content != "existing" {
		t.Errorf("destination tail order broken: head=%q", entries[0].Content)
	}

	count, _ := repo.CountBySession(ctx, "s-old")
	if count != 0 {
		t.Errorf("source still has %d entries after transfer", count)
	}
}

func TestSQLiteRepository_PendingMove(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	if move, err := repo.TakePendingMove(ctx, "s1"); err != nil || move != nil {
		t.Fatalf("expected nil move on empty, got %v err=%v", move, err)
	}

	move := &PendingMove{TaskID: "t1", WorkflowID: "w1", WorkflowStepID: "step-A", Position: 0}
	if err := repo.SetPendingMove(ctx, "s1", move); err != nil {
		t.Fatalf("set pending: %v", err)
	}

	// Upsert: replace with new target.
	move.WorkflowStepID = "step-B"
	if err := repo.SetPendingMove(ctx, "s1", move); err != nil {
		t.Fatalf("upsert pending: %v", err)
	}

	got, err := repo.TakePendingMove(ctx, "s1")
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if got == nil || got.WorkflowStepID != "step-B" {
		t.Errorf("expected step-B after upsert, got %+v", got)
	}

	// Take again -> nil.
	got, err = repo.TakePendingMove(ctx, "s1")
	if err != nil || got != nil {
		t.Errorf("expected empty after take, got %+v err=%v", got, err)
	}
}

// TestSQLiteRepository_ConcurrentInsertCap exercises the cap under contention:
// 50 goroutines insert into one session with cap=10. Exactly 10 should succeed.
func TestSQLiteRepository_ConcurrentInsertCap(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	const goroutines = 50
	const max = 10

	var (
		wg    sync.WaitGroup
		ok    atomic.Int32
		full  atomic.Int32
		other atomic.Int32
	)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", QueuedBy: "u"}, max)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrQueueFull):
				full.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	wg.Wait()

	if other.Load() != 0 {
		t.Errorf("unexpected non-cap errors: %d", other.Load())
	}
	if ok.Load() != int32(max) {
		t.Errorf("expected exactly %d successful inserts, got %d", max, ok.Load())
	}
	if full.Load() != int32(goroutines-max) {
		t.Errorf("expected %d ErrQueueFull, got %d", goroutines-max, full.Load())
	}
}
