package github

import (
	"context"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type syncComparisonTargetObserver struct {
	store          *Store
	calls          int
	taskID         string
	candidate      taskmodels.ComparisonTargetCandidate
	persistedTitle string
}

func (o *syncComparisonTargetObserver) ReconcileComparisonTarget(
	context.Context, string, taskmodels.ComparisonTargetCandidate,
) (*taskmodels.ComparisonTargetReconciliation, error) {
	return nil, nil
}

func (o *syncComparisonTargetObserver) ReconcileComparisonTargetFromSync(
	ctx context.Context, taskID string, candidate taskmodels.ComparisonTargetCandidate,
) (*taskmodels.ComparisonTargetReconciliation, error) {
	persisted, err := o.store.GetTaskPR(ctx, taskID)
	if err != nil {
		return nil, err
	}
	o.calls++
	o.taskID = taskID
	o.candidate = candidate
	o.persistedTitle = persisted.PRTitle
	return &taskmodels.ComparisonTargetReconciliation{
		Status: taskmodels.ComparisonTargetNoMatch,
	}, nil
}

func (o *syncComparisonTargetObserver) RemoveComparisonTargetForChange(
	context.Context, string, string, string, string, int,
) error {
	return nil
}

func TestSyncTaskPR_PersistsBeforeComparisonTargetReconciliation(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	createOutcomeSyncTestTaskPR(t, store, &TaskPR{
		TaskID: "t1", Owner: "owner", Repo: "repo", PRNumber: 1,
		PRURL: "https://github.com/owner/repo/pull/1", PRTitle: "Initial",
		HeadBranch: "feat", BaseBranch: "main", State: "open",
	})

	observer := &syncComparisonTargetObserver{store: store}
	svc.SetComparisonTargetObserver(observer)
	status := &PRStatus{PR: &PR{
		Number:        1,
		Title:         "Updated title",
		State:         "open",
		RepoOwner:     "owner",
		RepoName:      "repo",
		HeadRepoOwner: "contributor",
		HeadRepoName:  "repo",
		BaseRepoOwner: "upstream",
		BaseRepoName:  "repo",
		HeadBranch:    "feat",
		BaseBranch:    "main",
	}}

	if err := svc.SyncTaskPR(ctx, "t1", status); err != nil {
		t.Fatalf("SyncTaskPR: %v", err)
	}

	if observer.calls != 1 {
		t.Fatalf("comparison reconciliation calls = %d, want 1", observer.calls)
	}
	if observer.taskID != "t1" {
		t.Fatalf("comparison reconciliation task ID = %q, want t1", observer.taskID)
	}
	if observer.persistedTitle != "Updated title" {
		t.Fatalf("persisted title during reconciliation = %q, want Updated title", observer.persistedTitle)
	}
	if observer.candidate.Number != 1 || observer.candidate.HeadRepository.Path != "contributor/repo" {
		t.Fatalf("comparison candidate = %#v, want PR 1 from contributor/repo", observer.candidate)
	}
}
