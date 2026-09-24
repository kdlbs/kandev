package service

import (
	"context"
	"regexp"
	"testing"
)

var workflowEntryIdentityPattern = regexp.MustCompile(`^entry:[0-9]{20}$`)

func TestService_MoveTaskWorkflowFocusReturnsCommittedEntryIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-focus", "wf-source", "step-source", nil)

	result, err := svc.MoveTask(ctx, "task-focus", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if !workflowEntryIdentityPattern.MatchString(result.WorkflowEntryIdentity) {
		t.Fatalf("workflow entry identity = %q, want entry:<20-digit transition id>", result.WorkflowEntryIdentity)
	}
	if result.MoveID != "" {
		t.Fatalf("ordinary move unexpectedly returned move id %q", result.MoveID)
	}
}

func TestService_MoveTaskWorkflowFocusOmitsIdentityForNoOpMove(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-focus-noop", "wf-source", "step-source", nil)

	result, err := svc.MoveTask(ctx, "task-focus-noop", "wf-source", "step-source", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if result.Transitioned {
		t.Fatal("same-step move reported a transition")
	}
	if result.WorkflowEntryIdentity != "" {
		t.Fatalf("same-step move returned workflow entry identity %q", result.WorkflowEntryIdentity)
	}
}
