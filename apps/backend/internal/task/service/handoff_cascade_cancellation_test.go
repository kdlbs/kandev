package service

import (
	"context"
	"testing"
)

type contextCancellingRunCanceller struct {
	cancel context.CancelFunc
}

func (c contextCancellingRunCanceller) CancelTaskExecution(context.Context, string, string, bool) error {
	c.cancel()
	return nil
}

type cascadeContextCancellingRunCanceller struct {
	cancel context.CancelFunc
	errs   []error
}

func (c *cascadeContextCancellingRunCanceller) CancelTaskExecution(ctx context.Context, _ string, _ string, _ bool) error {
	c.errs = append(c.errs, ctx.Err())
	c.cancel()
	return ctx.Err()
}

type contextAwareArchiveCascadeRepo struct {
	*fakeCascadeRepo
}

func (r *contextAwareArchiveCascadeRepo) ArchiveTaskIfActiveWithVacatedStep(
	ctx context.Context,
	taskID, cascadeID string,
) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return r.fakeCascadeRepo.ArchiveTaskIfActiveWithVacatedStep(ctx, taskID, cascadeID)
}

// TestArchiveDeleteStopsAllRuntimes_SelfArchiveSurvivesRunCancellation covers AC-TASKS-RUNTIME-CLEANUP-001.10.
func TestArchiveDeleteStopsAllRuntimes_SelfArchiveSurvivesRunCancellation(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")

	ctx, cancel := context.WithCancel(context.Background())
	svc := NewHandoffService(
		&contextAwareArchiveCascadeRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		nil, nil, nil, newCascadeWSGroupRepo(), nil,
	)
	svc.SetRunCanceller(contextCancellingRunCanceller{cancel: cancel})

	if _, err := svc.ArchiveTaskTree(ctx, "root", false); err != nil {
		t.Fatalf("self-archive: %v", err)
	}
	task, err := tasks.GetTask(context.Background(), "root")
	if err != nil {
		t.Fatalf("load archived task: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("task remains active after its runtime cancellation")
	}
}

func TestArchiveDeleteStopsAllRuntimes_UsesLiveContextForEveryMember(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("child", "root", "ws-1")

	ctx, cancel := context.WithCancel(context.Background())
	canceller := &cascadeContextCancellingRunCanceller{cancel: cancel}
	svc := NewHandoffService(
		&contextAwareArchiveCascadeRepo{fakeCascadeRepo: newCascadeRepo(tasks)},
		nil, nil, nil, newCascadeWSGroupRepo(), nil,
	)
	svc.SetRunCanceller(canceller)

	if _, err := svc.ArchiveTaskTree(ctx, "root", true); err != nil {
		t.Fatalf("archive cascade: %v", err)
	}
	if len(canceller.errs) != 2 {
		t.Fatalf("runtime cancellations = %d, want 2", len(canceller.errs))
	}
	for i, err := range canceller.errs {
		if err != nil {
			t.Fatalf("runtime cancellation %d received canceled context: %v", i, err)
		}
	}
}
