package sqlite

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// openPostgresRepo opens an isolated Postgres schema with the task repository
// initialized (schema + prompt-order index applied).
func openPostgresRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}
	return repo
}

// seedPostgresSession creates a task session and user message rows in an isolated PostgreSQL schema for concurrency tests.
func seedPostgresSession(t *testing.T, repo *Repository, taskID, sessionID, turnID string, base time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskSessionID: sessionID, TaskID: taskID,
		StartedAt: base, CreatedAt: base, UpdatedAt: base,
	}); err != nil {
		t.Fatalf("CreateTurn(%s): %v", turnID, err)
	}
}

// TestPostgresPromptIndexReadsAndPagination repeats the indexed-read,
// tie-by-id, non-user, cross-session, and pagination assertions on the
// PostgreSQL dialect (persisted prompt_seq must agree across reads).
func TestPostgresPromptIndexReadsAndPagination(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	seedPostgresSession(t, repo, "task-pg-a", "sess-pg-a", "turn-pg-a", base)

	insertPromptRow(t, repo, "pg-a-1", "sess-pg-a", "turn-pg-a", "user", "p1", base)
	// Identical microsecond, reversed id order: ties break by id ascending.
	insertPromptRow(t, repo, "pg-a-zzz", "sess-pg-a", "turn-pg-a", "user", "p2", base)
	insertPromptRow(t, repo, "pg-a-3", "sess-pg-a", "turn-pg-a", "user", "p3", base.Add(time.Second))
	insertPromptRow(t, repo, "pg-a-agent", "sess-pg-a", "turn-pg-a", "agent", "reply", base.Add(2*time.Second))

	seedPostgresSession(t, repo, "task-pg-b", "sess-pg-b", "turn-pg-b", base.Add(time.Hour))
	insertPromptRow(t, repo, "pg-b-1", "sess-pg-b", "turn-pg-b", "user", "p1", base.Add(time.Hour))

	// Fixture rows bypass the create boundary (the legacy-row shape), so
	// assign ordinals with the production backfill like a migrated database.
	backfillPromptSeqForTest(t, repo)

	wantA := map[string]int{
		"pg-a-1": 1, "pg-a-zzz": 2, "pg-a-3": 3, "pg-a-agent": 0,
	}
	assertPromptOrdinals(t, ctx, repo, "sess-pg-a", wantA)

	wantB := map[string]int{"pg-b-1": 1}
	assertPromptOrdinals(t, ctx, repo, "sess-pg-b", wantB)

	// Non-first pages return absolute ordinals: paginate to the oldest page.
	pages := paginateAll(t, ctx, repo, "sess-pg-a", 1, "")
	if len(pages) != 4 {
		t.Fatalf("pages = %d, want 4", len(pages))
	}
	oldest := pages[len(pages)-1][0]
	if oldest.ID != "pg-a-1" || oldest.PromptIndex != 1 {
		t.Errorf("oldest page = %s (index %d), want pg-a-1 with absolute index 1", oldest.ID, oldest.PromptIndex)
	}
}

