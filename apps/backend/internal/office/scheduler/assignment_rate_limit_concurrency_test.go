package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestCheckAssignmentWakeAllowance_ConcurrentEvaluationsBothAdmit covers
// AC-OFFICE-ASSIGN-RATE-002.1: with the allowance one wake short of
// exhausted, two genuinely concurrent evaluations — started together via a
// shared barrier channel, not a sequential call pair — must each observe
// room and must each admit, even though admitting both together pushes the
// task past N. The gate's count-then-insert shape offers no other outcome:
// serializing the check itself would be the "convenient" answer the spec's
// own overview warns against, and neither goroutine here inserts its row
// until after both have evaluated, so the count each one reads cannot
// reflect the other's admission.
func TestCheckAssignmentWakeAllowance_ConcurrentEvaluationsBothAdmit(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()
	taskID := "rl-task-concurrent"

	for i := 0; i < AssignmentWakeAllowanceN-1; i++ {
		createAssignmentWakeRun(t, repo, taskID, time.Now().UTC())
	}

	payload := `{"task_id":"` + taskID + `","actor_type":"agent"}`
	start := make(chan struct{})
	var wg sync.WaitGroup
	refused := make([]bool, 2)

	wg.Add(2)
	for i := range refused {
		i := i
		go func() {
			defer wg.Done()
			<-start
			refused[i] = ss.checkAssignmentWakeAllowance(ctx, "agent-1", RunReasonTaskAssigned, payload)
		}()
	}
	close(start)
	wg.Wait()

	for i, r := range refused {
		if r {
			t.Fatalf("evaluation[%d] was refused with only %d of %d already admitted; AC-OFFICE-ASSIGN-RATE-002.1 forbids refusing before N are admitted", i, AssignmentWakeAllowanceN-1, AssignmentWakeAllowanceN)
		}
	}

	// Each evaluation having found room, both now insert — mirroring what
	// queueRun's CreateRun does immediately after a non-refusing check.
	// This is the "over-admission bounded by the number of concurrent
	// evaluations" the AC permits: two concurrent admits on top of the
	// pre-seeded N-1 leaves the task one over its allowance, not unbounded.
	createAssignmentWakeRun(t, repo, taskID, time.Now().UTC())
	createAssignmentWakeRun(t, repo, taskID, time.Now().UTC())

	evaluationInstant := time.Now().UTC()
	count, err := repo.CountAgentInitiatedAssignmentWakes(
		ctx, taskID, RunReasonTaskAssigned, evaluationInstant.Add(-AssignmentWakeAllowanceWindow), evaluationInstant)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != AssignmentWakeAllowanceN+1 {
		t.Fatalf("count = %d, want %d (N-1 seeded plus the two concurrent admits)", count, AssignmentWakeAllowanceN+1)
	}
}
