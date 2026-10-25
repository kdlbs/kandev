package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRecoverTaskLaunch_ForkPRDefaultPreservesTarget(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-fork-pr-default"
	const taskRepositoryID = "task-repo-fork-pr-default"
	seedTaskLaunchRecoveryFixture(t, repo, taskID, taskRepositoryID, models.RecoveryActionRetryDefault)

	taskRepository, err := repo.GetTaskRepository(ctx, taskRepositoryID)
	if err != nil {
		t.Fatalf("GetTaskRepository: %v", err)
	}
	target := forkPRRecoveryComparisonTarget()
	taskRepository.Metadata = map[string]interface{}{}
	if err := models.PutComparisonTarget(taskRepository.Metadata, &target); err != nil {
		t.Fatalf("PutComparisonTarget: %v", err)
	}
	if err := repo.UpdateTaskRepository(ctx, taskRepository); err != nil {
		t.Fatalf("UpdateTaskRepository: %v", err)
	}

	fake := &taskLaunchRecoveryServiceFake{
		environment: &models.TaskEnvironment{ID: "failed-env", Status: models.TaskEnvironmentStatusFailed},
	}
	svc := recoveryFixtureService(t, repo, fake)
	svc.taskLaunchRecoveryWorktree = taskLaunchRecoveryWorktreeFake{branch: "trunk"}

	_, err = svc.RecoverTaskLaunch(ctx, &TaskLaunchRecoveryRequest{
		TaskID: taskID, TaskRepositoryID: taskRepositoryID,
		Action: models.RecoveryActionRetryDefault, ErrorStamp: "recovery-stamp",
	})
	if err == nil || !strings.Contains(err.Error(), "cross-repository PR base") {
		t.Fatalf("RecoverTaskLaunch error = %v, want cross-repository PR base guard", err)
	}
	if len(fake.updated) != 0 || fake.resetCalls != 0 || len(fake.moveTaskIDs) != 0 {
		t.Fatalf("rejected recovery performed writes or relaunch work: base_updates=%#v reset_calls=%d moves=%#v", fake.updated, fake.resetCalls, fake.moveTaskIDs)
	}

	repository, err := repo.GetRepository(ctx, taskRepository.RepositoryID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if repository.DefaultBranch != "main" {
		t.Fatalf("repository default branch = %q, want unchanged main", repository.DefaultBranch)
	}
	storedTaskRepository, err := repo.GetTaskRepository(ctx, taskRepositoryID)
	if err != nil {
		t.Fatalf("reload task repository: %v", err)
	}
	storedTarget, found, err := models.LoadComparisonTarget(storedTaskRepository.Metadata)
	if err != nil || !found || !storedTarget.Equal(target) {
		t.Fatalf("stored comparison target = %#v, found=%v, err=%v, want unchanged %#v", storedTarget, found, err, target)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	launchError, found := models.LoadTaskLaunchError(task.Metadata)
	if !found || launchError.Stamp() != "recovery-stamp" {
		t.Fatalf("launch error = %#v, found=%v, want original recovery stamp", launchError, found)
	}
}

func TestRecoverTaskLaunch_DefaultRecoveryStillUpdatesOrdinaryBranch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-ordinary-default"
	const taskRepositoryID = "task-repo-ordinary-default"
	seedTaskLaunchRecoveryFixture(t, repo, taskID, taskRepositoryID, models.RecoveryActionRetryDefault)

	fake := &taskLaunchRecoveryServiceFake{}
	svc := recoveryFixtureService(t, repo, fake)
	svc.taskLaunchRecoveryWorktree = taskLaunchRecoveryWorktreeFake{branch: "trunk"}
	source, err := svc.loadTaskLaunchRecoverySource(ctx, &TaskLaunchRecoveryRequest{
		TaskID: taskID, TaskRepositoryID: taskRepositoryID,
		Action: models.RecoveryActionRetryDefault, ErrorStamp: "recovery-stamp",
	})
	if err != nil {
		t.Fatalf("loadTaskLaunchRecoverySource: %v", err)
	}
	if err := svc.recoverTaskLaunchBranch(ctx, &TaskLaunchRecoveryRequest{
		TaskID: taskID, TaskRepositoryID: taskRepositoryID,
		Action: models.RecoveryActionRetryDefault, ErrorStamp: "recovery-stamp",
	}, source); err != nil {
		t.Fatalf("recoverTaskLaunchBranch: %v", err)
	}
	if len(fake.updated) != 0 || len(fake.systemUpdated) != 1 || fake.systemUpdated[0].BaseBranch != "trunk" {
		t.Fatalf("manual updates = %#v, system updates = %#v, want system default trunk", fake.updated, fake.systemUpdated)
	}
	repository, err := repo.GetRepository(ctx, "repo-recovery")
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if repository.DefaultBranch != "trunk" {
		t.Fatalf("repository default branch = %q, want trunk", repository.DefaultBranch)
	}
}

func forkPRRecoveryComparisonTarget() models.ComparisonTarget {
	return models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42,
		HeadBranch: "feature/recovery", TargetBranch: "release/next",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork-owner/widget", RemoteURL: "https://github.com/fork-owner/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", RemoteURL: "https://github.com/upstream/widget.git",
		},
	}
}
