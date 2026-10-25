package service

import (
	"context"
	"errors"
	"testing"
)

type recordingTaskLifecycleCoordinator struct {
	calls    []string
	outcome  *CascadeOutcome
	err      error
	reason   string
	reasoned bool
}

func (c *recordingTaskLifecycleCoordinator) DeleteTaskTree(_ context.Context, rootID string, _ bool) (*CascadeOutcome, error) {
	c.calls = append(c.calls, rootID)
	return c.outcome, c.err
}

func (c *recordingTaskLifecycleCoordinator) DeleteTaskTreeWithReason(_ context.Context, rootID string, _ bool, reason string) (*CascadeOutcome, error) {
	c.calls = append(c.calls, rootID)
	c.reason = reason
	c.reasoned = true
	return c.outcome, c.err
}

func TestDeleteTaskWithLifecycleUsesCoordinator(t *testing.T) {
	coordinator := &recordingTaskLifecycleCoordinator{}
	svc := &Service{taskLifecycleCoordinator: coordinator}

	if err := svc.DeleteTaskWithLifecycle(context.Background(), "task-1"); err != nil {
		t.Fatalf("DeleteTaskWithLifecycle: %v", err)
	}
	if len(coordinator.calls) != 1 || coordinator.calls[0] != "task-1" {
		t.Fatalf("coordinator calls = %v, want [task-1]", coordinator.calls)
	}
}

func TestDeleteTaskWithLifecycleSuppressesPostCommitHousekeepingError(t *testing.T) {
	cleanupErr := errors.New("cleanup failed after delete")
	coordinator := &recordingTaskLifecycleCoordinator{
		outcome: &CascadeOutcome{ArchivedTaskIDs: []string{"task-1"}},
		err:     &CascadePostCommitError{Err: cleanupErr},
	}
	svc := &Service{taskLifecycleCoordinator: coordinator}

	if err := svc.DeleteTaskWithLifecycle(context.Background(), "task-1"); err != nil {
		t.Fatalf("DeleteTaskWithLifecycle returned post-commit error: %v", err)
	}
}

func TestDeleteTaskWithLifecycleAndReasonUsesReasonAwareCoordinator(t *testing.T) {
	coordinator := &recordingTaskLifecycleCoordinator{}
	svc := &Service{taskLifecycleCoordinator: coordinator}

	if err := svc.DeleteTaskWithLifecycleAndReason(context.Background(), "task-1", "review_pr_merged"); err != nil {
		t.Fatalf("DeleteTaskWithLifecycleAndReason: %v", err)
	}
	if !coordinator.reasoned || coordinator.reason != "review_pr_merged" {
		t.Fatalf("reasoned deletion = %v, reason %q", coordinator.reasoned, coordinator.reason)
	}
}

func TestRollbackPartialTaskUsesLifecycleCoordinator(t *testing.T) {
	cause := errors.New("post-insert finalization failed")
	coordinator := &recordingTaskLifecycleCoordinator{}
	svc := &Service{taskLifecycleCoordinator: coordinator}

	if err := svc.rollbackPartialTask(context.Background(), "task-rollback", cause); !errors.Is(err, cause) {
		t.Fatalf("rollbackPartialTask error = %v, want original cause", err)
	}
	if len(coordinator.calls) != 1 || coordinator.calls[0] != "task-rollback" {
		t.Fatalf("coordinator calls = %v, want [task-rollback]", coordinator.calls)
	}
}
