package dashboard

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// ctxCapturingPublisher is a fake TaskLifecyclePublisher that records the
// ctx.Err() observed by each call, so a test can assert the ctx actually
// used to reach the event bus is not the caller's already-cancelled one.
type ctxCapturingPublisher struct {
	getTaskCalled bool
	getTaskCtxErr error
	publishCalled bool
	publishCtxErr error
	publishedTask *taskmodels.Task
}

func (p *ctxCapturingPublisher) GetTask(ctx context.Context, id string) (*taskmodels.Task, error) {
	p.getTaskCalled = true
	p.getTaskCtxErr = ctx.Err()
	return &taskmodels.Task{ID: id}, nil
}

func (p *ctxCapturingPublisher) PublishTaskUpdated(ctx context.Context, task *taskmodels.Task, _ ...string) {
	p.publishCalled = true
	p.publishCtxErr = ctx.Err()
	p.publishedTask = task
}

// TestPublishCanonicalTaskUpdated_SurvivesCallerCancellation covers the
// window a caller (most commonly an HTTP request context) can be cancelled
// in between UpdateTaskStatus's DB write committing and this reload+publish
// running: a request disconnect must not suppress the task.updated event
// other WS-driven views depend on.
func TestPublishCanonicalTaskUpdated_SurvivesCallerCancellation(t *testing.T) {
	pub := &ctxCapturingPublisher{}
	s := &DashboardService{taskLifecycle: pub, logger: logger.Default()}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulates the caller disconnecting right after the DB write commits

	s.publishCanonicalTaskUpdated(ctx, "task-1")

	if !pub.getTaskCalled {
		t.Fatal("GetTask was not called after the caller's context was cancelled")
	}
	if pub.getTaskCtxErr != nil {
		t.Fatalf("GetTask context was cancelled (err=%v), want a detached context", pub.getTaskCtxErr)
	}
	if !pub.publishCalled {
		t.Fatal("PublishTaskUpdated was not called after the caller's context was cancelled")
	}
	if pub.publishCtxErr != nil {
		t.Fatalf("PublishTaskUpdated context was cancelled (err=%v), want a detached context", pub.publishCtxErr)
	}
	if pub.publishedTask == nil || pub.publishedTask.ID != "task-1" {
		t.Fatalf("published task = %+v, want ID task-1", pub.publishedTask)
	}
}

// nilTaskPublisher simulates GetTask returning (nil, nil): the task was
// deleted between UpdateTaskState's write and this reload.
type nilTaskPublisher struct {
	publishCalled bool
}

func (p *nilTaskPublisher) GetTask(context.Context, string) (*taskmodels.Task, error) {
	return nil, nil
}

func (p *nilTaskPublisher) PublishTaskUpdated(context.Context, *taskmodels.Task, ...string) {
	p.publishCalled = true
}

// TestPublishCanonicalTaskUpdated_NilTaskSkipsPublish asserts
// PublishTaskUpdated is never called with a nil task.
func TestPublishCanonicalTaskUpdated_NilTaskSkipsPublish(t *testing.T) {
	pub := &nilTaskPublisher{}
	s := &DashboardService{taskLifecycle: pub, logger: logger.Default()}

	s.publishCanonicalTaskUpdated(context.Background(), "task-1")

	if pub.publishCalled {
		t.Fatal("PublishTaskUpdated must not be called when GetTask returns a nil task")
	}
}
