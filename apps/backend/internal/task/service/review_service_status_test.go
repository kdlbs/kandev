package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// orderedFindingRepo forces two status requests to reach the repository before
// it applies the reopen-then-resolve ordering that exposes stale state.
type orderedFindingRepo struct {
	reviewRepo
	transitionsReady   chan struct{}
	releaseTransitions chan struct{}
	transitionCalls    int32
	openApplied        chan struct{}
	openOnce           sync.Once
}

func (r *orderedFindingRepo) awaitTransitionStart() {
	n := atomic.AddInt32(&r.transitionCalls, 1)
	if n > 2 {
		return
	}
	if n == 2 {
		close(r.transitionsReady)
	}
	<-r.releaseTransitions
}

func (r *orderedFindingRepo) TransitionTaskReviewFindingStatus(
	ctx context.Context,
	findingID string,
	status models.ReviewFindingStatus,
) (*models.TaskReviewFinding, error) {
	r.awaitTransitionStart()
	transitioner, ok := r.reviewRepo.(interface {
		TransitionTaskReviewFindingStatus(context.Context, string, models.ReviewFindingStatus) (*models.TaskReviewFinding, error)
	})
	if !ok {
		return nil, nil
	}
	if status == models.ReviewFindingResolved {
		<-r.openApplied
	}
	finding, err := transitioner.TransitionTaskReviewFindingStatus(ctx, findingID, status)
	if status == models.ReviewFindingOpen {
		r.openOnce.Do(func() { close(r.openApplied) })
	}
	return finding, err
}

func TestReviewService_UpdateFindingStatusDoesNotRestoreStaleResolvedAt(t *testing.T) {
	base, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-status-race")
	_, findings, err := base.PublishFindings(ctx, PublishFindingsRequest{
		TaskID: "task-status-race",
		Findings: []ReviewFindingInput{{
			FilePath: "a.go", StartLine: 1, EndLine: 1, Severity: "major",
			Category: "correctness", Title: "stale", Body: "stale state",
		}},
	})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	initial, err := base.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingResolved)
	if err != nil || initial.ResolvedAt == nil {
		t.Fatalf("seed resolved finding: finding=%+v err=%v", initial, err)
	}
	initialResolvedAt := *initial.ResolvedAt

	ordered := &orderedFindingRepo{
		reviewRepo:         repo,
		transitionsReady:   make(chan struct{}),
		releaseTransitions: make(chan struct{}),
		openApplied:        make(chan struct{}),
	}
	svc := NewReviewService(ordered, eventBus, base.logger)
	results := make(chan error, 2)
	go func() {
		_, updateErr := svc.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingOpen)
		results <- updateErr
	}()
	go func() {
		_, updateErr := svc.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingResolved)
		results <- updateErr
	}()

	select {
	case <-ordered.transitionsReady:
	case <-time.After(5 * time.Second):
		t.Fatal("status requests did not reach the transition barrier")
	}
	close(ordered.releaseTransitions)
	for range 2 {
		if updateErr := <-results; updateErr != nil {
			t.Fatalf("UpdateFindingStatus: %v", updateErr)
		}
	}

	final, err := repo.GetTaskReviewFinding(ctx, findings[0].ID)
	if err != nil {
		t.Fatalf("GetTaskReviewFinding: %v", err)
	}
	if final.Status != models.ReviewFindingResolved || final.ResolvedAt == nil {
		t.Fatalf("expected the final status to be resolved with a timestamp, got %+v", final)
	}
	if final.ResolvedAt.Equal(initialResolvedAt) {
		t.Fatalf("stale resolve restored the old timestamp %v", final.ResolvedAt)
	}
}