// TestPostgresPromptIndexCreateBoundary covers the atomic per-session create
// boundary on Postgres: fixed-clock/reverse-ID advance, the
// explicit-timestamp-preserve branch, and backward-clock clamping.
func TestPostgresPromptIndexCreateBoundary(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	seedPostgresSession(t, repo, "task-pg-bound", "sess-pg-bound", "turn-pg-bound", base)
	repo.clockNow = func() time.Time { return base }

	first := &models.Message{ID: "pg-live-z", TaskSessionID: "sess-pg-bound", TurnID: "turn-pg-bound", AuthorType: models.MessageAuthorUser, Content: "first"}
	if err := repo.CreateMessage(ctx, first); err != nil {
		t.Fatalf("first live create: %v", err)
	}
	second := &models.Message{ID: "pg-live-a", TaskSessionID: "sess-pg-bound", TurnID: "turn-pg-bound", AuthorType: models.MessageAuthorUser, Content: "second"}
	if err := repo.CreateMessage(ctx, second); err != nil {
		t.Fatalf("second live create: %v", err)
	}
	if first.PromptIndex != 1 || second.PromptIndex != 2 {
		t.Fatalf("live ordinals = (%d, %d), want (1, 2)", first.PromptIndex, second.PromptIndex)
	}
	if formatPromptKey(second.CreatedAt) != formatPromptKey(first.CreatedAt.Add(time.Microsecond)) {
		t.Errorf("key advance = %q, want one microsecond tick", formatPromptKey(second.CreatedAt))
	}

	// Explicit-timestamp-preserve branch: strictly-after-max preserved; at/before rejected.
	explicit := &models.Message{
		ID: "pg-explicit", TaskSessionID: "sess-pg-bound", TurnID: "turn-pg-bound",
		AuthorType: models.MessageAuthorUser, Content: "explicit",
		CreatedAt: base.Add(time.Minute),
	}
	if err := repo.CreateMessage(ctx, explicit); err != nil {
		t.Fatalf("explicit create: %v", err)
	}
	if explicit.PromptIndex != 3 || !explicit.CreatedAt.Equal(base.Add(time.Minute)) {
		t.Errorf("explicit create = index %d at %v, want 3 at %v", explicit.PromptIndex, explicit.CreatedAt, base.Add(time.Minute))
	}

	invalid := &models.Message{
		ID: "pg-invalid", TaskSessionID: "sess-pg-bound", TurnID: "turn-pg-bound",
		AuthorType: models.MessageAuthorUser, Content: "invalid",
		CreatedAt: base.Add(time.Minute),
	}
	if err := repo.CreateMessage(ctx, invalid); !errors.Is(err, ErrMessageTimestampNotAfterNewest) {
		t.Fatalf("equal-timestamp create error = %v, want ErrMessageTimestampNotAfterNewest", err)
	}

	// Backward clock: a live create far behind the newest user message must
	// not block — the ordinal comes from the durable sequence, and the
	// ordering timestamp clamps forward by one tick.
	repo.clockNow = func() time.Time { return base.Add(-3 * time.Minute) }
	skewed := &models.Message{ID: "pg-skewed", TaskSessionID: "sess-pg-bound", TurnID: "turn-pg-bound", AuthorType: models.MessageAuthorUser, Content: "skewed"}
	if err := repo.CreateMessage(ctx, skewed); err != nil {
		t.Fatalf("backward-clock create must not block: %v", err)
	}
	if skewed.PromptIndex != 4 {
		t.Errorf("backward-clock ordinal = %d, want 4 (durable sequence, no reuse)", skewed.PromptIndex)
	}
	if !skewed.CreatedAt.After(explicit.CreatedAt) {
		t.Errorf("clamped timestamp %v must stay after newest %v", skewed.CreatedAt, explicit.CreatedAt)
	}
	if got := formatPromptKey(skewed.CreatedAt); got != formatPromptKey(explicit.CreatedAt.Add(time.Microsecond)) {
		t.Errorf("clamped key = %q, want one tick after explicit %q", got, formatPromptKey(explicit.CreatedAt.Add(time.Microsecond)))
	}
}

// TestPostgresPromptIndexConcurrentDeleteAndCreate covers the concurrent
// delete/create regression: a user prompt deleted from another connection
// while creates are in flight must not renumber the survivors or cause the
// sequence to reuse the deleted ordinal.
func TestPostgresPromptIndexConcurrentDeleteAndCreate(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 8, 19, 15, 0, 0, 0, time.UTC)
	seedPostgresSession(t, repo, "task-pg-del", "sess-pg-del", "turn-pg-del", base)

	early := &models.Message{ID: "pg-del-early", TaskSessionID: "sess-pg-del", TurnID: "turn-pg-del", AuthorType: models.MessageAuthorUser, Content: "early"}
	if err := repo.CreateMessage(ctx, early); err != nil {
		t.Fatalf("create early: %v", err)
	}
	if early.PromptIndex != 1 {
		t.Fatalf("early ordinal = %d, want 1", early.PromptIndex)
	}

	// Two concurrent creates race a delete of the early prompt on the second
	// connection. Whatever the interleaving, survivors keep their ordinals
	// and the new prompts get fresh distinct ordinals (never reusing 1).
	messages := make([]*models.Message, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := &models.Message{
				ID:            "pg-del-new-" + string(rune('a'+i)),
				TaskSessionID: "sess-pg-del",
				TurnID:        "turn-pg-del",
				AuthorType:    models.MessageAuthorUser,
				Content:       "concurrent prompt",
			}
			errs[i] = repo.CreateMessage(ctx, msg)
			messages[i] = msg
		}(i)
	}
	if err := repo.DeleteMessage(ctx, early.ID); err != nil {
		t.Fatalf("concurrent delete of early prompt: %v", err)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent create %d: %v", i, err)
		}
	}
	ordinals := map[int]bool{messages[0].PromptIndex: true, messages[1].PromptIndex: true}
	if len(ordinals) != 2 || ordinals[1] {
		t.Fatalf("concurrent ordinals = %d/%d, want exactly {2, 3} (deleted ordinal 1 never reused)", messages[0].PromptIndex, messages[1].PromptIndex)
	}
	if !ordinals[2] || !ordinals[3] {
		t.Fatalf("concurrent ordinals = %d/%d, want exactly {2, 3}", messages[0].PromptIndex, messages[1].PromptIndex)
	}
	// The deleted prompt is gone and survivors keep their durable ordinals.
	if _, err := repo.GetMessageWithPromptIndex(ctx, early.ID); err == nil {
		t.Errorf("deleted early prompt still readable")
	}
	for _, msg := range messages {
		stored, err := repo.GetMessageWithPromptIndex(ctx, msg.ID)
		if err != nil {
			t.Fatalf("GetMessageWithPromptIndex(%s): %v", msg.ID, err)
		}
		if stored.PromptIndex != msg.PromptIndex {
			t.Errorf("reread ordinal %d != model ordinal %d for %s", stored.PromptIndex, msg.PromptIndex, msg.ID)
		}
	}
}

