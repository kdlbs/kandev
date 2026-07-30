package orchestrator

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/task/models"
)

// deadlineRecordingRepo wraps the real session store and captures whether the
// context propagated into GetTaskSession carries a deadline. It forces a
// terminal session state so waitForSessionReady returns on the first poll
// instead of running its full budget.
type deadlineRecordingRepo struct {
	sessionExecutorStore
	sawCall     bool
	hadDeadline bool
	deadline    time.Time
}

func (r *deadlineRecordingRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	r.sawCall = true
	r.deadline, r.hadDeadline = ctx.Deadline()
	return &models.TaskSession{ID: id, State: models.TaskSessionStateFailed, ErrorMessage: "forced-by-test"}, nil
}

// TestWaitForSessionReady_BoundsQueriesWithNonCancellableContext guards the fix
// for cubic's P2: resume/launch pass context.WithoutCancel(ctx) (no deadline) so
// the wait survives the caller's request timeout, but the GetTaskSession queries
// inside the poll loop must still be bounded by waitForSessionReady's own
// deadline — otherwise a single blocking query could hang past that budget,
// which is only checked between iterations.
func TestWaitForSessionReady_BoundsQueriesWithNonCancellableContext(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

	rec := &deadlineRecordingRepo{sessionExecutorStore: svc.repo}
	svc.repo = rec

	err := svc.waitForSessionReady(context.WithoutCancel(context.Background()), "session1")
	if err == nil {
		t.Fatal("expected error from the forced-failed session")
	}
	if !strings.Contains(err.Error(), "forced-by-test") {
		t.Fatalf("expected the forced-failed error, got %v", err)
	}
	if !rec.sawCall {
		t.Fatal("GetTaskSession was never called")
	}
	if !rec.hadDeadline {
		t.Error("GetTaskSession received a context WITHOUT a deadline — queries are unbounded (regression of cubic P2 fix)")
	}
}

func TestWaitForSessionReady_UsesAgentLaunchTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateRunning)

		rec := &deadlineRecordingRepo{sessionExecutorStore: svc.repo}
		svc.repo = rec
		startedAt := time.Now()

		if err := svc.waitForSessionReady(context.WithoutCancel(context.Background()), "session1"); err == nil {
			t.Fatal("expected error from the forced-failed session")
		}

		if !rec.hadDeadline {
			t.Fatal("GetTaskSession received a context WITHOUT a deadline")
		}
		if got := rec.deadline.Sub(startedAt); got != constants.AgentLaunchTimeout {
			t.Fatalf("waitForSessionReady deadline = %s, want %s", got, constants.AgentLaunchTimeout)
		}
	})
}
