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

func TestSQLiteRepository_UpdateContentAndMetadataPreservesUnrelatedKeys(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()
	msg := &QueuedMessage{
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "original",
		QueuedBy:  "user-1",
		Metadata: map[string]interface{}{
			"entity_references": []interface{}{"old"},
			"origin":            "inter-task",
		},
	}
	if err := repo.Insert(ctx, msg, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := repo.UpdateContentAndMetadata(
		ctx, "s1", msg.ID, "edited", nil,
		map[string]interface{}{"entity_references": []interface{}{"new"}},
		"user-1",
	); err != nil {
		t.Fatalf("update content and metadata: %v", err)
	}

	entries, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(entries))
	}
	if entries[0].Content != "edited" {
		t.Fatalf("content = %q, want edited", entries[0].Content)
	}
	if entries[0].Metadata["origin"] != "inter-task" {
		t.Fatalf("unrelated metadata lost: %+v", entries[0].Metadata)
	}
	wantReferences := []interface{}{"new"}
	gotReferences, ok := entries[0].Metadata["entity_references"].([]interface{})
	if !ok || len(gotReferences) != len(wantReferences) || gotReferences[0] != wantReferences[0] {
		t.Fatalf("entity references = %#v, want %#v", entries[0].Metadata["entity_references"], wantReferences)
	}

	if err := repo.UpdateContentAndMetadata(
		ctx, "s1", msg.ID, "edited again", nil,
		map[string]interface{}{"entity_references": nil},
		"user-1",
	); err != nil {
		t.Fatalf("clear references: %v", err)
	}
	entries, err = repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list after clear: %v", err)
	}
	if entries[0].Metadata["origin"] != "inter-task" {
		t.Fatalf("unrelated metadata lost while clearing: %+v", entries[0].Metadata)
	}
	if _, exists := entries[0].Metadata["entity_references"]; exists {
		t.Fatalf("stale references survived clear: %+v", entries[0].Metadata)
	}
}

func TestSQLiteRepository_ReplaceCoalescedDetectsMissingRow(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	ctx := context.Background()
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = repo.replaceCoalesced(ctx, tx,
		&QueuedMessage{ID: "missing", SessionID: "s1", QueuedBy: QueuedByWorkflow},
		&QueuedMessage{
			SessionID: "s1",
			TaskID:    "t1",
			Content:   "new",
			QueuedBy:  QueuedByWorkflow,
			Metadata:  map[string]interface{}{MetadataCoalesceKey: "ci-key"},
		},
	)
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound for vanished coalesced row, got %v", err)
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

// TestSQLiteRepository_TakeByID covers TakeByID's cross-session guard,
// out-of-FIFO-order removal, idempotent re-take of an already-taken id, and
// (unlike DeleteByID) that agent-authored entries are takeable.
func TestSQLiteRepository_TakeByID(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	first := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "first", QueuedBy: "u"}
	if err := repo.Insert(ctx, first, 0); err != nil {
		t.Fatalf("insert first: %v", err)
	}
	second := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "second", QueuedBy: "u"}
	if err := repo.Insert(ctx, second, 0); err != nil {
		t.Fatalf("insert second: %v", err)
	}
	third := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "third", QueuedBy: "u"}
	if err := repo.Insert(ctx, third, 0); err != nil {
		t.Fatalf("insert third: %v", err)
	}

	// Cross-session take attempt: must not affect the row.
	got, err := repo.TakeByID(ctx, "s-attacker", second.ID)
	if err != nil {
		t.Fatalf("cross-session take: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for cross-session take, got %+v", got)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 3 {
		t.Errorf("entries should survive cross-session take attempt, got count=%d", count)
	}

	// Taking the middle entry out of FIFO order must not disturb the others.
	got, err = repo.TakeByID(ctx, "s1", second.ID)
	if err != nil {
		t.Fatalf("take: %v", err)
	}
	if got == nil || got.Content != "second" {
		t.Fatalf("expected to take %q, got %+v", "second", got)
	}
	remaining, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(remaining) != 2 || remaining[0].Content != "first" || remaining[1].Content != "third" {
		t.Fatalf("expected [first, third] to remain in order, got %+v", remaining)
	}

	// Taking the same id again finds nothing — it's already gone.
	got, err = repo.TakeByID(ctx, "s1", second.ID)
	if err != nil {
		t.Fatalf("re-take: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil on re-take of already-taken id, got %+v", got)
	}

	// Unlike DeleteByID, TakeByID has no queued_by="agent" guard — the caller
	// is internal orchestrator code dispatching its own queued entry.
	agentMsg := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "agent", QueuedBy: QueuedByAgent}
	if err := repo.Insert(ctx, agentMsg, 0); err != nil {
		t.Fatalf("insert agent message: %v", err)
	}
	got, err = repo.TakeByID(ctx, "s1", agentMsg.ID)
	if err != nil {
		t.Fatalf("take agent-authored entry: %v", err)
	}
	if got == nil || got.Content != "agent" {
		t.Fatalf("expected to take agent-authored entry, got %+v", got)
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

func TestSQLiteRepository_ReplaceSessionPreservesQueuedIdentity(t *testing.T) {
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()

	original := &QueuedMessage{
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "original",
		Model:     "model-a",
		PlanMode:  true,
		Metadata:  map[string]interface{}{"sender": "task-a"},
		QueuedBy:  "agent",
	}
	if err := repo.Insert(ctx, original, 0); err != nil {
		t.Fatalf("insert original: %v", err)
	}
	if err := repo.SetPendingMove(ctx, "s1", &PendingMove{TaskID: "t1", WorkflowStepID: "step-a"}); err != nil {
		t.Fatalf("set pending move: %v", err)
	}
	if err := repo.Insert(ctx, &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "mutated", QueuedBy: "user"}, 0); err != nil {
		t.Fatalf("insert mutated: %v", err)
	}

	if err := repo.ReplaceSession(ctx, "s1", []QueuedMessage{*original}, &PendingMove{
		TaskID:         "t1",
		WorkflowStepID: "step-a",
		QueuedAt:       original.QueuedAt,
	}); err != nil {
		t.Fatalf("replace session: %v", err)
	}

	entries, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 restored entry, got %d", len(entries))
	}
	restored := entries[0]
	if restored.ID != original.ID || restored.Position != original.Position || !restored.QueuedAt.Equal(original.QueuedAt) {
		t.Fatalf("identity changed: got id=%s pos=%d queued_at=%s want id=%s pos=%d queued_at=%s",
			restored.ID, restored.Position, restored.QueuedAt, original.ID, original.Position, original.QueuedAt)
	}
	move, err := repo.TakePendingMove(ctx, "s1")
	if err != nil {
		t.Fatalf("take pending move: %v", err)
	}
	if move == nil || move.WorkflowStepID != "step-a" {
		t.Fatalf("pending move = %#v, want step-a", move)
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
