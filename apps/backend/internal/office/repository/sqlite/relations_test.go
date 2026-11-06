package sqlite_test

import (
	"context"
	"fmt"
	"testing"
)

func TestTaskRelationsAreSymmetricUniqueAndBatchReadable(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	if err := repo.AddRelation(ctx, "task-b", "task-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRelation(ctx, "task-a", "task-b"); err != nil {
		t.Fatal(err)
	}
	a, err := repo.ListRelationsForTask(ctx, "task-a")
	if err != nil || len(a) != 1 || a[0] != "task-b" {
		t.Fatalf("relations for a = %v, err %v", a, err)
	}
	batch, err := repo.ListRelationsForTasks(ctx, []string{"task-a", "task-b"})
	if err != nil || len(batch["task-a"]) != 1 || len(batch["task-b"]) != 1 {
		t.Fatalf("batch relations = %v, err %v", batch, err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM task_relations`); err != nil || count != 1 {
		t.Fatalf("row count = %d, err %v", count, err)
	}
	if err := repo.RemoveRelationsForTask(ctx, "task-a"); err != nil {
		t.Fatal(err)
	}
	b, err := repo.ListRelationsForTask(ctx, "task-b")
	if err != nil || len(b) != 0 {
		t.Fatalf("relations after removal = %v, err %v", b, err)
	}
}

// TestListRelationsForTasksDoesNotDuplicateAcrossChunkBoundary covers a
// relation whose two ends land in different 500-id query chunks: both
// chunks' IN-clause queries match the same row, so without dedupe each end
// would gain a duplicate peer entry.
func TestListRelationsForTasksDoesNotDuplicateAcrossChunkBoundary(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()

	ids := make([]string, 0, 501)
	for i := 0; i < 501; i++ {
		ids = append(ids, fmt.Sprintf("task-%03d", i))
	}
	// task-000 falls in the first 500-id chunk, task-500 falls in the second.
	if err := repo.AddRelation(ctx, ids[0], ids[500]); err != nil {
		t.Fatal(err)
	}

	batch, err := repo.ListRelationsForTasks(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if got := batch[ids[0]]; len(got) != 1 || got[0] != ids[500] {
		t.Fatalf("relations for %s = %v, want exactly one peer %s", ids[0], got, ids[500])
	}
	if got := batch[ids[500]]; len(got) != 1 || got[0] != ids[0] {
		t.Fatalf("relations for %s = %v, want exactly one peer %s", ids[500], got, ids[0])
	}
}
