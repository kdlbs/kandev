package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TestSwitchTaskRunner_ConcurrentBaseBranchUpdateNeverBlendsWithSwitch covers
// the in-place task-repository link update class-2 writer added by this
// change: UpdateTaskRepositoryBaseBranchAndClearComparisonTarget mutates the
// same link a runner switch's compatibility re-check reads, so the two must
// serialize on the task row lock and produce only the two outcomes
// AC-TASKS-RUNNER-SWITCH-002.3a permits.
func TestSwitchTaskRunner_ConcurrentBaseBranchUpdateNeverBlendsWithSwitch(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	var wg sync.WaitGroup
	var switchErr, branchErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, switchErr = repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	}()
	go func() {
		defer wg.Done()
		_, _, branchErr = repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx, taskRepo.ID, "develop")
	}()
	wg.Wait()

	if branchErr != nil {
		t.Fatalf("UpdateTaskRepositoryBaseBranchAndClearComparisonTarget error = %v, want nil", branchErr)
	}

	switchRejectedAsStale := false
	if switchErr != nil {
		if !errors.Is(switchErr, repoerrors.ErrRunnerEvaluationUnavailable) {
			t.Fatalf("switch error = %v, want nil or ErrRunnerEvaluationUnavailable", switchErr)
		}
		switchRejectedAsStale = true
	}

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	stored, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)

	if switchRejectedAsStale {
		if stored != "profile-old" {
			t.Fatalf("switch was rejected but stored profile = %q, want unchanged profile-old", stored)
		}
		return
	}
	if stored != "profile-new" {
		t.Fatalf("switch committed but stored profile = %q, want profile-new", stored)
	}
}
