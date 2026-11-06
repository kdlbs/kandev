package sqlite_test

import (
	"context"
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
