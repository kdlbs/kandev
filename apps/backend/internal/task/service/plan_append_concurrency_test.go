package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// TestConcurrentAppendsBothSurvive pins AC-TASKS-PLAN-APPEND-003.3/003.4's
// "two concurrent appends" case: writer 1 is gated inside its write
// transaction (already holding the per-task lock, past its own HEAD read)
// while writer 2 attempts a concurrent append to the same task. Writer 2
// must queue, not read stale content, so that its own composition runs
// against writer 1's just-committed content — proving both fragments
// survive rather than one silently overwriting the other.
func TestConcurrentAppendsBothSurvive(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-gate")

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-append-gate", Title: "Plan", Content: "base", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-gate", Content: "fragment-A", Mode: PlanWriteModeAppend,
			AuthorKind: "agent", AuthorName: "A",
		})
		firstDone <- err
	}()
	<-writeStarted // writer 1 now holds the per-task lock, blocked inside its write tx

	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-gate", Content: "fragment-B", Mode: PlanWriteModeAppend,
			AuthorKind: "agent", AuthorName: "B",
		})
		secondDone <- err
	}()

	waitForPlanLockWaiters(t, svc.locks, "task-append-gate", 2)
	select {
	case err := <-secondDone:
		t.Fatalf("second append returned (err=%v) before the gated first write released the per-task lock", err)
	default:
	}

	release()
	if err := <-firstDone; err != nil {
		t.Fatalf("first append: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second append: %v", err)
	}

	head, err := svc.GetPlan(ctx, "task-append-gate")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	want := "base\n\nfragment-A\n\nfragment-B"
	if head.Content != want {
		t.Fatalf("expected both fragments to survive in commit order, got %q, want %q", head.Content, want)
	}
}

// TestAppendConcurrentWithReplaceNeverLosesEitherWrite pins
// AC-TASKS-PLAN-APPEND-003.3/003.4's "append concurrent with a replace"
// case. The append (launched first) is gated inside its write transaction,
// forcing the whole per-task lock table — the same table a plain replace
// goes through — to serialize the concurrent replace behind it. This is the
// scenario the design calls out: an "append-only" lock that didn't also
// cover replace would let the replace commit while the append was gated,
// and the append's already-composed (now stale) content would then
// overwrite it on release — the lost update this test must catch.
func TestAppendConcurrentWithReplaceNeverLosesEitherWrite(t *testing.T) {
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-append-replace-race")

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-append-replace-race", Title: "Plan", Content: "base", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreateTaskPlan: %v", err)
	}

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	release := newGuardedRelease(t, releaseWrite)
	gated := &gatedWriteRepo{Repository: repo, writeStarted: writeStarted, proceed: releaseWrite}
	svc := NewPlanService(gated, eventBus, log)

	appendDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-replace-race", Content: "fragment", Mode: PlanWriteModeAppend,
			AuthorKind: "agent", AuthorName: "A",
		})
		appendDone <- err
	}()
	<-writeStarted // append holds the per-task lock, blocked inside its write tx, past its own HEAD read

	replaceDone := make(chan error, 1)
	go func() {
		_, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
			TaskID: "task-append-replace-race", Content: "replaced", AuthorKind: "agent", AuthorName: "B",
		})
		replaceDone <- err
	}()

	waitForPlanLockWaiters(t, svc.locks, "task-append-replace-race", 2)
	select {
	case err := <-replaceDone:
		t.Fatalf("replace returned (err=%v) before the gated append released the per-task lock - the two modes are not sharing one lock", err)
	default:
	}

	release()
	if err := <-appendDone; err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := <-replaceDone; err != nil {
		t.Fatalf("replace: %v", err)
	}

	// This gating forces exactly one serial order: append commits first
	// (composing "base"+"fragment"), then the queued replace commits and
	// overwrites the whole document. The forbidden outcome this test exists
	// to catch is the append's pre-computed content landing *after* the
	// replace (i.e. "base\n\nfragment", discarding "replaced" entirely) -
	// which is exactly what an append-only lock would produce.
	head, err := svc.GetPlan(ctx, "task-append-replace-race")
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if head.Content != "replaced" {
		t.Fatalf("expected the queued replace to win (content = %q), got %q - a lost update", "replaced", head.Content)
	}
	if strings.Contains(head.Content, "fragment") {
		t.Fatalf("append's stale composed content leaked into the final document: %q", head.Content)
	}
}
