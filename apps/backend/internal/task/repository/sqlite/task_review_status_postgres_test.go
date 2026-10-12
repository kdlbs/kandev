package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresTaskReviewFinding_ConcurrentTransitionsKeepResolvedTimestamp
// exercises the same transition through two real PostgreSQL connections.
// It skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresTaskReviewFinding_ConcurrentTransitionsKeepResolvedTimestamp(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedReviewTask(t, ctx, repo, "task-status-postgres-concurrent")
	run := newReviewRun(t, ctx, repo, "task-status-postgres-concurrent")
	f := finding(run.ID, "task-status-postgres-concurrent", "a.go", "issue", 4)
	if err := repo.CreateTaskReviewFindings(ctx, []*models.TaskReviewFinding{f}); err != nil {
		t.Fatalf("CreateTaskReviewFindings: %v", err)
	}

	start := make(chan struct{})
	results := make(chan struct {
		finding *models.TaskReviewFinding
		err     error
	}, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := repo.TransitionTaskReviewFindingStatus(ctx, f.ID, models.ReviewFindingResolved)
			results <- struct {
				finding *models.TaskReviewFinding
				err     error
			}{finding: got, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var resolvedAt *models.TaskReviewFinding
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent transition: %v", result.err)
		}
		if result.finding == nil || result.finding.Status != models.ReviewFindingResolved || result.finding.ResolvedAt == nil {
			t.Fatalf("concurrent transition returned invalid finding: %+v", result.finding)
		}
		if resolvedAt == nil {
			resolvedAt = result.finding
			continue
		}
		if !result.finding.ResolvedAt.Equal(*resolvedAt.ResolvedAt) {
			t.Fatalf("concurrent transitions returned different resolved_at values: %v and %v", resolvedAt.ResolvedAt, result.finding.ResolvedAt)
		}
	}

	final, err := repo.GetTaskReviewFinding(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetTaskReviewFinding: %v", err)
	}
	if final.Status != models.ReviewFindingResolved || final.ResolvedAt == nil {
		t.Fatalf("final finding is not resolved: %+v", final)
	}
	if resolvedAt == nil {
		t.Fatal("concurrent transitions returned no findings")
	}
	if !final.ResolvedAt.Equal(*resolvedAt.ResolvedAt) {
		t.Fatalf("final resolved_at=%v, want %v", final.ResolvedAt, resolvedAt.ResolvedAt)
	}
}
