package sqlite

import (
	"context"
	"sync"
	"testing"
)

// TestInspectTaskTransferRelationsMemoizesSchemaDiscovery proves the N+1
// schema-inspection queries (PRAGMA table_info / information_schema.columns
// sweeps in inspectTransferRelationInventory and addDiscoveredTaskTransferRelations)
// run at most once per Repository instance: a table created after the first
// call must NOT appear in a later call's discovered projections, because the
// discovery result is served from the cache instead of being recomputed.
func TestInspectTaskTransferRelationsMemoizesSchemaDiscovery(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()

	_, firstCounts, firstInventory, err := repo.inspectTaskTransferRelations(ctx)
	if err != nil {
		t.Fatalf("inspectTaskTransferRelations (first): %v", err)
	}
	if firstInventory.labels {
		t.Fatalf("labels inventory true before office_labels/office_task_labels exist")
	}
	for _, projection := range firstCounts {
		if projection.table == "task_transfer_relations_probe" {
			t.Fatalf("unexpected probe table present before creation")
		}
	}

	// Mutate schema after the cache has been populated: add a brand-new
	// task-keyed table and the office labels tables that flip
	// transferRelationInventory.labels to true. If inspectTaskTransferRelations
	// recomputed on every call, the second call below would observe both.
	mustExecTransferTest(t, repo, `CREATE TABLE task_transfer_relations_probe (task_id TEXT NOT NULL)`)
	mustExecTransferTest(t, repo,
		`CREATE TABLE office_labels (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `CREATE TABLE office_task_labels (task_id TEXT NOT NULL, label_id TEXT NOT NULL)`)

	_, secondCounts, secondInventory, err := repo.inspectTaskTransferRelations(ctx)
	if err != nil {
		t.Fatalf("inspectTaskTransferRelations (second): %v", err)
	}
	if secondInventory.labels {
		t.Fatalf("cached inventory changed after schema mutation; caching is not effective")
	}
	for _, projection := range secondCounts {
		if projection.table == "task_transfer_relations_probe" {
			t.Fatalf("cached discovery re-ran and picked up a table created after warm-up")
		}
	}
}

// TestInspectTaskTransferRelationsConcurrentWarmupIsSafe exercises the cache's
// double-checked locking under concurrent first-use so the race detector can
// prove there is no data race between readers and the single writer that
// populates the cache.
func TestInspectTaskTransferRelationsConcurrentWarmupIsSafe(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	ctx := context.Background()

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := range goroutines {
		go func(index int) {
			defer wg.Done()
			_, _, _, err := repo.inspectTaskTransferRelations(ctx)
			errs[index] = err
		}(i)
	}
	wg.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: inspectTaskTransferRelations: %v", index, err)
		}
	}
	if !repo.transferRelationCache.ready {
		t.Fatalf("cache not marked ready after concurrent warm-up")
	}
}
