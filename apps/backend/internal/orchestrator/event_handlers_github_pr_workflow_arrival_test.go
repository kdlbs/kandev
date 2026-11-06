package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-WORKFLOW-PR-EVENTS-001.5, AC-TASKS-WORKFLOW-PR-EVENTS-001.6
func TestRegisteredPRWorkflowPreservesNewerStepArrival(t *testing.T) {
	for _, reenter := range []bool{true, false} {
		name := "same arrival commits"
		if reenter {
			name = "departure and reentry supersedes evaluation"
		}
		t.Run(name, func(t *testing.T) {
			f := newPRWorkflowFixture(t, prMergedAcceptEvents())
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var unblock sync.Once
			var pause sync.Once
			repo := &prWorkflowFaultRepository{Repository: f.repo, beforeCommit: func() {
				pause.Do(func() {
					close(entered)
					<-release
				})
			}}
			registerPRWorkflowFixture(t, f, repo)
			t.Cleanup(func() { unblock.Do(func() { close(release) }) })
			go func() {
				defer close(done)
				publishPRWorkflowFixture(t, f)
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("registered PR handler never reached actual repository CAS")
			}
			ctx := context.Background()
			if reenter {
				movePRWorkflowAwayAndBack(t, f)
			}
			before, err := f.repo.ListTaskStepTransitions(ctx, prWorkflowTaskID, 0, 10)
			if err != nil {
				t.Fatal(err)
			}
			unblock.Do(func() { close(release) })
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("registered PR handler did not finish")
			}
			wantStep, wantHistory := "step_done", 1
			if reenter {
				wantStep, wantHistory = "step_accept", 0
			}
			if got := f.taskStep(t); got != wantStep {
				t.Errorf("step after suspended PR commit = %s, want %s", got, wantStep)
			}
			if len(f.recorder.Calls()) != wantHistory {
				t.Errorf("PR history rows = %d, want %d", len(f.recorder.Calls()), wantHistory)
			}
			after, err := f.repo.ListTaskStepTransitions(ctx, prWorkflowTaskID, 0, 10)
			if err != nil {
				t.Fatal(err)
			}
			if reenter {
				assertPRWorkflowReentryLedger(t, before, after)
			}
			// On the unfixed source the stale move also starts entry work; join it.
			if f.taskStep(t) == "step_done" {
				f.waitOnEnter(t)
			}
		})
	}
}

func movePRWorkflowAwayAndBack(t *testing.T, f *prWorkflowFixture) {
	t.Helper()
	ctx := context.Background()
	for _, step := range []string{"step_exec", "step_accept"} {
		task, err := f.repo.GetTask(ctx, prWorkflowTaskID)
		if err != nil {
			t.Fatal(err)
		}
		task.WorkflowStepID = step
		if err := f.repo.UpdateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
}

func assertPRWorkflowReentryLedger(t *testing.T, before, after []models.StepTransition) {
	t.Helper()
	if len(before) != 3 || len(after) != len(before) || after[0].ID != before[0].ID {
		t.Errorf("newer arrival ledger changed: before=%+v after=%+v", before, after)
	}
}
