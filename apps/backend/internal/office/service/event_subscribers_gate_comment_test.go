package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestQueueCommentRun_FanOutIncompleteLoggedOnceNoSecondDispatch pins
// AC-OFFICE-GATE-COMMENT-001.16: a channel-inbound comment always publishes
// OfficeCommentCreated without engine_dispatched (this package's
// CreateComment never dispatches synchronously — see comments.go), so
// queueCommentRun's subscriber dispatch is the only dispatch attempt for
// this comment. When that single attempt fails with
// ErrCommentFanOutIncomplete, handleCommentCreated logs it once (the same
// generic "queue comment run failed" path used for every other dispatch
// error) and there is no second, retried dispatch within this event
// delivery — the sentinel gets exactly the single redispatch the design
// allows, never an unbounded retry loop.
func TestQueueCommentRun_FanOutIncompleteLoggedOnceNoSecondDispatch(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	svc, _ := newTestServiceWithBusLogger(t, log)

	sentinelErr := fmt.Errorf("%w: task task-1 step review role \"reviewer\": boom",
		engine.ErrCommentFanOutIncomplete)
	disp := &fakeDispatcher{nextErr: sentinelErr}
	svc.SetWorkflowEngineDispatcher(disp)

	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "agent-1")
	insertTestTask(t, svc, "task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-x",
		Body:       "please decide",
	}
	if err := svc.CreateComment(ctx, comment); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	calls := disp.Calls()
	if len(calls) != 1 {
		t.Fatalf("dispatcher calls = %d, want exactly 1 (the single allowed redispatch)", len(calls))
	}
	if got := logs.FilterMessageSnippet("queue comment run failed").Len(); got != 1 {
		t.Fatalf("expected exactly 1 error log for the failed redispatch, got %d", got)
	}
}
