package dashboard

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// ctxCapturingPublisher is a fake TaskLifecyclePublisher that records the
// ctx.Err() observed by each call, so a test can assert the ctx actually
// used to reach the event bus is not the caller's already-cancelled one.
type ctxCapturingPublisher struct {
	publishCalled bool
	publishCtxErr error
	publishedID   string
}

func (p *ctxCapturingPublisher) PublishTaskUpdatedByID(ctx context.Context, id string) {
	p.publishCalled = true
	p.publishCtxErr = ctx.Err()
	p.publishedID = id
}

// TestPublishCanonicalTaskUpdated_SurvivesCallerCancellation covers the
// window a caller (most commonly an HTTP request context) can be cancelled
// after UpdateTaskStatus's DB write commits: a request disconnect must not
// suppress the task.updated event other WS-driven views depend on.
func TestPublishCanonicalTaskUpdated_SurvivesCallerCancellation(t *testing.T) {
	pub := &ctxCapturingPublisher{}
	s := &DashboardService{taskLifecycle: pub, logger: logger.Default()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulates the caller disconnecting right after the DB write commits

	s.publishCanonicalTaskUpdated(ctx, "task-1")

	if !pub.publishCalled {
		t.Fatal("PublishTaskUpdatedByID was not called after the caller's context was cancelled")
	}
	if pub.publishCtxErr != nil {
		t.Fatalf("PublishTaskUpdatedByID context was cancelled (err=%v), want a detached context", pub.publishCtxErr)
	}
	if pub.publishedID != "task-1" {
		t.Fatalf("published task ID = %q, want task-1", pub.publishedID)
	}
}