// TestPostgresPromptIndexConcurrentCreatesDistinct uses a real multi-connection
// pool to prove the per-session advisory lock serializes concurrent user
// creates: both final ordinals are distinct (1 and 2) and the correlated
// read agrees with each model.
func TestPostgresPromptIndexConcurrentCreatesDistinct(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	seedPostgresSession(t, repo, "task-pg-conc", "sess-pg-conc", "turn-pg-conc", base)

	const goroutines = 2
	messages := make([]*models.Message, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := &models.Message{
				ID:            "pg-conc-" + string(rune('a'+i)),
				TaskSessionID: "sess-pg-conc",
				TurnID:        "turn-pg-conc",
				AuthorType:    models.MessageAuthorUser,
				Content:       "concurrent prompt",
			}
			errs[i] = repo.CreateMessage(ctx, msg)
			messages[i] = msg
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent create %d: %v", i, err)
		}
	}
	ordinals := map[int]bool{messages[0].PromptIndex: true, messages[1].PromptIndex: true}
	if len(ordinals) != 2 || !ordinals[1] || !ordinals[2] {
		t.Fatalf("concurrent final ordinals = %d/%d, want exactly {1, 2}", messages[0].PromptIndex, messages[1].PromptIndex)
	}
	for _, msg := range messages {
		stored, err := repo.GetMessageWithPromptIndex(ctx, msg.ID)
		if err != nil {
			t.Fatalf("GetMessageWithPromptIndex(%s): %v", msg.ID, err)
		}
		if stored.PromptIndex != msg.PromptIndex {
			t.Errorf("reread ordinal %d != model ordinal %d for %s", stored.PromptIndex, msg.PromptIndex, msg.ID)
		}
	}
}

// TestPostgresListMessagesPaginatedAround exercises the PostgreSQL timestamp
// cast path for target inclusion, descending ordering, and truncation.
func TestPostgresListMessagesPaginatedAround(t *testing.T) {
	repo := openPostgresRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 19, 16, 0, 0, 0, time.UTC)
	seedPostgresSession(t, repo, "task-pg-around", "sess-pg-around", "turn-pg-around", base)
	for index, id := range []string{"pg-m1", "pg-m2", "pg-m3", "pg-m4"} {
		insertPromptRow(t, repo, id, "sess-pg-around", "turn-pg-around", "agent", id, base.Add(time.Duration(index)*time.Second))
	}

	page, hasMore, err := repo.ListMessagesPaginated(ctx, "sess-pg-around", models.ListMessagesOptions{
		Around: "pg-m2", Limit: 2, Sort: "desc",
	})
	if err != nil {
		t.Fatalf("ListMessagesPaginated around: %v", err)
	}
	if !hasMore {
		t.Fatal("around page hasMore = false, want true")
	}
	if got := messageIDs(page); !reflect.DeepEqual(got, []string{"pg-m3", "pg-m2"}) {
		t.Fatalf("around ids = %v, want [pg-m3 pg-m2]", got)
	}
}
