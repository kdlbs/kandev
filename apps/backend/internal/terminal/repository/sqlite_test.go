package repository

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/terminal/models"
)

func setupTestRepo(t *testing.T) *Repository {
	t.Helper()
	// shared-cache + MaxOpenConns(1) mirrors the production writer pool
	// (see internal/task/repository/sqlite/base.go) so all goroutines see
	// the same in-memory database — without this, each connection gets
	// its own DB and the concurrency test would see "no such table".
	rawDB, err := sql.Open("sqlite3", "file::memory:?cache=shared&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	return repo
}

func TestCreate_AssignsSequentialSeqs(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	t1, err := repo.Create(ctx, "task-1", "env-1", "term-a", "")
	if err != nil {
		t.Fatalf("create #1: %v", err)
	}
	t2, err := repo.Create(ctx, "task-1", "env-1", "term-b", "")
	if err != nil {
		t.Fatalf("create #2: %v", err)
	}

	if t1.Seq != 1 {
		t.Errorf("first seq = %d, want 1", t1.Seq)
	}
	if t2.Seq != 2 {
		t.Errorf("second seq = %d, want 2", t2.Seq)
	}
	if t1.State != models.StateOpen {
		t.Errorf("default state = %q, want open", t1.State)
	}
}

func TestCreate_SeqIsPerTask(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "task-1", "env-1", "term-a", "")
	tB, err := repo.Create(ctx, "task-2", "env-2", "term-b", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tB.Seq != 1 {
		t.Errorf("seq in different task = %d, want 1", tB.Seq)
	}
}

func TestDelete_PreservesSeqGaps(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "task-1", "env-1", "t-a", "")
	t2, _ := repo.Create(ctx, "task-1", "env-1", "t-b", "")
	_, _ = repo.Create(ctx, "task-1", "env-1", "t-c", "")

	if err := repo.Delete(ctx, t2.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	tD, err := repo.Create(ctx, "task-1", "env-1", "t-d", "")
	if err != nil {
		t.Fatalf("create after delete: %v", err)
	}
	if tD.Seq != 4 {
		t.Errorf("seq after delete = %d, want 4 (gap preserved)", tD.Seq)
	}
}

func TestListByTask_ReturnsAllStates(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "task-1", "env-1", "t-a", "")
	tB, _ := repo.Create(ctx, "task-1", "env-1", "t-b", "")
	_ = repo.SetState(ctx, tB.ID, models.StateParked)

	open, err := repo.ListByTask(ctx, "task-1", false)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 {
		t.Errorf("open count = %d, want 1", len(open))
	}

	all, err := repo.ListByTask(ctx, "task-1", true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("all count = %d, want 2", len(all))
	}
}

func TestListByTask_OrderedBySeq(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := repo.Create(ctx, "task-1", "env-1", "t-"+string(rune('a'+i)), ""); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	list, err := repo.ListByTask(ctx, "task-1", true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for i, term := range list {
		if term.Seq != i+1 {
			t.Errorf("position %d seq = %d, want %d", i, term.Seq, i+1)
		}
	}
}

func TestRename_PersistsCustomName(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	term, _ := repo.Create(ctx, "task-1", "env-1", "t-a", "")
	name := "build watcher"
	if err := repo.Rename(ctx, term.ID, &name); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := repo.Get(ctx, term.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CustomName == nil || *got.CustomName != "build watcher" {
		t.Errorf("custom_name = %v, want 'build watcher'", got.CustomName)
	}
}

func TestRename_NilClearsCustomName(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	term, _ := repo.Create(ctx, "task-1", "env-1", "t-a", "")
	name := "x"
	_ = repo.Rename(ctx, term.ID, &name)
	if err := repo.Rename(ctx, term.ID, nil); err != nil {
		t.Fatalf("rename nil: %v", err)
	}

	got, _ := repo.Get(ctx, term.ID)
	if got.CustomName != nil {
		t.Errorf("custom_name after nil rename = %v, want nil", got.CustomName)
	}
}

func TestSetState_Park(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	term, _ := repo.Create(ctx, "task-1", "env-1", "t-a", "")
	if err := repo.SetState(ctx, term.ID, models.StateParked); err != nil {
		t.Fatalf("set state: %v", err)
	}

	got, _ := repo.Get(ctx, term.ID)
	if got.State != models.StateParked {
		t.Errorf("state = %q, want parked", got.State)
	}
}

func TestGet_NotFoundReturnsErrNotFound(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, err := repo.Get(ctx, "missing-id")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestCreate_ConcurrentSeqAllocation pounds Create with N goroutines and
// verifies every seq is unique (no UNIQUE constraint collisions). Locks in
// the atomic INSERT … SELECT contract.
func TestCreate_ConcurrentSeqAllocation(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	const N = 25
	var wg sync.WaitGroup
	errs := make(chan error, N)
	seqs := make(chan int, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			term, err := repo.Create(ctx, "task-1", "env-1", "t-"+uuid.New().String(), "")
			if err != nil {
				errs <- err
				return
			}
			seqs <- term.Seq
		}()
	}
	wg.Wait()
	close(errs)
	close(seqs)

	for err := range errs {
		t.Errorf("concurrent create: %v", err)
	}
	seen := make(map[int]struct{}, N)
	for s := range seqs {
		if _, dup := seen[s]; dup {
			t.Errorf("duplicate seq %d allocated", s)
		}
		seen[s] = struct{}{}
	}
	if len(seen) != N {
		t.Errorf("seen %d unique seqs, want %d", len(seen), N)
	}
}

func TestCreate_WithInitialCommand(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	term, err := repo.Create(ctx, "task-1", "env-1", "t-a", "npm run dev")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if term.InitialCommand != "npm run dev" {
		t.Errorf("initial_command = %q, want 'npm run dev'", term.InitialCommand)
	}
}

func TestDelete_TaskCascade(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "task-1", "env-1", "t-a", "")
	_, _ = repo.Create(ctx, "task-1", "env-1", "t-b", "")
	_, _ = repo.Create(ctx, "task-2", "env-2", "t-c", "")

	n, err := repo.DeleteByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("delete by task: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted count = %d, want 2", n)
	}

	rest, _ := repo.ListByTask(ctx, "task-2", true)
	if len(rest) != 1 {
		t.Errorf("task-2 untouched count = %d, want 1", len(rest))
	}
}
